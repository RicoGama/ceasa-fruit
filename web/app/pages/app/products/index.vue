<script setup lang="ts">
import type { DropdownMenuItem, TableColumn } from '@nuxt/ui'
import type { ProductSummary } from '~/types'

definePageMeta({ layout: 'dashboard', permission: 'products.view' })
useHead({ title: 'Produtos · CEASA Fruit' })

const api = useApi()
const toast = useToast()
const auth = useAuthStore()
// Ver exige products.view (página); cadastrar e editar, products.edit.
const canEdit = computed(() => auth.can('products.edit'))

// --- Filtros e paginação ------------------------------------------------------
const limit = 20
const page = ref(1)
const status = ref<'active' | 'inactive' | 'all'>('active')
const statusOptions = [
  { label: 'Ativos', value: 'active' },
  { label: 'Inativos', value: 'inactive' },
  { label: 'Todos', value: 'all' },
]
type Category = keyof typeof productCategoryLabels
const category = ref<Category | 'all'>('all')
const categoryOptions = [{ label: 'Todas as categorias', value: 'all' }, ...toOptions(productCategoryLabels)]

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
watch([status, category], () => { page.value = 1 })

// --- Dados ----------------------------------------------------------------------
const { data, status: loadStatus, error, refresh } = await useAsyncData('products', async () => {
  const { data, error } = await api.GET('/products', {
    params: {
      query: {
        search: debouncedSearch.value || undefined,
        status: status.value,
        category: category.value === 'all' ? undefined : category.value,
        limit,
        offset: (page.value - 1) * limit,
      },
    },
  })
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
}, { watch: [debouncedSearch, status, category, page] })

// Tempo real: alguém cadastrou ou alterou -> a lista recarrega sozinha.
useLiveRefresh('products', refresh)

// --- Tabela -------------------------------------------------------------------
const desktopOnly = { class: { th: 'hidden md:table-cell', td: 'hidden md:table-cell' } }

const columns: TableColumn<ProductSummary>[] = [
  { accessorKey: 'displayName', header: 'Produto' },
  { accessorKey: 'category', header: 'Categoria', meta: desktopOnly },
  { accessorKey: 'stockUnit', header: 'Unidade', meta: desktopOnly },
  { accessorKey: 'active', header: 'Situação' },
  { id: 'actions' },
]

// "Caixa · ≈ 18 kg" (o peso médio aparece quando a unidade não é o próprio kg).
function unitText(p: ProductSummary) {
  const unit = stockUnitLabels[p.stockUnit]
  return p.nominalWeightKg ? `${unit} · ≈ ${formatQuantity(p.nominalWeightKg)} kg` : unit
}

function rowActions(p: ProductSummary): DropdownMenuItem[] {
  return [
    { label: canEdit.value ? 'Abrir cadastro' : 'Ver cadastro', icon: 'i-lucide-pencil', to: `/app/products/${p.id}` },
    ...(canEdit.value && p.active
      ? [{ label: 'Inativar', icon: 'i-lucide-archive', color: 'error' as const, onSelect: () => { toDeactivate.value = p } }]
      : []),
  ]
}

// --- Inativar -----------------------------------------------------------------
const toDeactivate = ref<ProductSummary | null>(null)
const deactivating = ref(false)
const confirmOpen = computed({
  get: () => toDeactivate.value !== null,
  set: (v) => { if (!v) toDeactivate.value = null },
})

async function deactivate() {
  if (!toDeactivate.value) return
  deactivating.value = true
  const { error } = await api.DELETE('/products/{id}', { params: { path: { id: toDeactivate.value.id } } })
  deactivating.value = false
  if (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: 'Produto inativado', color: 'success' })
  toDeactivate.value = null
  await refresh()
}
</script>

<template>
  <UDashboardPanel id="products">
    <template #header>
      <AppNavbar>
        <template #right>
          <UButton v-if="canEdit" icon="i-lucide-plus" to="/app/products/new" aria-label="Novo produto">
            <span class="hidden sm:inline">Novo produto</span>
          </UButton>
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <div class="flex flex-wrap items-center gap-2">
        <UInput
          v-model="search"
          icon="i-lucide-search"
          placeholder="Buscar por nome, variedade, classificação ou código"
          class="w-full sm:max-w-sm"
        />
        <USelect v-model="category" :items="categoryOptions" class="w-48" />
        <USelect v-model="status" :items="statusOptions" class="w-32" />
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
        <template #displayName-cell="{ row }">
          <NuxtLink :to="`/app/products/${row.original.id}`" class="block group">
            <p class="font-medium text-highlighted group-hover:text-primary">
              {{ row.original.displayName }}
            </p>
            <p v-if="row.original.code" class="font-mono text-xs text-muted">
              {{ row.original.code }}
            </p>
            <p class="md:hidden text-xs text-muted">
              {{ unitText(row.original) }}
            </p>
          </NuxtLink>
        </template>
        <template #category-cell="{ row }">
          <UBadge
            :label="productCategoryLabels[row.original.category]"
            :color="productCategoryColors[row.original.category]"
            variant="subtle"
          />
        </template>
        <template #stockUnit-cell="{ row }">
          {{ unitText(row.original) }}
          <UBadge v-if="row.original.sellsByWeight" label="vende a kg" color="neutral" variant="outline" size="sm" class="ms-1" />
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
            icon="i-lucide-apple"
            :title="debouncedSearch ? 'Nenhum produto encontrado' : 'Nenhum produto ainda'"
            :description="debouncedSearch ? 'Tente outra busca.' : 'Cadastre os produtos que você compra e vende.'"
            :actions="debouncedSearch || !canEdit ? [] : [{ label: 'Cadastrar produto', icon: 'i-lucide-plus', to: '/app/products/new' }]"
          />
        </template>
      </UTable>

      <div v-if="(data?.total ?? 0) > limit" class="flex items-center justify-between gap-3 border-t border-default pt-4">
        <p class="text-sm text-muted">
          {{ data?.total }} produtos
        </p>
        <UPagination v-model:page="page" :total="data?.total ?? 0" :items-per-page="limit" />
      </div>
    </template>
  </UDashboardPanel>

  <UModal
    v-model:open="confirmOpen"
    title="Inativar produto?"
    :description="`${toDeactivate?.displayName ?? ''} deixa de aparecer para compra e venda, mas o histórico é mantido. Dá para reativar no cadastro.`"
    :ui="{ footer: 'justify-end' }"
  >
    <template #footer>
      <UButton label="Cancelar" color="neutral" variant="ghost" @click="toDeactivate = null" />
      <UButton label="Inativar" color="error" :loading="deactivating" @click="deactivate" />
    </template>
  </UModal>
</template>
