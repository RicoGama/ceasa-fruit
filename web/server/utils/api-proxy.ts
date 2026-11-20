import type { H3Event } from 'h3'

// Repassa a requisição para a API em Go (mesma origem para o navegador: o
// cookie de sessão funciona sem CORS).
//
// Cuidado com o X-Forwarded-For: a API confia nesse cabeçalho quando a
// requisição vem do Nuxt (mesma máquina), para saber o IP real de quem usa o
// sistema (log e auditoria). Por isso o Nuxt NÃO repassa o cabeçalho que o
// navegador mandou, senão qualquer aparelho na rede do box poderia escrever
// um IP falso nele.
//   - conexão vinda da própria máquina (ex.: um Caddy fazendo HTTPS na
//     frente): mantém o cabeçalho que esse proxy colocou;
//   - conexão vinda da rede: o cabeçalho passa a ser o IP real da conexão.
//
// Em desenvolvimento (npm run dev) o Nuxt recebe as requisições de um
// processo interno, sem endereço de rede (peer vazio), e esse processo já
// repassa o cabeçalho do navegador como veio: lá o IP do log não é confiável.
// Em produção (node .output/server/index.mjs) a conexão é direta e vale a regra acima.
const LOCAL = new Set(['127.0.0.1', '::1', '::ffff:127.0.0.1'])

function clientIP(event: H3Event): string {
  const peer = event.node.req.socket.remoteAddress ?? ''
  if (peer === '' || LOCAL.has(peer)) {
    const forwarded = getRequestHeader(event, 'x-forwarded-for')
    if (forwarded) return forwarded
  }
  return peer.replace(/^::ffff:/, '')
}

export function proxyToApi(event: H3Event, path: string) {
  const { apiUrl } = useRuntimeConfig(event)
  const query = getRequestURL(event).search
  return proxyRequest(event, `${apiUrl}${path}${query}`, {
    headers: { 'x-forwarded-for': clientIP(event) },
  })
}
