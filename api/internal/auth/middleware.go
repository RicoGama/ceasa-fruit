package auth

import (
	"net/http"

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

// RequireUser exige um usuário logado (qualquer organização, ou nenhuma).
func RequireUser(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if _, ok := FromContext(ctx.Context()); !ok {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "authentication required")
			return
		}
		next(ctx)
	}
}

// RequireOrganization exige usuário logado COM uma organização ativa selecionada.
// É o middleware de todas as rotas de negócio (fornecedores, clientes, vendas...).
func RequireOrganization(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		s, ok := FromContext(ctx.Context())
		switch {
		case !ok:
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "authentication required")
		case s.OrganizationID == nil:
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "no organization selected")
		case s.OrganizationStatus != "active":
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "organization is suspended")
		default:
			next(ctx)
		}
	}
}

// RequirePlatformAdmin exige o administrador da plataforma (o dono do SaaS).
func RequirePlatformAdmin(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		s, ok := FromContext(ctx.Context())
		switch {
		case !ok:
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "authentication required")
		case !s.IsPlatformAdmin:
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "platform admin only")
		default:
			next(ctx)
		}
	}
}

// RequireSecurity marca as operações de um grupo como autenticadas no OpenAPI.
func RequireSecurity(op *huma.Operation) {
	op.Security = []map[string][]string{{SecuritySchemeName: {}}}
}

// RequirePlatformAdminHTTP é a mesma regra de RequirePlatformAdmin, mas como
// middleware net/http comum, para proteger handlers que não são do Huma
// (ex.: o painel das filas).
func RequirePlatformAdminHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := FromContext(r.Context())
		switch {
		case !ok:
			http.Error(w, "authentication required", http.StatusUnauthorized)
		case !s.IsPlatformAdmin:
			http.Error(w, "platform admin only", http.StatusForbidden)
		default:
			next.ServeHTTP(w, r)
		}
	})
}
