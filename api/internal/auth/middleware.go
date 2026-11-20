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

// RequireUser exige um usuário logado (qualquer papel). As rotas que precisam
// de mais que isso usam Protect.
func RequireUser(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if _, ok := FromContext(ctx.Context()); !ok {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "authentication required")
			return
		}
		next(ctx)
	}
}

// RequirePermission exige um usuário logado com PELO MENOS UMA das
// permissões informadas. O administrador sempre passa.
func RequirePermission(api huma.API, perms ...Permission) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		s, ok := FromContext(ctx.Context())
		switch {
		case !ok:
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "authentication required")
		case !slices.ContainsFunc(perms, s.Can):
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "not allowed for your role")
		default:
			next(ctx)
		}
	}
}

// Protect amarra permissões a uma rota: barra quem não tem e anota no OpenAPI
// (extensão x-permissions), para o /docs mostrar e o teste conferir que
// nenhuma rota ficou sem permissão.
//
//	huma.Register(api, auth.Protect(api, huma.Operation{...}, auth.ProductsEdit), handler)
func Protect(api huma.API, op huma.Operation, perms ...Permission) huma.Operation {
	op.Middlewares = append(op.Middlewares, RequirePermission(api, perms...))
	if op.Extensions == nil {
		op.Extensions = map[string]any{}
	}
	op.Extensions["x-permissions"] = perms
	return op
}

// RequirePermissionHTTP é o mesmo, como middleware net/http comum, para
// handlers que não são do Huma (ex.: o painel das filas).
func RequirePermissionHTTP(p Permission, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := FromContext(r.Context())
		switch {
		case !ok:
			http.Error(w, "authentication required", http.StatusUnauthorized)
		case !s.Can(p):
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
