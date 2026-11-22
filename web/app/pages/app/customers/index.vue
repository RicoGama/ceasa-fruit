<script setup lang="ts">
import type { DropdownMenuItem, TableColumn } from '@nuxt/ui'
import type { CustomerSummary } from '~/types'

definePageMeta({ layout: 'dashboard', permission: 'customers.view' })
useHead({ title: 'Clientes · CEASA Fruit' })

const api = useApi()
const toast = useToast()
const auth = useAuthStore()
const canEdit = computed(() => auth.can('customers.edit'))

// --- Filtros e paginação ------------------------------------------------------
const limit = 20
const page = ref(1)
const status = ref<'active' | 'inactive' | 'blocked' | 'all'>('active')
const statusOptions = [
  { label: 'Ativos', value: 'active' },
  { label: 'Inativos', value: 'inactive' },
  { label: 'Bloqueados', value: 'blocked' },
  { label: 'Todos', value: 'all' },
]

// A busca espera o usuário parar de digitar (300 ms) antes de chamar a API.
const search = ref('')
const debouncedSearch = ref('')
let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, (value) => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    debouncedSearch.value = value.trim()
    page.value = 1
  }, 300)
})
const customerType = ref<keyof typeof customerTypeLabels | 'all'>('all')
const customerTypeOptions = [{ label: 'Todos os tipos', value: 'all' }, ...toOptions(customerTypeLabels)]
watch([status, customerType], () => { page.value = 1 })

// --- Dados ----------------------------------------------------------------------
// useAsyncData refaz a chamada sozinho quando algo em `watch` muda.
const { data, status: loadStatus, error, refresh } = await useAsyncData('customers', async () => {
  const { data, error } = await api.GET('/customers', {
    params: {
      query: {
        search: debouncedSearch.value || undefined,
        status: status.value,
        customerType: customerType.value,
        limit,
        offset: (page.value - 1) * limit,
      },
    },
  })
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
}, { watch: [debouncedSearch, status, customerType, page] })

// Tempo real: alguém cadastrou ou alterou -> a lista recarrega sozinha.
useLiveRefresh('customers', refresh)

// --- Tabela -----------------------------------------------------------------
// No celular, documento, tipo e cidade saem da tabela (classes
// "hidden md:table-cell"); o documento passa a aparecer embaixo do nome.
const desktopOnly = { class: { th: 'hidden md:table-cell', td: 'hidden md:table-cell' } }

const columns: TableColumn<CustomerSummary>[] = [
  { accessorKey: 'legalName', header: 'Cliente' },
  { accessorKey: 'document', header: 'CPF/CNPJ', meta: desktopOnly },
  { accessorKey: 'customerType', header: 'Tipo', meta: desktopOnly },
  { id: 'city', header: 'Cidade/UF', meta: desktopOnly },
  { accessorKey: 'status', header: 'Situação' },
  { id: 'actions' },
]

function rowActions(s: CustomerSummary): DropdownMenuItem[] {
  return [
    { label: canEdit.value ? 'Abrir cadastro' : 'Ver cadastro', icon: 'i-lucide-pencil', to: `/app/customers/${s.id}` },
    ...(canEdit.value && s.status === 'active'
      ? [{ label: 'Inativar', icon: 'i-lucide-archive', color: 'error' as const, onSelect: () => { toDeactivate.value = s } }]
      : []),
  ]
}

// --- Inativar -----------------------------------------------------------------
const toDeactivate = ref<CustomerSummary | null>(null)
const deactivating = ref(false)
const confirmOpen = computed({
  get: () => toDeactivate.value !== null,
  set: (v) => { if (!v) toDeactivate.value = null },
})

async function deactivate() {
  if (!toDeactivate.value) return
  deactivating.value = true
  const { error } = await api.DELETE('/customers/{id}', { params: { path: { id: toDeactivate.value.id } } })
  deactivating.value = false
  if (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: 'Cliente inativado', color: 'success' })
  toDeactivate.value = null
  await refresh()
}
</script>

<template>
  <UDashboardPanel id="customers">
    <template #header>
      <AppNavbar>
        <template #right>
          <UButton v-if="canEdit" icon="i-lucide-plus" to="/app/customers/new" aria-label="Novo cliente">
            <span class="hidden sm:inline">Novo cliente</span>
          </UButton>
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <div class="flex flex-wrap items-center gap-2">
        <UInput
          v-model="search"
          icon="i-lucide-search"
          placeholder="Buscar por nome, apelido ou documento"
          class="w-full sm:max-w-sm"
        />
        <USelect v-model="customerType" :items="customerTypeOptions" class="w-[calc(60%-0.25rem)] sm:w-64" />
        <USelect v-model="status" :items="statusOptions" class="w-[calc(40%-0.25rem)] sm:w-36" />
      </div>

      <UAlert
        v-if="error"
        color="error"
        variant="subtle"
        icon="i-lucide-circle-alert"
        title="Não foi possível carregar os dados"
        :description="error.message"
        :actions="[{ label: 'Tentar de novo', color: 'error', variant: 'outline', onClick: () => refresh() }]"
      />

      <UTable
        v-else
        :data="data?.items ?? []"
        :columns="columns"
        :loading="loadStatus === 'pending'"
        class="shrink-0"
      >
        <template #legalName-cell="{ row }">
          <NuxtLink :to="`/app/customers/${row.original.id}`" class="block group">
            <p class="flex flex-wrap items-center gap-x-2 font-medium text-highlighted group-hover:text-primary">
              {{ displayName(row.original) }}
              <UBadge v-if="row.original.alsoSupplier" label="também fornecedor" color="neutral" variant="outline" size="sm" />
            </p>
            <p v-if="row.original.tradeName" class="text-xs text-muted">
              {{ row.original.legalName }}
            </p>
            <p class="md:hidden text-xs text-muted">
              {{ customerTypeLabels[row.original.customerType] }}
              <span v-if="row.original.ceasaBox"> · Box {{ row.original.ceasaBox }}</span>
            </p>
          </NuxtLink>
        </template>
        <template #document-cell="{ row }">
          <span v-if="row.original.document" class="font-mono text-sm">{{ formatDocument(row.original.document) }}</span>
          <span v-else class="text-dimmed">—</span>
        </template>
        <template #city-cell="{ row }">
          {{ row.original.city ? `${row.original.city.name} / ${row.original.city.state}` : '—' }}
        </template>
        <template #customerType-cell="{ row }">
          {{ customerTypeLabels[row.original.customerType] }}
          <p v-if="row.original.ceasaBox" class="text-xs text-muted">
            Box {{ row.original.ceasaBox }}<span v-if="row.original.ceasaPavilion"> · Pavilhão {{ row.original.ceasaPavilion }}</span>
          </p>
        </template>
        <template #status-cell="{ row }">
          <UBadge
            :label="supplierStatusLabels[row.original.status]"
            :color="supplierStatusColors[row.original.status]"
            variant="subtle"
          />
        </template>
        <template #actions-cell="{ row }">
          <div class="text-right">
            <UDropdownMenu :items="rowActions(row.original)" :content="{ align: 'end' }">
              <UButton icon="i-lucide-ellipsis-vertical" color="neutral" variant="ghost" aria-label="Ações" />
            </UDropdownMenu>
          </div>
        </template>

        <template #empty>
          <UEmpty
            icon="i-lucide-users"
            :title="debouncedSearch ? 'Nenhum cliente encontrado' : 'Nenhum cliente ainda'"
            :description="debouncedSearch ? 'Tente outra busca.' : 'Cadastre os sacolões, mercados, restaurantes e feirantes que compram de você.'"
            :actions="debouncedSearch || !canEdit ? [] : [{ label: 'Cadastrar cliente', icon: 'i-lucide-plus', to: '/app/customers/new' }]"
          />
        </template>
      </UTable>

      <div v-if="(data?.total ?? 0) > limit" class="flex items-center justify-between gap-3 border-t border-default pt-4">
        <p class="text-sm text-muted">
          {{ data?.total }} clientes
        </p>
        <UPagination v-model:page="page" :total="data?.total ?? 0" :items-per-page="limit" />
      </div>
    </template>
  </UDashboardPanel>

  <UModal
    v-model:open="confirmOpen"
    title="Inativar cliente?"
    :description="`${toDeactivate ? displayName(toDeactivate) : ''} deixa de aparecer nas listas, mas o histórico é mantido. Dá para reativar no cadastro.`"
    :ui="{ footer: 'justify-end' }"
  >
    <template #footer>
      <UButton label="Cancelar" color="neutral" variant="ghost" @click="toDeactivate = null" />
      <UButton label="Inativar" color="error" :loading="deactivating" @click="deactivate" />
    </template>
  </UModal>
</template>
