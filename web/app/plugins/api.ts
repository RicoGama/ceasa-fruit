import createClient from 'openapi-fetch'
import type { paths } from '~/types/api'

// Cria UM cliente HTTP para o app inteiro e o disponibiliza como $api.
// openapi-fetch usa os tipos gerados: caminho, parâmetros, corpo e resposta
// de cada chamada são verificados pelo TypeScript.
export default defineNuxtPlugin((nuxtApp) => {
  const api = createClient<paths>({
    baseUrl: '/api', // o Nuxt repassa /api/* para a API em Go (ver nuxt.config.ts)
    credentials: 'include',
    // Sem rede (Wi-Fi do box caiu), o fetch "explode" e a tela ficaria com o
    // botão girando para sempre. Aqui a falha vira uma resposta 503 comum, que
    // toda tela já sabe mostrar ("Sem conexão com o servidor. Tente de novo.").
    fetch: async (request: Request) => {
      try {
        return await globalThis.fetch(request)
      }
      catch {
        return new Response(JSON.stringify({ title: 'Service Unavailable', status: 503, detail: 'no connection to the server' }), {
          status: 503, headers: { 'Content-Type': 'application/problem+json' },
        })
      }
    },
  })

  // Idempotência: toda gravação leva Idempotency-Key; tentar de novo algo que
  // ficou sem resposta reaproveita a chave (ver utils/idempotency.ts).
  api.use({
    async onRequest({ request }) {
      if (!needsIdempotencyKey(request.method, request.url)) return
      request.headers.set('Idempotency-Key', await idempotencyKeyFor(request))
      return request
    },
    onResponse({ request, response }) {
      settleIdempotencyKey(
        request.headers.get('Idempotency-Key'),
        response.status,
        response.status === 409 && response.headers.has('Retry-After'), // ainda processando
      )
    },
  })

  // Middleware do cliente: se a sessão expirar no meio do uso, qualquer
  // chamada recebe 401 e mandamos o usuário para o login.
  api.use({
    onResponse({ request, response }) {
      const isAuthCall = new URL(request.url).pathname.startsWith('/api/auth/')
      if (isAuthCall) return

      // 403: as permissões da pessoa mudaram (o admin mexeu no papel dela).
      // Relê o "quem sou eu" para os menus e botões refletirem na hora.
      if (response.status === 403) {
        nuxtApp.runWithContext(() => useAuthStore().fetchMe())
      }

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
