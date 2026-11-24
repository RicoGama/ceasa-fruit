// Idempotência no front: cada ação de gravar (POST/PUT/PATCH/DELETE) leva um
// cabeçalho Idempotency-Key. Se o resultado ficar INCERTO (sem resposta, rede
// caiu, 502/503/504, "ainda processando"), a próxima tentativa do MESMO pedido
// leva a MESMA chave: a API devolve a resposta da primeira vez em vez de
// gravar de novo (ver api/internal/idempotency).
//
// "Mesmo pedido" = mesmo método, endereço e conteúdo. Depois de uma resposta
// definitiva (sucesso ou erro de validação), a chave é esquecida: uma nova
// venda igual, de propósito, é uma venda nova.

const pending = new Map<string, string>() // pedido -> chave
const byKey = new Map<string, string>() // chave -> pedido
const uncertain = new Set<string>() // chaves cuja última tentativa ficou sem resposta definitiva

function newKey(): string {
  // randomUUID só existe em página segura (HTTPS ou localhost); o sistema
  // roda em HTTPS, mas por garantia há o plano B.
  const c: Crypto = globalThis.crypto
  if (typeof c.randomUUID === 'function') return c.randomUUID()
  const bytes = new Uint8Array(16)
  c.getRandomValues(bytes)
  return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')
}

const writeMethods = new Set(['POST', 'PUT', 'PATCH', 'DELETE'])

// Login e logout ficam de fora (a resposta traz o cookie da sessão).
export function needsIdempotencyKey(method: string, url: string) {
  return writeMethods.has(method.toUpperCase()) && !new URL(url, 'http://x').pathname.startsWith('/api/auth/')
}

// Chave para este pedido: a mesma de uma tentativa anterior que ficou sem
// resposta definitiva, ou uma nova.
export async function idempotencyKeyFor(request: Request): Promise<string> {
  const body = request.body ? await request.clone().text() : ''
  const id = `${request.method} ${request.url}\n${body}`
  let key = pending.get(id)
  if (!key) {
    key = newKey()
    pending.set(id, key)
    byKey.set(key, id)
    // Não cresce sem limite (aba aberta o dia inteiro).
    if (pending.size > 200) {
      const [oldId, oldKey] = pending.entries().next().value as [string, string]
      pending.delete(oldId)
      byKey.delete(oldKey)
    }
  }
  return key
}

// Resposta chegou: se for definitiva, a chave é esquecida.
export function settleIdempotencyKey(key: string | null, status: number, inProgress: boolean) {
  if (!key) return
  if (status === 502 || status === 503 || status === 504 || inProgress) {
    uncertain.add(key)
    return
  }
  uncertain.delete(key)
  const id = byKey.get(key)
  if (id) pending.delete(id)
  byKey.delete(key)
}

// Há alguma gravação desta aba sem resultado definitivo? (o tempo real usa
// para não mostrar "outra pessoa alterou" quando foi a própria pessoa)
export function hasUncertainWrite() {
  return uncertain.size > 0
}
