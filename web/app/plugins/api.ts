import createClient from 'openapi-fetch'
import type { paths } from '~/types/api'

// Cria UM cliente HTTP para o app inteiro e o disponibiliza como $api.
// openapi-fetch usa os tipos gerados: caminho, parâmetros, corpo e resposta
// de cada chamada são verificados pelo TypeScript.
export default defineNuxtPlugin((nuxtApp) => {
  const api = createClient<paths>({
    baseUrl: '/api', // o Nuxt repassa /api/* para a API em Go (ver nuxt.config.ts)
    credentials: 'include',
  })

  // Middleware do cliente: se a sessão expirar no meio do uso, qualquer
  // chamada recebe 401 e mandamos o usuário para o login.
  api.use({
    onResponse({ request, response }) {
      const isAuthCall = new URL(request.url).pathname.startsWith('/api/auth/')
      if (isAuthCall) return

      if (response.status === 401) {
        nuxtApp.runWithContext(() => {
          useAuthStore().reset()
          navigateTo('/login')
        })
      }

    },
  })

  return { provide: { api } }
})
