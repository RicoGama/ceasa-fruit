// Configuração do Nuxt. Documentação: https://nuxt.com/docs/api/nuxt-config
export default defineNuxtConfig({
  modules: ['@nuxt/ui'],

  css: ['~/assets/css/main.css'],

  ui: {
    // Usamos a fonte do sistema; desliga o @nuxt/fonts (que consultaria
    // catálogos de fontes na internet a cada build/dev).
    fonts: false,
  },

  icon: {
    // Embute no JavaScript do app todos os ícones usados no código (procura
    // "i-lucide-..." nos arquivos). Assim nenhum ícone é baixado em tempo de
    // execução — nem da internet, nem do servidor.
    clientBundle: { scan: true },
    // Ícones que não estiverem no pacote vêm do próprio Nuxt (nunca da internet).
    // O caminho padrão seria /api/_nuxt_icon, mas /api/** é da API em Go.
    provider: 'server',
    localApiEndpoint: '/_nuxt_icon',
    fallbackToApi: false,
  },

  devtools: { enabled: true },

  app: {
    head: {
      htmlAttrs: { lang: 'pt-BR' },
      title: 'CEASA Fruit',
    },
  },

  // Renderização por rota:
  // - site público: HTML gerado no build (rápido e bom para SEO)
  // - login e dashboard: SPA, renderizados só no navegador (não precisam de SEO)
  routeRules: {
    '/': { prerender: true },
    '/login': { ssr: false },
    '/signup': { ssr: false },
    '/app/**': { ssr: false },
    // Tudo que começa com /api vai para a API em Go. Como o navegador fala só
    // com o Nuxt (mesma origem), o cookie de sessão funciona sem CORS.
    '/api/**': { proxy: `${process.env.NUXT_API_URL ?? 'http://localhost:8080'}/**` },
  },

  devServer: {
    // O servidor de desenvolvimento só aceita "localhost" por segurança
    // (páginas e a conexão de hot reload). Liberamos o vhost local servido
    // pelo Caddy (ver Caddyfile na raiz do projeto).
    allowedHosts: ['ceasa-fruit.test'],
  },

  compatibilityDate: '2026-11-01',
})
