<script setup lang="ts">
import type { components } from '~/types/api'

definePageMeta({ layout: 'dashboard', permission: 'audit.view' })
useHead({ title: 'Histórico de alterações · CEASA Fruit' })

const api = useApi()
const route = useRoute()

// --- Filtros (na URL: dá para mandar o link "o que o João mudou") ----------------
const days = ref(route.query.days ? Number(route.query.days) : 7)
const actorId = ref(route.query.actorId ? Number(route.query.actorId) : 0)
const entityType = ref<AuditEntityType | 'all'>((route.query.entityType as AuditEntityType) || 'all')

const dayOptions = [
  { label: 'Hoje', value: 1 },
  { label: 'Últimos 7 dias', value: 7 },
  { label: 'Últimos 30 dias', value: 30 },
  { label: 'Últimos 90 dias', value: 90 },
  { label: 'Último ano', value: 366 },
]
const typeOptions = [
  { label: 'Todos os cadastros', value: 'all' },
  ...Object.entries(auditEntityLabels).map(([value, label]) => ({ label, value })),
]
const { data: users } = await useAsyncData('changes-users', async () => (await api.GET('/users')).data?.items ?? [])
const userOptions = computed(() => [
  { label: 'Todas as pessoas', value: 0 },
  ...(users.value ?? []).map(u => ({ label: u.name, value: u.id })),
])

// --- Dados -------------------------------------------------------------------------
const events = ref<components['schemas']['AuditEvent'][]>([])
const refs = ref<Record<string, string>>({})
const nextBefore = ref<number | null>(null)
const loading = ref(false)
const error = ref('')

async function load(more = false) {
  loading.value = true
  error.value = ''
  const { data, error: err } = await api.GET('/audit', {
    params: {
      query: {
        days: days.value,
        actorId: actorId.value || undefined,
        entityType: entityType.value,
        before: more && nextBefore.value ? nextBefore.value : undefined,
      },
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
watch([days, actorId, entityType], () => load(), { immediate: true })

// Tempo real: alguém salvou algo. Espera 1 s (um "salvar" grava várias linhas).
let timer: ReturnType<typeof setTimeout> | undefined
useRealtime('audit_log', () => {
  clearTimeout(timer)
  timer = setTimeout(() => load(), 1000)
})
onScopeDispose(() => clearTimeout(timer))
</script>

<template>
  <UDashboardPanel id="changes">
    <template #header>
      <AppNavbar />
    </template>

    <template #body>
      <p class="text-sm text-muted">
        Todas as alterações de cadastro: quem fez, quando e de quanto para quanto. Cada cadastro também tem o seu botão "Histórico".
      </p>

      <div class="flex flex-wrap items-center gap-2">
        <USelect v-model="actorId" :items="userOptions" class="w-full sm:w-56" />
        <USelect v-model="entityType" :items="typeOptions" class="w-[calc(50%-0.25rem)] sm:w-56" />
        <USelect v-model="days" :items="dayOptions" class="w-[calc(50%-0.25rem)] sm:w-44" />
      </div>

      <UAlert v-if="error" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="error" :actions="[{ label: 'Tentar de novo', color: 'error', variant: 'outline', onClick: () => load() }]" />
      <UEmpty v-else-if="!loading && !events.length" icon="i-lucide-file-pen-line" title="Nenhuma alteração no período" description="Mude o período ou os filtros." />

      <div class="max-w-3xl">
        <AuditEventList :events="events" :refs="refs" show-entity />
      </div>
      <div v-if="nextBefore || loading" class="flex max-w-3xl justify-center pb-6">
        <UButton :loading="loading" label="Carregar mais antigas" color="neutral" variant="outline" size="sm" @click="load(true)" />
      </div>
    </template>
  </UDashboardPanel>
</template>
