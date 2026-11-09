// Package auth cuida de login, sessões e do controle de acesso por organização.
//
// Fluxo resumido:
//  1. Login/cadastro cria uma sessão: um token aleatório vai para o navegador
//     num cookie HttpOnly e o hash do token vai para a tabela sessions.
//  2. Em toda requisição, o middleware LoadSession lê o cookie, busca a sessão
//     e coloca um *Session no context.Context.
//  3. Os middlewares do Huma (RequireUser, RequireOrganization,
//     RequirePlatformAdmin) barram quem não pode acessar cada grupo de rotas.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
)

const (
	// CookieName é o nome do cookie de sessão no navegador.
	CookieName = "ceasa_session"
	// SessionTTL é quanto tempo uma sessão dura sem precisar logar de novo.
	SessionTTL = 30 * 24 * time.Hour
)

// Session é o usuário autenticado da requisição atual.
type Session struct {
	TokenHash       []byte
	UserID          int64
	UserName        string
	UserEmail       string
	IsPlatformAdmin bool

	// Organização selecionada; nil se o usuário ainda não escolheu uma.
	OrganizationID     *int64
	Role               string // "owner", "admin" ou "member" (vazio sem organização)
	OrganizationStatus string // "active" ou "suspended" (vazio sem organização)
}

// ---------------------------------------------------------------------------
// Session no context.Context
// ---------------------------------------------------------------------------

// ctxKey é um tipo privado: impede que outro pacote use a mesma chave por acidente.
type ctxKey struct{}

// WithSession devolve um novo context carregando a sessão.
func WithSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// FromContext devolve a sessão da requisição, se houver.
func FromContext(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(*Session)
	return s, ok && s != nil
}

// OrganizationID devolve a organização ativa da requisição.
//
// Só pode ser chamada em rotas protegidas por RequireOrganization. Se não
// houver organização, é erro de programação (alguém esqueceu o middleware),
// então usamos panic: o middleware Recoverer transforma em 500 e o log mostra.
func OrganizationID(ctx context.Context) int64 {
	s, ok := FromContext(ctx)
	if !ok || s.OrganizationID == nil {
		panic("auth.OrganizationID called on a route without RequireOrganization")
	}
	return *s.OrganizationID
}

// ---------------------------------------------------------------------------
// Tokens e cookies
// ---------------------------------------------------------------------------

// NewToken gera um token aleatório (128 bits) e o hash que vai para o banco.
func NewToken() (token string, hash []byte) {
	token = rand.Text() // Go 1.24+: texto aleatório criptograficamente seguro
	return token, HashToken(token)
}

// HashToken calcula o SHA-256 do token. Não precisa de Argon2 aqui: o token já
// é aleatório e longo, então não dá para adivinhá-lo por força bruta.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:] // converte o array [32]byte em slice []byte
}

// Cookies monta os cookies de sessão. Secure deve ser true em produção (HTTPS).
type Cookies struct {
	Secure bool
}

// New cria o cookie que guarda o token no navegador.
//   - HttpOnly: o JavaScript da página não consegue ler (protege contra XSS).
//   - SameSite=Lax: o navegador não envia o cookie em POSTs vindos de outros
//     sites (protege contra CSRF).
func (c Cookies) New(token string, expires time.Time) http.Cookie {
	return http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// Clear cria um cookie vencido, que faz o navegador apagar o de sessão.
func (c Cookies) Clear() http.Cookie {
	return http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// ---------------------------------------------------------------------------
// Middleware (chi / net/http)
// ---------------------------------------------------------------------------

// SessionLoader é o que o middleware precisa do banco.
type SessionLoader interface {
	GetSession(ctx context.Context, tokenHash []byte) (db.GetSessionRow, error)
}

// LoadSession lê o cookie e, se a sessão for válida, coloca-a no context.
// Ele NÃO bloqueia ninguém: quem decide se a rota exige login são os
// middlewares Require* do Huma. Assim rotas públicas continuam funcionando.
func LoadSession(loader SessionLoader, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r) // sem cookie: segue como anônimo
				return
			}

			hash := HashToken(cookie.Value)
			row, err := loader.GetSession(r.Context(), hash)
			if errors.Is(err, pgx.ErrNoRows) {
				next.ServeHTTP(w, r) // sessão vencida ou inexistente: anônimo
				return
			}
			if err != nil {
				logger.Error("loading session", "error", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			s := &Session{
				TokenHash:       hash,
				UserID:          row.UserID,
				UserName:        row.UserName,
				UserEmail:       row.UserEmail,
				IsPlatformAdmin: row.IsPlatformAdmin,
			}
			// Role vem de um LEFT JOIN: é nil se a organização da sessão não
			// existe mais ou se o usuário foi removido dela. Nesse caso tratamos
			// como "sem organização selecionada".
			if row.OrganizationID != nil && row.Role != nil && row.OrganizationStatus != nil {
				s.OrganizationID = row.OrganizationID
				s.Role = *row.Role
				s.OrganizationStatus = *row.OrganizationStatus
			}

			next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), s)))
		})
	}
}
