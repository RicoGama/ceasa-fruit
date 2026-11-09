// Middleware global: roda antes de cada troca de página.
// Decide quem pode ver o quê:
//   /login e /signup -> só para quem NÃO está logado
//   /app/**          -> exige login e (exceto admin/seleção) uma organização ativa
//   /app/admin/**    -> exige administrador da plataforma
export default defineNuxtRouteMiddleware(async (to) => {
  const isApp = to.path === '/app' || to.path.startsWith('/app/')
  const isGuestPage = to.path === '/login' || to.path === '/signup'
  if (!isApp && !isGuestPage) return // site público

  const auth = useAuthStore()
  if (!auth.loaded) await auth.fetchMe()

  if (isGuestPage) {
    return auth.isLoggedIn ? navigateTo('/app') : undefined
  }

  if (!auth.isLoggedIn) {
    return navigateTo({ path: '/login', query: { redirect: to.fullPath } })
  }

  const isAdminPage = to.path.startsWith('/app/admin')
  if (isAdminPage) {
    return auth.isPlatformAdmin ? undefined : navigateTo('/app')
  }

  // Sem organização escolhida, ou com a atual suspensa: vai para a seleção,
  // que explica o que aconteceu e deixa trocar de organização.
  const isSelectPage = to.path === '/app/select-organization'
  if (!isSelectPage && auth.currentOrganization?.status !== 'active') {
    return navigateTo('/app/select-organization')
  }
})
