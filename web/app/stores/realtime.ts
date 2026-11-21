// Tempo real no front: UMA conexão (EventSource em /api/events) para o app
// inteiro, aberta enquanto a pessoa está logada (plugins/realtime.client.ts).
//
// A API manda avisos curtos: { topic: 'suppliers', id: 42 }. Cada tela se
// inscreve nos assuntos que mostra (composables/useRealtime.ts) e recarrega
// o que precisar. Os dados em si vêm sempre pela API (com as permissões).
//
// status: 'connecting' (abrindo), 'live' (recebendo), 'offline' (caiu; o
// navegador tenta de novo sozinho a cada 3s).

export interface RealtimeEvent { topic: string, id?: number }
type Handler = (ev: RealtimeEvent) => void

export const useRealtimeStore = defineStore('realtime', () => {
  const status = ref<'connecting' | 'live' | 'offline'>('connecting')
  const handlers = new Map<string, Set<Handler>>()

  function on(topic: string, fn: Handler) {
    if (!handlers.has(topic)) handlers.set(topic, new Set())
    handlers.get(topic)!.add(fn)
    return () => handlers.get(topic)?.delete(fn)
  }

  // "resync" (a conexão voltou ou a API perdeu avisos): todas as telas
  // recarregam o que estão mostrando, sem saber exatamente o que mudou.
  function dispatch(ev: RealtimeEvent) {
    const targets = ev.topic === 'resync'
      ? [...handlers.values()].flatMap(set => [...set])
      : [...(handlers.get(ev.topic) ?? [])]
    for (const fn of targets) {
      try {
        fn(ev)
      }
      catch (e) {
        console.error('[realtime] handler failed', e)
      }
    }
  }

  return { status, on, dispatch }
})
