package auth

import (
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"
)

// SecuritySchemeName é o nome do esquema de autenticação no OpenAPI. Aparece
// no /docs e diz ao cliente gerado que as rotas usam o cookie de sessão.
const SecuritySchemeName = "sessionCookie"

// SecurityScheme descreve o cookie de sessão para o OpenAPI.
var SecurityScheme = &huma.SecurityScheme{
	Type: "apiKey",
	In:   "cookie",
	Name: CookieName,
}

// Os middlewares do Huma têm a forma func(ctx huma.Context, next func(huma.Context)).
// Para deixar passar, chamamos next(ctx); para barrar, escrevemos o erro e
// simplesmente não chamamos next.

// RequireUser exige um usuário logado (qualquer papel).
func RequireUser(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if _, ok := FromContext(ctx.Context()); !ok {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "authentication required")
			return
		}
		next(ctx)
	}
}

// RequireRole exige um usuário logado com um dos papéis informados.
// O administrador sempre passa.
func RequireRole(api huma.API, roles ...string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		s, ok := FromContext(ctx.Context())
		switch {
		case !ok:
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "authentication required")
		case !s.IsAdmin() && !slices.Contains(roles, s.Role):
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "not allowed for your role")
		default:
			next(ctx)
		}
	}
}

// RequireAdminHTTP é a regra "só administrador" como middleware net/http comum,
// para proteger handlers que não são do Huma (ex.: o painel das filas).
func RequireAdminHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := FromContext(r.Context())
		switch {
		case !ok:
			http.Error(w, "authentication required", http.StatusUnauthorized)
		case !s.IsAdmin():
			http.Error(w, "not allowed for your role", http.StatusForbidden)
		default:
			next.ServeHTTP(w, r)
		}
	})
}

// RequireSecurity marca as operações de um grupo como autenticadas no OpenAPI.
func RequireSecurity(op *huma.Operation) {
	op.Security = []map[string][]string{{SecuritySchemeName: {}}}
}
