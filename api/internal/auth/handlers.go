package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/mail"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// ---------------------------------------------------------------------------
// Tipos da API
// ---------------------------------------------------------------------------

// Me descreve o usuário logado e a empresa da instalação.
// É o que o front chama logo ao abrir para saber "quem sou eu".
type Me struct {
	User    CurrentUser  `json:"user"`
	Company *CompanyName `json:"company" doc:"null until the company is configured (make seed)"`
}

type CurrentUser struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role" enum:"admin,manager,seller,checker,finance,driver"`
}

type CompanyName struct {
	Name      string  `json:"name"`
	TradeName *string `json:"tradeName"`
}

// Saídas com e sem cookie. O campo com a tag header:"Set-Cookie" vira um
// cabeçalho da resposta (o Huma chama o método String() do http.Cookie).
type meOutput struct {
	Body Me
}

type meWithCookieOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      Me
}

type logoutOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
}

type loginInput struct {
	Body struct {
		Email    string `json:"email" maxLength:"254"`
		Password string `json:"password" maxLength:"128"`
	}
}

// EmailQueue é o que as rotas de autenticação precisam da fila: colocar um
// e-mail para envio dentro de uma transação (*queue.Queue satisfaz).
type EmailQueue interface {
	EnqueueEmail(ctx context.Context, tx pgx.Tx, msg mail.Message) error
}

// Options agrupa as dependências das rotas de autenticação.
type Options struct {
	DB      postgres.DB
	Cookies Cookies
	Emails  EmailQueue
	// AppURL é o endereço do front, usado nos links dos e-mails.
	AppURL string
}

type handler struct {
	db      postgres.DB
	cookies Cookies
	emails  EmailQueue
	appURL  string
}

// Register adiciona as rotas de autenticação.
//   - public: rotas sem login (login, logout, recuperação de senha)
//   - authed: rotas que exigem login (me)
//
// Não há cadastro público: quem cria usuários é o administrador (pacote user).
func Register(public, authed huma.API, opts Options) {
	h := &handler{
		db: opts.DB, cookies: opts.Cookies,
		emails: opts.Emails, appURL: opts.AppURL,
	}
	tags := []string{"Auth"}

	huma.Register(public, huma.Operation{
		OperationID: "login", Method: http.MethodPost, Path: "/auth/login",
		Summary: "Log in", Tags: tags,
	}, h.login)

	huma.Register(public, huma.Operation{
		OperationID: "logout", Method: http.MethodPost, Path: "/auth/logout",
		Summary: "Log out", Tags: tags, DefaultStatus: http.StatusNoContent,
	}, h.logout)

	huma.Register(public, huma.Operation{
		OperationID: "forgot-password", Method: http.MethodPost, Path: "/auth/password/forgot",
		Summary:     "Request a password reset link by email",
		Description: "Always answers 202, whether the email exists or not, so it cannot be used to discover accounts.",
		Tags:        tags, DefaultStatus: http.StatusAccepted,
	}, h.forgotPassword)

	huma.Register(public, huma.Operation{
		OperationID: "reset-password", Method: http.MethodPost, Path: "/auth/password/reset",
		Summary:     "Set a new password using the emailed token (reset or invite)",
		Description: "Ends every session of the user; they must log in again.",
		Tags:        tags, DefaultStatus: http.StatusNoContent,
	}, h.resetPassword)

	huma.Register(authed, huma.Operation{
		OperationID: "get-me", Method: http.MethodGet, Path: "/auth/me",
		Summary: "Current user and company", Tags: tags,
	}, h.me)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (h *handler) login(ctx context.Context, in *loginInput) (*meWithCookieOutput, error) {
	// Mesma mensagem para todo tipo de falha: não revelamos se o e-mail existe.
	invalid := huma.Error401Unauthorized("invalid email or password")

	var (
		token   string
		expires time.Time
		me      Me
	)
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		user, err := tx.GetUserByEmail(ctx, NormalizeEmail(in.Body.Email))
		if errors.Is(err, pgx.ErrNoRows) {
			_, _ = VerifyPassword(dummyHash(), in.Body.Password) // gasta o mesmo tempo
			return invalid
		}
		if err != nil {
			return err
		}

		ok, err := VerifyPassword(user.PasswordHash, in.Body.Password)
		if err != nil {
			return err
		}
		if !ok || !user.Active {
			return invalid
		}

		var tokenHash []byte
		token, tokenHash = NewToken()
		expires = time.Now().Add(SessionTTL)
		err = tx.CreateSession(ctx, db.CreateSessionParams{TokenHash: tokenHash, UserID: user.ID, ExpiresAt: expires})
		if err != nil {
			return err
		}
		me, err = buildMe(ctx, tx, CurrentUser{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role})
		return err
	})
	if err != nil {
		return nil, err
	}
	return &meWithCookieOutput{SetCookie: h.cookies.New(token, expires), Body: me}, nil
}

// logout apaga a sessão no banco (se houver) e manda o navegador apagar o cookie.
func (h *handler) logout(ctx context.Context, _ *struct{}) (*logoutOutput, error) {
	if s, ok := FromContext(ctx); ok {
		err := h.db.InTx(ctx, func(tx postgres.Tx) error {
			return tx.DeleteSession(ctx, s.TokenHash)
		})
		if err != nil {
			return nil, err
		}
	}
	return &logoutOutput{SetCookie: h.cookies.Clear()}, nil
}

func (h *handler) me(ctx context.Context, _ *struct{}) (*meOutput, error) {
	s, _ := FromContext(ctx) // RequireUser garante que existe
	var me Me
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		var err error
		me, err = buildMe(ctx, tx, CurrentUser{ID: s.UserID, Name: s.UserName, Email: s.UserEmail, Role: s.Role})
		return err
	})
	if err != nil {
		return nil, err
	}
	return &meOutput{Body: me}, nil
}

func buildMe(ctx context.Context, tx postgres.Tx, u CurrentUser) (Me, error) {
	me := Me{User: u}
	c, err := tx.GetCompany(ctx)
	switch {
	case errors.Is(err, pgx.ErrNoRows): // empresa ainda não configurada
	case err != nil:
		return Me{}, err
	default:
		me.Company = &CompanyName{Name: c.Name, TradeName: c.TradeName}
	}
	return me, nil
}

// ---------------------------------------------------------------------------
// Recuperação de senha
// ---------------------------------------------------------------------------

// PasswordResetTTL é quanto tempo o link de redefinição vale.
const PasswordResetTTL = time.Hour

type forgotPasswordInput struct {
	Body struct {
		Email string `json:"email" maxLength:"254"`
	}
}

type resetPasswordInput struct {
	Body struct {
		Token    string `json:"token" minLength:"10" maxLength:"100"`
		Password string `json:"password" minLength:"8" maxLength:"128"`
	}
}

// forgotPassword responde sempre 202, exista o e-mail ou não. Se a resposta
// mudasse, qualquer um descobriria quais e-mails têm conta no sistema.
func (h *handler) forgotPassword(ctx context.Context, in *forgotPasswordInput) (*struct{}, error) {
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		user, err := tx.GetUserByEmail(ctx, NormalizeEmail(in.Body.Email))
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !user.Active) {
			return nil // e-mail desconhecido: não faz nada, e responde igual
		}
		if err != nil {
			return err
		}
		return SendPasswordLink(ctx, tx, h.emails, h.appURL, user, PasswordLinkReset)
	})
	return nil, err
}

// resetPassword troca a senha usando o token do e-mail. O token é de uso
// único, e todas as sessões abertas do usuário são encerradas: se alguém
// tinha entrado com a senha antiga, perde o acesso.
func (h *handler) resetPassword(ctx context.Context, in *resetPasswordInput) (*struct{}, error) {
	hash, err := HashPassword(in.Body.Password)
	if err != nil {
		return nil, err
	}

	err = h.db.InTx(ctx, func(q postgres.Tx) error {
		userID, err := q.ConsumePasswordResetToken(ctx, HashToken(strings.TrimSpace(in.Body.Token)))
		if err != nil {
			return err
		}
		if err := q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: userID, PasswordHash: hash}); err != nil {
			return err
		}
		if err := q.InvalidatePasswordResetTokens(ctx, userID); err != nil {
			return err
		}
		return q.DeleteUserSessions(ctx, userID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, huma.Error400BadRequest("invalid or expired token")
	}
	return nil, err
}

// ---------------------------------------------------------------------------
// Link de senha por e-mail (redefinição ou convite)
// ---------------------------------------------------------------------------

// PasswordLinkKind diferencia o e-mail de "esqueci a senha" do de convite.
type PasswordLinkKind int

const (
	PasswordLinkReset  PasswordLinkKind = iota // vale 1 hora
	PasswordLinkInvite                         // vale 3 dias: novo usuário cria a senha
)

// SendPasswordLink cria um token de uso único e coloca na fila o e-mail com o
// link para criar ou redefinir a senha, tudo dentro da transação tx.
// Usado aqui ("esqueci a senha") e pelo cadastro de usuários (convite).
func SendPasswordLink(ctx context.Context, tx postgres.Tx, emails EmailQueue, appURL string, user db.User, kind PasswordLinkKind) error {
	ttl, template, query := PasswordResetTTL, "password_reset", ""
	if kind == PasswordLinkInvite {
		ttl, template, query = 72*time.Hour, "invite", "&invite=1"
	}

	token, tokenHash := NewToken()
	err := tx.CreatePasswordResetToken(ctx, db.CreatePasswordResetTokenParams{
		TokenHash: tokenHash, UserID: user.ID, ExpiresAt: time.Now().Add(ttl),
	})
	if err != nil {
		return err
	}
	msg, err := mail.Render(template, user.Email, user.Name, map[string]any{
		"Name":             user.Name,
		"ResetURL":         appURL + "/reset-password?token=" + token + query,
		"ExpiresInMinutes": int(ttl.Minutes()),
		"ExpiresInDays":    int(ttl.Hours() / 24),
		"AppURL":           appURL,
	})
	if err != nil {
		return err
	}
	return emails.EnqueueEmail(ctx, tx.Raw, msg)
}

// NormalizeEmail tira espaços e deixa em minúsculas (como o banco exige).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
