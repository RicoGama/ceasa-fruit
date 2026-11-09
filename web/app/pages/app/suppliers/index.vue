<script setup lang="ts">
import type { DropdownMenuItem, TableColumn } from '@nuxt/ui'
import type { Supplier } from '~/types'

definePageMeta({ layout: 'dashboard' })
useHead({ title: 'Fornecedores · CEASA Fruit' })

const api = useApi()
const toast = useToast()

// --- Filtros e paginação ------------------------------------------------------
const limit = 20
const page = ref(1)
const status = ref<'active' | 'inactive' | 'all'>('active')
const statusOptions = [
  { label: 'Ativos', value: 'active' },
  { label: 'Inativos', value: 'inactive' },
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
// No celular, documento, cidade e telefone saem da tabela (classes
// "hidden md:table-cell"); o documento passa a aparecer embaixo do nome.
const desktopOnly = { class: { th: 'hidden md:table-cell', td: 'hidden md:table-cell' } }

const columns: TableColumn<Supplier>[] = [
  { accessorKey: 'name', header: 'Nome' },
  { accessorKey: 'document', header: 'CPF/CNPJ', meta: desktopOnly },
  { id: 'location', header: 'Cidade/UF', meta: desktopOnly },
  { accessorKey: 'phone', header: 'Telefone', meta: desktopOnly },
  { accessorKey: 'active', header: 'Status' },
  { id: 'actions' },
]

function rowActions(s: Supplier): DropdownMenuItem[] {
  return [
    { label: 'Editar', icon: 'i-lucide-pencil', onSelect: () => openForm(s) },
    s.active
      ? { label: 'Inativar', icon: 'i-lucide-archive', color: 'error', onSelect: () => { toDeactivate.value = s } }
      : { label: 'Reativar', icon: 'i-lucide-archive-restore', onSelect: () => reactivate(s) },
  ]
}

// --- Criar / editar --------------------------------------------------------------
const formOpen = ref(false)
const editing = ref<Supplier | null>(null)

function openForm(s: Supplier | null) {
  editing.value = s
  formOpen.value = true
}

// --- Inativar / reativar ---------------------------------------------------------
const toDeactivate = ref<Supplier | null>(null)
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

async function reactivate(s: Supplier) {
  // PUT exige todos os campos: reenviamos o fornecedor com active = true.
  const { error } = await api.PUT('/suppliers/{id}', {
    params: { path: { id: s.id } },
    body: {
      name: s.name,
      document: s.document,
      phone: s.phone ?? undefined,
      email: s.email ?? undefined,
      city: s.city ?? undefined,
      state: s.state ?? undefined,
      notes: s.notes ?? undefined,
      active: true,
    },
  })
  if (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: 'Fornecedor reativado', color: 'success' })
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
          <UButton label="Novo fornecedor" icon="i-lucide-plus" @click="openForm(null)" />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <div class="flex flex-wrap items-center gap-2">
        <UInput
          v-model="search"
          icon="i-lucide-search"
          placeholder="Buscar por nome ou documento"
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
        <template #name-cell="{ row }">
          <p class="font-medium text-highlighted">
            {{ row.original.name }}
          </p>
          <p class="md:hidden font-mono text-xs text-muted">
            {{ formatDocument(row.original.document) }}
          </p>
        </template>
        <template #document-cell="{ row }">
          <span class="font-mono text-sm">{{ formatDocument(row.original.document) }}</span>
        </template>
        <template #location-cell="{ row }">
          {{ [row.original.city, row.original.state].filter(Boolean).join(' / ') || '—' }}
        </template>
        <template #phone-cell="{ row }">
          {{ row.original.phone || '—' }}
        </template>
        <template #active-cell="{ row }">
          <UBadge
            :label="row.original.active ? 'Ativo' : 'Inativo'"
            :color="row.original.active ? 'success' : 'neutral'"
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
            :actions="debouncedSearch ? [] : [{ label: 'Cadastrar fornecedor', icon: 'i-lucide-plus', onClick: () => openForm(null) }]"
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

  <SuppliersSupplierFormModal v-model:open="formOpen" :supplier="editing" @saved="refresh()" />

  <UModal
    v-model:open="confirmOpen"
    title="Inativar fornecedor?"
    :description="`${toDeactivate?.name} deixa de aparecer nas listas, mas o histórico é mantido. Dá para reativar depois.`"
    :ui="{ footer: 'justify-end' }"
  >
    <template #footer>
      <UButton label="Cancelar" color="neutral" variant="ghost" @click="toDeactivate = null" />
      <UButton label="Inativar" color="error" :loading="deactivating" @click="deactivate" />
    </template>
  </UModal>
</template>
