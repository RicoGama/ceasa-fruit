<script setup lang="ts">
import { z } from 'zod'
import type { Supplier, SupplierProduct } from '~/types'
import type { PickedProduct } from '~/components/ProductPicker.vue'

// Aba "Produtos" do fornecedor: o que ele fornece, com safra, modalidade
// própria e preferência. Atualiza em tempo real.
const props = defineProps<{ supplier: Supplier }>()
const emit = defineEmits<{ count: [n: number] }>()

const api = useApi()
const toast = useToast()
const auth = useAuthStore()
const canEdit = computed(() => auth.can('suppliers.edit'))

// Sem await: a aba carrega em paralelo, sem segurar a tela do cadastro.
const { data, status, error, refresh } = useAsyncData(`supplier-products-${props.supplier.id}`, async () => {
  const { data, error } = await api.GET('/suppliers/{id}/products', { params: { path: { id: props.supplier.id } } })
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data.items
})
watch(data, items => emit('count', items?.length ?? 0), { immediate: true })

// Tempo real: outra pessoa ligou, alterou ou tirou um produto deste fornecedor.
let timer: ReturnType<typeof setTimeout> | undefined
useRealtime('supplier_products', (ev) => {
  if (ev.topic !== 'resync' && ev.id !== props.supplier.id) return
  clearTimeout(timer)
  timer = setTimeout(() => refresh(), 300)
})

// Modalidade efetiva, para mostrar: a própria do produto ou a do fornecedor.
const supplierMode = computed(() => {
  const mode = purchaseModeLabels[props.supplier.defaultPurchaseMode]
  return props.supplier.defaultPurchaseMode === 'consignment' && props.supplier.consignmentCommission
    ? `${mode} ${formatPercent(props.supplier.consignmentCommission)}`
    : mode
})
function modeText(sp: SupplierProduct) {
  if (!sp.purchaseMode) return `${supplierMode.value} (padrão)`
  const mode = purchaseModeLabels[sp.purchaseMode]
  return sp.consignmentCommission ? `${mode} ${formatPercent(sp.consignmentCommission)}` : mode
}

// --- Modal de adicionar / editar ----------------------------------------------------
type Mode = 'inherit' | 'direct' | 'consignment'
const modalOpen = ref(false)
const editing = ref<SupplierProduct | null>(null)
const saving = ref(false)
const formRef = useTemplateRef('formRef')
const form = reactive({
  product: null as PickedProduct | null,
  supplierProductName: '',
  months: [] as number[],
  mode: 'inherit' as Mode,
  commission: '',
  preferred: false,
  notes: '',
})

const schema = z.object({
  product: z.object({ id: z.number() }, { error: 'Escolha o produto' }).nullable().refine(v => !!v, 'Escolha o produto'),
  commission: z.string(),
}).superRefine((v, ctx) => {
  if (form.mode !== 'consignment') return
  const c = parseDecimalInput(v.commission, 4)
  if (c === null || Number(c) < 0 || Number(c) > 100) {
    ctx.addIssue({ code: 'custom', path: ['commission'], message: 'Informe a comissão (0 a 100%)' })
  }
})

const modeItems = computed(() => [
  { value: 'inherit', label: `Padrão do fornecedor (${supplierMode.value})` },
  { value: 'direct', label: 'Compra direta' },
  { value: 'consignment', label: 'Consignação' },
])
const linkedIds = computed(() => (data.value ?? []).map(sp => sp.productId))

function openAdd() {
  editing.value = null
  Object.assign(form, { product: null, supplierProductName: '', months: [], mode: 'inherit', commission: '', preferred: false, notes: '' })
  modalOpen.value = true
}

function openEdit(sp: SupplierProduct) {
  editing.value = sp
  Object.assign(form, {
    product: { id: sp.productId, displayName: sp.product.displayName },
    supplierProductName: sp.supplierProductName ?? '',
    months: [...sp.months],
    mode: (sp.purchaseMode ?? 'inherit') as Mode,
    commission: sp.consignmentCommission ? formatQuantity(sp.consignmentCommission) : '',
    preferred: sp.preferred,
    notes: sp.notes ?? '',
  })
  modalOpen.value = true
}

function toggleMonth(m: number) {
  form.months = form.months.includes(m) ? form.months.filter(x => x !== m) : [...form.months, m].sort((a, b) => a - b)
}

async function save() {
  saving.value = true
  const fields = {
    supplierProductName: form.supplierProductName.trim() || undefined,
    months: form.months,
    purchaseMode: form.mode === 'inherit' ? undefined : form.mode,
    consignmentCommission: form.mode === 'consignment' ? (parseDecimalInput(form.commission, 4) ?? undefined) : undefined,
    preferred: form.preferred,
    notes: form.notes.trim() || undefined,
  }
  const id = props.supplier.id
  const { error } = editing.value
    ? await api.PUT('/suppliers/{id}/products/{productId}', {
        params: { path: { id, productId: editing.value.productId } },
        body: { ...fields, version: editing.value.version },
      })
    : await api.POST('/suppliers/{id}/products', { params: { path: { id } }, body: { productId: form.product!.id, ...fields } })
  saving.value = false

  if (error) {
    if (isStaleError(error)) {
      modalOpen.value = false
      toast.add({ title: apiErrorMessage(error), description: 'A lista foi atualizada; abra o produto de novo.', color: 'warning' })
      await refresh()
      return
    }
    formRef.value?.setErrors(apiFieldErrors(error).map(e => ({
      ...e, name: e.name === 'productId' ? 'product' : e.name === 'consignmentCommission' ? 'commission' : e.name,
    })))
    toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: editing.value ? 'Produto do fornecedor atualizado' : 'Produto adicionado ao fornecedor', color: 'success' })
  modalOpen.value = false
  await refresh()
}

// --- Remover ------------------------------------------------------------------------
const toRemove = ref<SupplierProduct | null>(null)
const removing = ref(false)
const confirmOpen = computed({
  get: () => toRemove.value !== null,
  set: (v) => { if (!v) toRemove.value = null },
})
async function remove() {
  if (!toRemove.value) return
  removing.value = true
  const { error } = await api.DELETE('/suppliers/{id}/products/{productId}', {
    params: { path: { id: props.supplier.id, productId: toRemove.value.productId } },
  })
  removing.value = false
  if (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: 'Produto removido do fornecedor', color: 'success' })
  toRemove.value = null
  await refresh()
}
</script>

<template>
  <div class="mx-auto w-full max-w-4xl space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <p class="text-sm text-muted">
        O que este fornecedor costuma trazer. Na compra, os produtos dele aparecem primeiro.
      </p>
      <UButton v-if="canEdit" label="Adicionar produto" icon="i-lucide-plus" @click="openAdd" />
    </div>

    <UAlert
      v-if="error"
      color="error"
      variant="subtle"
      icon="i-lucide-circle-alert"
      title="Não foi possível carregar os produtos"
      :description="error.message"
      :actions="[{ label: 'Tentar de novo', color: 'error', variant: 'outline', onClick: () => refresh() }]"
    />

    <UEmpty
      v-else-if="status !== 'pending' && !data?.length"
      icon="i-lucide-apple"
      title="Nenhum produto ligado"
      :description="canEdit ? 'Adicione o que este fornecedor costuma trazer.' : 'Ninguém informou ainda o que este fornecedor traz.'"
      :actions="canEdit ? [{ label: 'Adicionar produto', icon: 'i-lucide-plus', onClick: openAdd }] : []"
    />

    <!-- Uma linha por produto; no celular as informações quebram para baixo -->
    <ul v-else class="divide-y divide-default overflow-hidden rounded-2xl border border-default bg-default">
      <li
        v-for="sp in data ?? []"
        :key="sp.productId"
        class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 p-4 sm:grid-cols-[minmax(0,1fr)_10rem_12rem_auto]"
      >
        <div class="min-w-0">
          <div class="flex items-center gap-1.5">
            <UIcon v-if="sp.preferred" name="i-lucide-star" class="size-4 shrink-0 text-warning" aria-label="Preferencial" />
            <NuxtLink :to="`/app/products/${sp.productId}`" class="truncate font-medium text-highlighted hover:text-primary">
              {{ sp.product.displayName }}
            </NuxtLink>
            <UBadge v-if="!sp.product.active" label="inativo" color="neutral" variant="subtle" size="sm" />
          </div>
          <p v-if="sp.supplierProductName" class="truncate text-xs text-muted">
            Na nota dele: “{{ sp.supplierProductName }}”
          </p>
          <!-- celular: safra e modalidade embaixo do nome -->
          <p class="text-xs text-muted sm:hidden">
            {{ formatMonths(sp.months) }} · {{ modeText(sp) }}
          </p>
        </div>
        <div class="hidden text-sm sm:block">
          <span class="text-muted">Safra</span>
          <p class="text-highlighted">
            {{ formatMonths(sp.months) }}
          </p>
        </div>
        <div class="hidden text-sm sm:block">
          <span class="text-muted">Modalidade</span>
          <p class="truncate text-highlighted">
            {{ modeText(sp) }}
          </p>
        </div>
        <UDropdownMenu
          v-if="canEdit"
          :items="[
            { label: 'Editar', icon: 'i-lucide-pencil', onSelect: () => openEdit(sp) },
            { label: 'Remover do fornecedor', icon: 'i-lucide-unlink', color: 'error', onSelect: () => { toRemove = sp } },
          ]"
          :content="{ align: 'end' }"
        >
          <UButton icon="i-lucide-ellipsis-vertical" color="neutral" variant="ghost" :aria-label="`Ações de ${sp.product.displayName}`" />
        </UDropdownMenu>
      </li>
    </ul>

    <UModal
      v-model:open="modalOpen"
      :title="editing ? editing.product.displayName : 'Adicionar produto'"
      :description="editing ? 'Dados deste produto para este fornecedor.' : `Um produto que ${supplier.tradeName || supplier.legalName} fornece.`"
    >
      <template #body>
        <UForm id="supplier-product-form" ref="formRef" :schema="schema" :state="form" class="space-y-4" @submit="save">
          <UFormField v-if="!editing" label="Produto" name="product" required>
            <ProductPicker v-model="form.product" :exclude="linkedIds" />
          </UFormField>

          <UFormField label="Nome que o fornecedor usa" name="supplierProductName" help="Como aparece na nota ou no romaneio dele (opcional)">
            <UInput v-model="form.supplierProductName" placeholder="Ex.: Tomate saladete" class="w-full" />
          </UFormField>

          <UFormField label="Safra" name="months" :help="form.months.length ? formatMonths(form.months) : 'Nenhum mês marcado = tem o ano todo'">
            <div class="grid grid-cols-6 gap-1.5">
              <UButton
                v-for="(name, i) in monthShort"
                :key="name"
                :label="name"
                size="sm"
                :color="form.months.includes(i + 1) ? 'primary' : 'neutral'"
                :variant="form.months.includes(i + 1) ? 'solid' : 'outline'"
                class="justify-center capitalize"
                :aria-pressed="form.months.includes(i + 1)"
                @click="toggleMonth(i + 1)"
              />
            </div>
          </UFormField>

          <div class="grid gap-4 sm:grid-cols-[1fr_8rem]">
            <UFormField label="Modalidade" name="mode">
              <USelect v-model="form.mode" :items="modeItems" class="w-full" />
            </UFormField>
            <UFormField v-if="form.mode === 'consignment'" label="Comissão" name="commission" required>
              <UInput v-model="form.commission" inputmode="decimal" placeholder="12,5" class="w-full">
                <template #trailing>
                  <span class="text-xs text-muted">%</span>
                </template>
              </UInput>
            </UFormField>
          </div>

          <UFormField name="preferred">
            <USwitch v-model="form.preferred" label="Fornecedor preferencial deste produto" description="Aparece primeiro na hora de comprar" />
          </UFormField>

          <UFormField label="Observações" name="notes">
            <UTextarea v-model="form.notes" :rows="2" placeholder="Ex.: chega terça e sexta" class="w-full" />
          </UFormField>
        </UForm>
      </template>
      <template #footer>
        <div class="flex w-full justify-end gap-2">
          <UButton label="Cancelar" color="neutral" variant="ghost" @click="modalOpen = false" />
          <UButton type="submit" form="supplier-product-form" :label="editing ? 'Salvar' : 'Adicionar'" :loading="saving" />
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="confirmOpen"
      title="Remover este produto do fornecedor?"
      :description="`${toRemove?.product.displayName ?? ''} deixa de aparecer como produto deste fornecedor. O produto continua cadastrado.`"
      :ui="{ footer: 'justify-end' }"
    >
      <template #footer>
        <UButton label="Cancelar" color="neutral" variant="ghost" @click="toRemove = null" />
        <UButton label="Remover" color="error" :loading="removing" @click="remove" />
      </template>
    </UModal>
  </div>
</template>
