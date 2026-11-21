import type { RealtimeEvent } from '~/stores/realtime'

// Abre a conexão de tempo real quando a pessoa entra e fecha quando sai.
// (".client": só roda no navegador.)
export default defineNuxtPlugin(() => {
  const auth = useAuthStore()
  const realtime = useRealtimeStore()
  let source: EventSource | null = null
  let opened = false
  let retryTimer: ReturnType<typeof setTimeout> | undefined
  let retryDelay = 1000

  function connect() {
    disconnect()
    realtime.status = 'connecting'
    source = new EventSource('/api/events', { withCredentials: true })

    source.onopen = () => {
      realtime.status = 'live'
      retryDelay = 1000
      // Reconectou depois de uma queda: pode ter perdido avisos no meio.
      if (opened) realtime.dispatch({ topic: 'resync' })
      opened = true
    }

    source.onerror = () => {
      realtime.status = 'offline'
      // Se a conexão caiu no meio, o navegador tenta de novo sozinho (retry
      // de 3s mandado pela API). Mas se a RESPOSTA veio com erro (ex.: 502
      // do proxy enquanto a API está fora do ar), o EventSource desiste para
      // sempre (readyState CLOSED). Aí tentamos nós, com espera crescente.
      if (source?.readyState === EventSource.CLOSED) {
        clearTimeout(retryTimer)
        retryTimer = setTimeout(() => {
          if (auth.isLoggedIn) connect()
        }, retryDelay)
        retryDelay = Math.min(retryDelay * 2, 15000)
      }
    }

    source.onmessage = (msg) => {
      let ev: RealtimeEvent
      try {
        ev = JSON.parse(msg.data)
      }
      catch {
        return
      }
      if (ev.topic === 'me') {
        // Mudou o próprio usuário ou o papel dele: relê as permissões e
        // reabre a conexão (a API passa a filtrar pelos novos acessos).
        auth.fetchMe().then(() => auth.isLoggedIn && connect())
        return
      }
      if (ev.topic === 'company') auth.fetchMe() // nome da empresa no menu
      realtime.dispatch(ev)
    }
  }

  function disconnect() {
    clearTimeout(retryTimer)
    source?.close()
    source = null
  }

  watch(() => auth.isLoggedIn, (loggedIn) => {
    if (loggedIn) connect()
    else {
      disconnect()
      opened = false
    }
  }, { immediate: true })
})
