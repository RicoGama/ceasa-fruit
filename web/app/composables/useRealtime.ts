import type { RealtimeEvent } from '~/stores/realtime'

// Escuta avisos de um ou mais assuntos enquanto o componente estiver na tela.
//   useRealtime('suppliers', ev => ...)
export function useRealtime(topics: string | string[], handler: (ev: RealtimeEvent) => void) {
  const realtime = useRealtimeStore()
  const offs = ([] as string[]).concat(topics).map(t => realtime.on(t, handler))
  onScopeDispose(() => offs.forEach(off => off()))
}

// Listas: recarrega quando qualquer registro do assunto muda. Vários avisos
// seguidos (ex.: importação) viram uma recarga só (espera 300 ms de calma).
export function useLiveRefresh(topics: string | string[], refresh: () => unknown) {
  let timer: ReturnType<typeof setTimeout> | undefined
  useRealtime(topics, () => {
    clearTimeout(timer)
    timer = setTimeout(() => refresh(), 300)
  })
  onScopeDispose(() => clearTimeout(timer))
}

// Cadastro aberto: quando OUTRA pessoa altera ou exclui este registro.
//   - a tela não tem alterações: recarrega sozinha, sem perguntar;
//   - a tela tem alterações não salvas: não mexe no que a pessoa digitou e
//     mostra um aviso com o botão "Recarregar" (remoteChange).
// fetchVersion busca a versão atual (undefined = não existe mais).
export function useLiveRecord(opts: {
  topic: string
  id: () => number | undefined
  version: () => number
  dirty: () => boolean
  fetchVersion: () => Promise<number | undefined>
  reload: () => unknown
}) {
  const remoteChange = ref<'updated' | 'deleted' | null>(null)
  let remoteVersion = 0

  async function check() {
    if (!opts.id()) return
    const current = await opts.fetchVersion()
    if (current === undefined) {
      remoteChange.value = 'deleted'
      return
    }
    if (current <= opts.version()) return // é a nossa própria gravação
    if (!opts.dirty()) {
      await opts.reload()
      return
    }
    // Uma gravação desta tela ficou sem resposta (Wi-Fi caiu na volta): a
    // mudança que chegou é, quase sempre, ela mesma. Não assusta a pessoa
    // com "outra pessoa alterou"; ao tentar de novo, a API devolve o
    // resultado daquela gravação (idempotência).
    if (hasUncertainWrite()) return
    remoteVersion = current
    remoteChange.value = 'updated'
  }

  // Salvou e ficou com a versão mais nova: o aviso não vale mais.
  watch(opts.version, (v) => {
    if (remoteChange.value === 'updated' && v >= remoteVersion) remoteChange.value = null
  })

  useRealtime(opts.topic, (ev) => {
    if (ev.topic === 'resync' || ev.id === opts.id()) check()
  })
  return { remoteChange }
}
