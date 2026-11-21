<script setup lang="ts">
import type { TableColumn } from '@nuxt/ui'
import type { components } from '~/types/api'

type Entry = components['schemas']['AccessEntry']

definePageMeta({ layout: 'dashboard', permission: 'audit.view' })
useHead({ title: 'Registro de acessos · CEASA Fruit' })

const api = useApi()
const auth = useAuthStore()
const route = useRoute()

// --- Filtros (ficam na URL: dá para mandar o link de "acessos do João") -------
const limit = 50
const page = ref(1)
const days = ref(route.query.days ? Number(route.query.days) : 7)
const kind = ref<'all' | 'session' | 'failed_login'>('all')
const userId = ref<number | undefined>(route.query.userId ? Number(route.query.userId) : undefined)

const dayOptions = [
  { label: 'Hoje', value: 1 },
  { label: 'Últimos 7 dias', value: 7 },
  { label: 'Últimos 30 dias', value: 30 },
  { label: 'Últimos 90 dias', value: 90 },
]
const kindOptions = [
  { label: 'Entradas e falhas', value: 'all' },
  { label: 'Só entradas', value: 'session' },
  { label: 'Só tentativas que falharam', value: 'failed_login' },
]

// Filtro por pessoa: a lista de usuários só aparece para quem pode vê-la.
const { data: users } = useAsyncData('access-users', async () => {
  if (!auth.can('users.manage')) return []
  const { data } = await api.GET('/users')
  return data?.items ?? []
})
const userOptions = computed(() => [
  { label: 'Todas as pessoas', value: 0 },
  ...(users.value ?? []).map(u => ({ label: u.name, value: u.id })),
])
const userFilter = computed({
  get: () => userId.value ?? 0,
  set: (v) => { userId.value = v || undefined },
})
watch([days, kind, userId], () => { page.value = 1 })

// --- Dados ----------------------------------------------------------------------
const { data, status, error, refresh } = await useAsyncData('access-log', async () => {
  const { data, error } = await api.GET('/access-log', {
    params: {
      query: {
        days: days.value,
        kind: kind.value === 'all' ? undefined : kind.value,
        userId: userId.value,
        limit,
        offset: (page.value - 1) * limit,
      },
    },
  })
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
}, { watch: [days, kind, userId, page] })

// Tempo real: alguém entrou ou saiu. E a cada minuto, porque "online agora" e
// "sem atividade" mudam com o tempo, mesmo sem ninguém entrar ou sair.
useLiveRefresh('access_log', refresh)
const ticker = setInterval(() => refresh(), 60_000)
onScopeDispose(() => clearInterval(ticker))

// --- Como mostrar cada linha ------------------------------------------------------
const failedReasons: Record<string, string> = {
  wrong_password: 'Senha errada',
  unknown_email: 'E-mail não cadastrado',
  inactive_user: 'Usuário desativado',
}
const endedReasons: Record<string, string> = {
  logout: 'Saiu do sistema',
  password_changed: 'Senha trocada',
  deactivated: 'Usuário desativado',
}

function situation(e: Entry): { label: string, color: 'success' | 'neutral' | 'error' | 'warning', detail?: string } {
  switch (e.status) {
    case 'online':
      return { label: 'Online agora', color: 'success' }
    case 'idle':
      return { label: 'Sem atividade', color: 'neutral', detail: `desde ${formatWhen(e.lastSeenAt)}` }
    case 'expired':
      return { label: 'Sessão expirou', color: 'neutral', detail: `última atividade ${formatWhen(e.lastSeenAt)}` }
    case 'ended':
      return { label: endedReasons[e.endReason ?? ''] ?? 'Saiu', color: e.endReason === 'logout' ? 'neutral' : 'warning', detail: e.endedAt ? formatWhen(e.endedAt) : undefined }
    default:
      return { label: failedReasons[e.endReason ?? ''] ?? 'Falhou', color: 'error' }
  }
}

function duration(e: Entry) {
  if (e.kind === 'failed_login') return '—'
  return formatDuration(e.startedAt, e.endedAt ?? e.lastSeenAt)
}

const desktopOnly = { class: { th: 'hidden md:table-cell', td: 'hidden md:table-cell' } }
const columns: TableColumn<Entry>[] = [
  { id: 'who', header: 'Pessoa' },
  { accessorKey: 'startedAt', header: 'Entrou', meta: desktopOnly },
  { accessorKey: 'status', header: 'Saiu / situação' },
  { id: 'duration', header: 'Tempo', meta: desktopOnly },
  { id: 'device', header: 'Aparelho', meta: desktopOnly },
]
</script>

<template>
  <UDashboardPanel id="access-log">
    <template #header>
      <AppNavbar />
    </template>

    <template #body>
      <p class="text-sm text-muted">
        Quem entrou no sistema, quando, de qual aparelho e como saiu. Quem só fecha o navegador aparece com a última atividade.
      </p>

      <div class="flex flex-wrap items-center gap-2">
        <USelect v-if="auth.can('users.manage')" v-model="userFilter" :items="userOptions" class="w-full sm:w-56" />
        <USelect v-model="days" :items="dayOptions" class="w-[calc(50%-0.25rem)] sm:w-44" />
        <USelect v-model="kind" :items="kindOptions" class="w-[calc(50%-0.25rem)] sm:w-60" />
      </div>

      <UAlert
        v-if="error"
        color="error"
        variant="subtle"
        icon="i-lucide-circle-alert"
        title="Não foi possível carregar o registro"
        :description="error.message"
        :actions="[{ label: 'Tentar de novo', color: 'error', variant: 'outline', onClick: () => refresh() }]"
      />

      <UTable
        v-else
        :data="data?.items ?? []"
        :columns="columns"
        :loading="status === 'pending' && !data"
        class="shrink-0"
      >
        <template #who-cell="{ row }">
          <div class="min-w-0">
            <p class="truncate font-medium text-highlighted">
              {{ row.original.userName ?? row.original.email }}
            </p>
            <p v-if="row.original.userName" class="truncate text-xs text-muted">
              {{ row.original.email }}
            </p>
            <!-- celular: entrada e aparelho embaixo do nome -->
            <p class="text-xs text-muted md:hidden">
              Entrou {{ formatWhen(row.original.startedAt) }}
            </p>
            <p class="flex items-center gap-1 text-xs text-muted md:hidden">
              <UIcon :name="describeDevice(row.original.userAgent).icon" class="size-3.5" />
              {{ describeDevice(row.original.userAgent).label }}
            </p>
          </div>
        </template>
        <template #startedAt-cell="{ row }">
          <span class="whitespace-nowrap">{{ formatWhen(row.original.startedAt) }}</span>
        </template>
        <template #status-cell="{ row }">
          <div class="flex flex-col items-start gap-0.5">
            <UBadge
              :label="situation(row.original).label"
              :color="situation(row.original).color"
              :icon="row.original.status === 'online' ? 'i-lucide-circle-dot' : undefined"
              variant="subtle"
            />
            <span v-if="situation(row.original).detail" class="text-xs text-muted">
              {{ situation(row.original).detail }}
            </span>
          </div>
        </template>
        <template #duration-cell="{ row }">
          <span class="whitespace-nowrap text-muted">{{ duration(row.original) }}</span>
        </template>
        <template #device-cell="{ row }">
          <div class="flex items-center gap-2">
            <UIcon :name="describeDevice(row.original.userAgent).icon" class="size-4 shrink-0 text-dimmed" />
            <div class="min-w-0">
              <p class="truncate text-sm">
                {{ describeDevice(row.original.userAgent).label }}
              </p>
              <p v-if="row.original.ip" class="font-mono text-xs text-muted">
                {{ row.original.ip }}
              </p>
            </div>
          </div>
        </template>

        <template #empty>
          <UEmpty icon="i-lucide-history" title="Nenhum acesso no período" description="Mude o período ou os filtros." />
        </template>
      </UTable>

      <div v-if="(data?.total ?? 0) > limit" class="flex items-center justify-between gap-3 border-t border-default pt-4">
        <p class="text-sm text-muted">
          {{ data?.total }} registros
        </p>
        <UPagination v-model:page="page" :total="data?.total ?? 0" :items-per-page="limit" />
      </div>
    </template>
  </UDashboardPanel>
</template>
