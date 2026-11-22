<script setup lang="ts">
import type { components } from '~/types/api'

// Histórico de um cadastro: quem mudou o quê e quando. Atualiza sozinho
// quando alguém salva (tempo real) e carrega mais ao rolar até o fim.
const props = defineProps<{
  entityType: AuditEntityType
  entityId: number
}>()

const api = useApi()
const events = ref<components['schemas']['AuditEvent'][]>([])
const refs = ref<Record<string, string>>({})
const nextBefore = ref<number | null>(null)
const loading = ref(false)
const error = ref('')

async function load(more = false) {
  loading.value = true
  error.value = ''
  const { data, error: err } = await api.GET('/audit/{entityType}/{entityId}', {
    params: {
      path: { entityType: props.entityType, entityId: props.entityId },
      query: { before: more && nextBefore.value ? nextBefore.value : undefined },
    },
  })
  loading.value = false
  if (err) {
    error.value = apiErrorMessage(err)
    return
  }
  events.value = more ? [...events.value, ...data.events] : data.events
  refs.value = { ...refs.value, ...data.refs }
  nextBefore.value = data.nextBefore ?? null
}

watch(() => [props.entityType, props.entityId], () => load(), { immediate: true })
// Alguém salvou este cadastro: recarrega o começo da lista.
useRealtime('audit_log', (ev) => {
  if (ev.topic === 'resync' || ev.id === props.entityId) load()
})
</script>

<template>
  <div class="space-y-4">
    <UAlert v-if="error" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="error" />
    <p v-else-if="!loading && !events.length" class="text-sm text-muted">
      Nenhuma alteração registrada ainda.
    </p>
    <AuditEventList :events="events" :refs="refs" />
    <div v-if="nextBefore || loading" class="flex justify-center">
      <UButton :loading="loading" label="Carregar mais antigas" color="neutral" variant="outline" size="sm" @click="load(true)" />
    </div>
  </div>
</template>
