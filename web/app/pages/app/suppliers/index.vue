<script setup lang="ts">
import type { DropdownMenuItem, TableColumn } from '@nuxt/ui'
import type { SupplierSummary } from '~/types'

definePageMeta({ layout: 'dashboard' })
useHead({ title: 'Fornecedores · CEASA Fruit' })

const api = useApi()
const toast = useToast()

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
watch(status, () => { page.value = 1 })

// --- Dados ----------------------------------------------------------------------
// useAsyncData refaz a chamada sozinho quando algo em `watch` muda.
const { data, status: loadStatus, error, refresh } = await useAsyncData('suppliers', async () => {
  const { data, error } = await api.GET('/suppliers', {
    params: {
      query: {
        search: debouncedSearch.value || undefined,
        status: status.value,
        limit,
        offset: (page.value - 1) * limit,
      },
    },
  })
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
}, { watch: [debouncedSearch, status, page] })

// --- Tabela -----------------------------------------------------------------
// No celular, documento, cidade e modalidade saem da tabela (classes
// "hidden md:table-cell"); o documento passa a aparecer embaixo do nome.
const desktopOnly = { class: { th: 'hidden md:table-cell', td: 'hidden md:table-cell' } }

const columns: TableColumn<SupplierSummary>[] = [
  { accessorKey: 'legalName', header: 'Fornecedor' },
  { accessorKey: 'document', header: 'CPF/CNPJ', meta: desktopOnly },
  { id: 'city', header: 'Cidade/UF', meta: desktopOnly },
  { accessorKey: 'defaultPurchaseMode', header: 'Modalidade', meta: desktopOnly },
  { accessorKey: 'status', header: 'Situação' },
  { id: 'actions' },
]

function rowActions(s: SupplierSummary): DropdownMenuItem[] {
  return [
    { label: 'Abrir cadastro', icon: 'i-lucide-pencil', to: `/app/suppliers/${s.id}` },
    ...(s.status === 'active'
      ? [{ label: 'Inativar', icon: 'i-lucide-archive', color: 'error' as const, onSelect: () => { toDeactivate.value = s } }]
      : []),
  ]
}

// --- Inativar -----------------------------------------------------------------
const toDeactivate = ref<SupplierSummary | null>(null)
const deactivating = ref(false)
const confirmOpen = computed({
  get: () => toDeactivate.value !== null,
  set: (v) => { if (!v) toDeactivate.value = null },
})

async function deactivate() {
  if (!toDeactivate.value) return
  deactivating.value = true
  const { error } = await api.DELETE('/suppliers/{id}', { params: { path: { id: toDeactivate.value.id } } })
  deactivating.value = false
  if (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: 'Fornecedor inativado', color: 'success' })
  toDeactivate.value = null
  await refresh()
}
</script>

<template>
  <UDashboardPanel id="suppliers">
    <template #header>
      <UDashboardNavbar title="Fornecedores">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
        <template #right>
          <UButton label="Novo fornecedor" icon="i-lucide-plus" to="/app/suppliers/new" />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <div class="flex flex-wrap items-center gap-2">
        <UInput
          v-model="search"
          icon="i-lucide-search"
          placeholder="Buscar por nome, apelido ou documento"
          class="w-full sm:max-w-sm"
        />
        <USelect v-model="status" :items="statusOptions" class="w-36" />
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
          <NuxtLink :to="`/app/suppliers/${row.original.id}`" class="block group">
            <p class="font-medium text-highlighted group-hover:text-primary">
              {{ displayName(row.original) }}
            </p>
            <p v-if="row.original.tradeName" class="text-xs text-muted">
              {{ row.original.legalName }}
            </p>
            <p v-if="row.original.document" class="md:hidden font-mono text-xs text-muted">
              {{ formatDocument(row.original.document) }}
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
        <template #defaultPurchaseMode-cell="{ row }">
          {{ purchaseModeLabels[row.original.defaultPurchaseMode] }}
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
            icon="i-lucide-truck"
            :title="debouncedSearch ? 'Nenhum fornecedor encontrado' : 'Nenhum fornecedor ainda'"
            :description="debouncedSearch ? 'Tente outra busca.' : 'Cadastre os produtores e atacadistas de quem você compra.'"
            :actions="debouncedSearch ? [] : [{ label: 'Cadastrar fornecedor', icon: 'i-lucide-plus', to: '/app/suppliers/new' }]"
          />
        </template>
      </UTable>

      <div v-if="(data?.total ?? 0) > limit" class="flex items-center justify-between gap-3 border-t border-default pt-4">
        <p class="text-sm text-muted">
          {{ data?.total }} fornecedores
        </p>
        <UPagination v-model:page="page" :total="data?.total ?? 0" :items-per-page="limit" />
      </div>
    </template>
  </UDashboardPanel>

  <UModal
    v-model:open="confirmOpen"
    title="Inativar fornecedor?"
    :description="`${toDeactivate ? displayName(toDeactivate) : ''} deixa de aparecer nas listas, mas o histórico é mantido. Dá para reativar no cadastro.`"
    :ui="{ footer: 'justify-end' }"
  >
    <template #footer>
      <UButton label="Cancelar" color="neutral" variant="ghost" @click="toDeactivate = null" />
      <UButton label="Inativar" color="error" :loading="deactivating" @click="deactivate" />
    </template>
  </UModal>
</template>
