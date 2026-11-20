// Middleware global: roda antes de cada troca de página.
// Decide quem pode ver o quê:
//   /login, /forgot-password, /reset-password -> só para quem NÃO está logado
//   /app/**                                    -> exige login
//   páginas com definePageMeta({ permission }) -> exigem a permissão
export default defineNuxtRouteMiddleware(async (to) => {
  const guestPages = ['/login', '/forgot-password', '/reset-password']
  const isGuestPage = guestPages.includes(to.path)
  const isApp = to.path === '/app' || to.path.startsWith('/app/')
  if (!isApp && !isGuestPage) return

  const auth = useAuthStore()
  if (!auth.loaded) await auth.fetchMe()

  if (isGuestPage) {
    return auth.isLoggedIn ? navigateTo('/app') : undefined
  }

  if (!auth.isLoggedIn) {
    return navigateTo({ path: '/login', query: { redirect: to.fullPath } })
  }

  if (to.meta.permission && !auth.can(to.meta.permission)) {
    return navigateTo('/app')
  }
})
