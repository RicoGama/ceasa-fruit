package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/mail"
)

// ---------------------------------------------------------------------------
// Tipos da API
// ---------------------------------------------------------------------------

// Me descreve o usuário logado, as organizações dele e qual está ativa.
// É o que o front chama logo ao abrir para saber "quem sou eu".
type Me struct {
	User                  User                     `json:"user"`
	Organizations         []OrganizationMembership `json:"organizations"`
	CurrentOrganizationID *int64                   `json:"currentOrganizationId" doc:"null until the user selects an organization"`
}

type User struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	IsPlatformAdmin bool   `json:"isPlatformAdmin"`
}

// OrganizationMembership é uma organização da qual o usuário participa, com o papel dele.
type OrganizationMembership struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status" enum:"active,suspended"`
	Role   string `json:"role" enum:"owner,admin,member"`
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

type signupInput struct {
	Body struct {
		Name             string `json:"name" minLength:"2" maxLength:"120"`
		Email            string `json:"email" format:"email" maxLength:"254"`
		Password         string `json:"password" minLength:"8" maxLength:"128"`
		OrganizationName string `json:"organizationName" minLength:"2" maxLength:"120" doc:"Company that is subscribing to the platform"`
	}
}

func (in *signupInput) Resolve(huma.Context) []error {
	in.Body.Name = strings.TrimSpace(in.Body.Name)
	in.Body.Email = normalizeEmail(in.Body.Email)
	in.Body.OrganizationName = strings.TrimSpace(in.Body.OrganizationName)
	return nil
}

type loginInput struct {
	Body struct {
		Email    string `json:"email" maxLength:"254"`
		Password string `json:"password" maxLength:"128"`
	}
}

type switchOrganizationInput struct {
	Body struct {
		OrganizationID int64 `json:"organizationId" minimum:"1"`
	}
}

// ---------------------------------------------------------------------------
// Registro das rotas
// ---------------------------------------------------------------------------

// EmailQueue é o que as rotas de autenticação precisam da fila: colocar um
// e-mail para envio dentro de uma transação (*queue.Queue satisfaz).
type EmailQueue interface {
	EnqueueEmail(ctx context.Context, tx pgx.Tx, msg mail.Message) error
}

// Options agrupa as dependências das rotas de autenticação.
type Options struct {
	Pool    *pgxpool.Pool
	Cookies Cookies
	Emails  EmailQueue
	// AppURL é o endereço do front, usado nos links dos e-mails.
	AppURL string
}

type handler struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	cookies Cookies
	emails  EmailQueue
	appURL  string
}

// Register adiciona as rotas de autenticação.
//   - public: rotas sem login (cadastro, login, logout, recuperação de senha)
//   - authed: rotas que exigem login, mas não organização (me, trocar organização)
func Register(public, authed huma.API, opts Options) {
	h := &handler{
		pool: opts.Pool, q: db.New(opts.Pool), cookies: opts.Cookies,
		emails: opts.Emails, appURL: opts.AppURL,
	}
	tags := []string{"Auth"}

	huma.Register(public, huma.Operation{
		OperationID:   "signup",
		Method:        http.MethodPost,
		Path:          "/auth/signup",
		Summary:       "Create an account and a new organization",
		Tags:          tags,
		DefaultStatus: http.StatusCreated,
	}, h.signup)

	huma.Register(public, huma.Operation{
		OperationID: "login",
		Method:      http.MethodPost,
		Path:        "/auth/login",
		Summary:     "Log in",
		Tags:        tags,
	}, h.login)

	huma.Register(public, huma.Operation{
		OperationID:   "logout",
		Method:        http.MethodPost,
		Path:          "/auth/logout",
		Summary:       "Log out",
		Tags:          tags,
		DefaultStatus: http.StatusNoContent,
	}, h.logout)

	huma.Register(public, huma.Operation{
		OperationID:   "forgot-password",
		Method:        http.MethodPost,
		Path:          "/auth/password/forgot",
		Summary:       "Request a password reset link by email",
		Description:   "Always answers 202, whether the email exists or not, so it cannot be used to discover accounts.",
		Tags:          tags,
		DefaultStatus: http.StatusAccepted,
	}, h.forgotPassword)

	huma.Register(public, huma.Operation{
		OperationID:   "reset-password",
		Method:        http.MethodPost,
		Path:          "/auth/password/reset",
		Summary:       "Set a new password using the emailed token",
		Description:   "Ends every session of the user; they must log in again.",
		Tags:          tags,
		DefaultStatus: http.StatusNoContent,
	}, h.resetPassword)

	huma.Register(authed, huma.Operation{
		OperationID: "get-me",
		Method:      http.MethodGet,
		Path:        "/auth/me",
		Summary:     "Current user and organizations",
		Tags:        tags,
	}, h.me)

	huma.Register(authed, huma.Operation{
		OperationID: "switch-organization",
		Method:      http.MethodPut,
		Path:        "/auth/organization",
		Summary:     "Select the active organization",
		Tags:        tags,
	}, h.switchOrganization)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// signup cria usuário + organização + vínculo de dono + sessão, tudo numa
// transação: ou grava tudo, ou nada (sem usuário "órfão" se algo falhar no meio).
func (h *handler) signup(ctx context.Context, in *signupInput) (*meWithCookieOutput, error) {
	hash, err := HashPassword(in.Body.Password)
	if err != nil {
		return nil, err
	}
	token, tokenHash := NewToken()
	expires := time.Now().Add(SessionTTL)

	var user db.User
	var org db.Organization

	// pgx.BeginFunc abre a transação, roda a função e faz COMMIT se ela
	// devolver nil, ou ROLLBACK se devolver erro.
	err = pgx.BeginFunc(ctx, h.pool, func(tx pgx.Tx) error {
		q := h.q.WithTx(tx) // mesmas queries, mas dentro da transação

		var err error
		user, err = q.CreateUser(ctx, db.CreateUserParams{
			Name: in.Body.Name, Email: in.Body.Email, PasswordHash: hash,
		})
		if err != nil {
			return err
		}
		org, err = q.CreateOrganization(ctx, in.Body.OrganizationName)
		if err != nil {
			return err
		}
		err = q.CreateMembership(ctx, db.CreateMembershipParams{
			OrganizationID: org.ID, UserID: user.ID, Role: "owner",
		})
		if err != nil {
			return err
		}
		err = q.CreateSession(ctx, db.CreateSessionParams{
			TokenHash: tokenHash, UserID: user.ID, OrganizationID: &org.ID, ExpiresAt: expires,
		})
		if err != nil {
			return err
		}

		// E-mail de boas-vindas: entra na fila NA MESMA transação. A resposta
		// não espera o SMTP, e se o cadastro falhar o e-mail some junto.
		msg, err := mail.Render("welcome", user.Email, user.Name, map[string]any{
			"Name": user.Name, "OrganizationName": org.Name, "AppURL": h.appURL,
		})
		if err != nil {
			return err
		}
		return h.emails.EnqueueEmail(ctx, tx, msg)
	})
	if isUniqueViolation(err) {
		return nil, huma.Error409Conflict("email already registered")
	}
	if err != nil {
		return nil, err
	}

	me, err := h.buildMe(ctx, user.ID, user.Name, user.Email, user.IsPlatformAdmin, &org.ID)
	if err != nil {
		return nil, err
	}
	return &meWithCookieOutput{SetCookie: h.cookies.New(token, expires), Body: me}, nil
}

func (h *handler) login(ctx context.Context, in *loginInput) (*meWithCookieOutput, error) {
	// Mesma mensagem para todo tipo de falha: não revelamos se o e-mail existe.
	invalid := huma.Error401Unauthorized("invalid email or password")

	user, err := h.q.GetUserByEmail(ctx, normalizeEmail(in.Body.Email))
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = VerifyPassword(dummyHash(), in.Body.Password) // gasta o mesmo tempo
		return nil, invalid
	}
	if err != nil {
		return nil, err
	}

	ok, err := VerifyPassword(user.PasswordHash, in.Body.Password)
	if err != nil {
		return nil, err
	}
	if !ok || !user.Active {
		return nil, invalid
	}

	orgs, err := h.q.ListUserOrganizations(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	// Com uma organização só, já entra nela. Com várias, o front mostra a
	// lista e chama PUT /auth/organization.
	var current *int64
	if len(orgs) == 1 {
		current = &orgs[0].ID
	}

	token, tokenHash := NewToken()
	expires := time.Now().Add(SessionTTL)
	err = h.q.CreateSession(ctx, db.CreateSessionParams{
		TokenHash: tokenHash, UserID: user.ID, OrganizationID: current, ExpiresAt: expires,
	})
	if err != nil {
		return nil, err
	}

	return &meWithCookieOutput{
		SetCookie: h.cookies.New(token, expires),
		Body:      toMe(user.ID, user.Name, user.Email, user.IsPlatformAdmin, orgs, current),
	}, nil
}

// logout apaga a sessão no banco (se houver) e manda o navegador apagar o cookie.
func (h *handler) logout(ctx context.Context, _ *struct{}) (*logoutOutput, error) {
	if s, ok := FromContext(ctx); ok {
		if err := h.q.DeleteSession(ctx, s.TokenHash); err != nil {
			return nil, err
		}
	}
	return &logoutOutput{SetCookie: h.cookies.Clear()}, nil
}

func (h *handler) me(ctx context.Context, _ *struct{}) (*meOutput, error) {
	s, _ := FromContext(ctx) // RequireUser garante que existe
	me, err := h.buildMe(ctx, s.UserID, s.UserName, s.UserEmail, s.IsPlatformAdmin, s.OrganizationID)
	if err != nil {
		return nil, err
	}
	return &meOutput{Body: me}, nil
}

func (h *handler) switchOrganization(ctx context.Context, in *switchOrganizationInput) (*meOutput, error) {
	s, _ := FromContext(ctx)
	orgID := in.Body.OrganizationID

	m, err := h.q.GetMembership(ctx, db.GetMembershipParams{UserID: s.UserID, OrganizationID: orgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, huma.Error403Forbidden("not a member of this organization")
	}
	if err != nil {
		return nil, err
	}
	if m.OrganizationStatus != "active" {
		return nil, huma.Error403Forbidden("organization is suspended")
	}

	err = h.q.SetSessionOrganization(ctx, db.SetSessionOrganizationParams{
		TokenHash: s.TokenHash, OrganizationID: &orgID,
	})
	if err != nil {
		return nil, err
	}

	me, err := h.buildMe(ctx, s.UserID, s.UserName, s.UserEmail, s.IsPlatformAdmin, &orgID)
	if err != nil {
		return nil, err
	}
	return &meOutput{Body: me}, nil
}

// ---------------------------------------------------------------------------
// Funções auxiliares
// ---------------------------------------------------------------------------

func (h *handler) buildMe(ctx context.Context, userID int64, name, email string, admin bool, current *int64) (Me, error) {
	orgs, err := h.q.ListUserOrganizations(ctx, userID)
	if err != nil {
		return Me{}, err
	}
	return toMe(userID, name, email, admin, orgs, current), nil
}

func toMe(userID int64, name, email string, admin bool, orgs []db.ListUserOrganizationsRow, current *int64) Me {
	me := Me{
		User:                  User{ID: userID, Name: name, Email: email, IsPlatformAdmin: admin},
		Organizations:         make([]OrganizationMembership, 0, len(orgs)),
		CurrentOrganizationID: current,
	}
	for _, o := range orgs {
		me.Organizations = append(me.Organizations, OrganizationMembership{
			ID: o.ID, Name: o.Name, Status: o.Status, Role: o.Role,
		})
	}
	return me
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ---------------------------------------------------------------------------
// Recuperação de senha
// ---------------------------------------------------------------------------

// passwordResetTTL é quanto tempo o link de redefinição vale.
const passwordResetTTL = time.Hour

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
	user, err := h.q.GetUserByEmail(ctx, normalizeEmail(in.Body.Email))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !user.Active) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	token, tokenHash := NewToken()
	err = pgx.BeginFunc(ctx, h.pool, func(tx pgx.Tx) error {
		err := h.q.WithTx(tx).CreatePasswordResetToken(ctx, db.CreatePasswordResetTokenParams{
			TokenHash: tokenHash, UserID: user.ID, ExpiresAt: time.Now().Add(passwordResetTTL),
		})
		if err != nil {
			return err
		}
		msg, err := mail.Render("password_reset", user.Email, user.Name, map[string]any{
			"Name":             user.Name,
			"ResetURL":         h.appURL + "/reset-password?token=" + token,
			"ExpiresInMinutes": int(passwordResetTTL.Minutes()),
		})
		if err != nil {
			return err
		}
		return h.emails.EnqueueEmail(ctx, tx, msg)
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

	err = pgx.BeginFunc(ctx, h.pool, func(tx pgx.Tx) error {
		q := h.q.WithTx(tx)
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
