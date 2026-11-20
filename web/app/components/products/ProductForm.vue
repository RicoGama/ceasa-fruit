<script setup lang="ts">
import { z } from 'zod'
import type { Product, ProductFields } from '~/types'

// Tela de cadastro/edição de produto.
// product = null -> novo; preenchido -> edição.
const props = defineProps<{ product: Product | null }>()

const api = useApi()
const toast = useToast()
const auth = useAuthStore()
const form = useTemplateRef('form')
const saving = ref(false)
// Quem não tem products.edit só consulta.
const canEdit = computed(() => auth.can('products.edit'))

type StockUnit = ProductFields['stockUnit']
type Category = ProductFields['category']

// --- Estado do formulário ------------------------------------------------------
// Números decimais ficam como TEXTO no formato brasileiro ("18,5") enquanto a
// pessoa digita; na hora de salvar viram o formato da API ("18.5").
interface PackagingRow { id?: number, name: string, quantity: string }

function emptyState() {
  return {
    name: '',
    variety: '',
    grade: '',
    category: 'vegetable' as Category,
    code: '',
    active: true,
    stockUnit: 'box' as StockUnit,
    nominalWeightKg: '',
    sellsByWeight: false,
    packagings: [] as PackagingRow[],
    ncm: '',
    origin: 0,
    notes: '',
  }
}
type State = ReturnType<typeof emptyState>

const decimal = (v?: string | null) => (v ? formatQuantity(v) : '')

function fromProduct(p: Product): State {
  return {
    name: p.name,
    variety: p.variety ?? '',
    grade: p.grade ?? '',
    category: p.category,
    code: p.code ?? '',
    active: p.active,
    stockUnit: p.stockUnit,
    nominalWeightKg: decimal(p.nominalWeightKg),
    sellsByWeight: p.sellsByWeight,
    packagings: p.packagings.map(k => ({ id: k.id, name: k.name, quantity: decimal(k.quantity) })),
    ncm: formatNcm(p.ncm),
    origin: p.origin,
    notes: p.notes ?? '',
  }
}

const state = reactive<State>(props.product ? fromProduct(props.product) : emptyState())
// Versão que esta tela leu (concorrência otimista): vai junto ao salvar.
const version = ref(props.product?.version ?? 0)

const savedSnapshot = ref(JSON.stringify(state))
const dirty = computed(() => JSON.stringify(state) !== savedSnapshot.value)

const unitShort = computed(() => stockUnitShort[state.stockUnit])
const isKg = computed(() => state.stockUnit === 'kg')
// Nome do produto como vai aparecer nas listas, notas e etiquetas.
const preview = computed(() => [state.name, state.variety, state.grade].map(s => s.trim()).filter(Boolean).join(' '))

// --- Validação --------------------------------------------------------------------
const validDecimal = (v: string) => !v.trim() || (parseDecimalInput(v, 3) !== null && Number(parseDecimalInput(v, 3)) > 0)

const schema = z.object({
  name: z.string().trim().min(2, 'Informe o nome do produto'),
  nominalWeightKg: z.string().refine(validDecimal, 'Peso inválido'),
  ncm: z.string().refine(v => !v || onlyDigits(v).length === 8, 'O NCM tem 8 dígitos'),
  packagings: z.array(z.object({
    name: z.string().trim().min(1, 'Informe o nome'),
    quantity: z.string().refine(validDecimal, 'Quantidade inválida'),
  })),
}).superRefine((v, ctx) => {
  if (v.packagings.length) {
    const names = v.packagings.map(p => p.name.trim().toLowerCase())
    names.forEach((n, i) => {
      if (n && names.indexOf(n) !== i) ctx.addIssue({ code: 'custom', path: ['packagings', i, 'name'], message: 'Nome repetido' })
    })
  }
})

// Vender a quilo precisa do peso médio (a não ser que a unidade já seja o kg).
watch(() => state.stockUnit, (unit) => {
  if (unit === 'kg') state.nominalWeightKg = ''
})

// --- Embalagens -------------------------------------------------------------------
function addPackaging(name = '') {
  state.packagings.push({ name, quantity: '' })
}
const suggestions = computed(() =>
  ['Pilha', 'Pallet', 'Meia pilha'].filter(s => !state.packagings.some(p => p.name.trim().toLowerCase() === s.toLowerCase())),
)

// --- Envio ----------------------------------------------------------------------
const t = (s: string) => (s.trim() ? s.trim() : undefined)

function toBody(v: State): ProductFields {
  return {
    name: v.name.trim(),
    variety: t(v.variety),
    grade: t(v.grade),
    category: v.category,
    code: t(v.code),
    active: v.active,
    stockUnit: v.stockUnit,
    nominalWeightKg: isKg.value ? undefined : (parseDecimalInput(v.nominalWeightKg, 3) ?? undefined),
    sellsByWeight: v.sellsByWeight,
    packagings: v.packagings.map(p => ({
      id: p.id,
      name: p.name.trim(),
      quantity: parseDecimalInput(p.quantity, 3) ?? undefined,
    })),
    ncm: t(v.ncm),
    origin: v.origin,
    notes: t(v.notes),
  }
}

async function onSubmit() {
  if (state.sellsByWeight && !isKg.value && !state.nominalWeightKg.trim()) {
    form.value?.setErrors([{ name: 'nominalWeightKg', message: 'Informe o peso médio para vender a quilo' }])
    return
  }
  saving.value = true
  const body = toBody(state)
  const result = props.product
    ? await api.PUT('/products/{id}', { params: { path: { id: props.product.id } }, body: { ...body, version: version.value } })
    : await api.POST('/products', { body })
  saving.value = false

  if (result.error) {
    if (isStaleError(result.error)) {
      toast.add({
        title: apiErrorMessage(result.error),
        description: 'Recarregue para ver a versão atual. O que você mudou nesta tela será descartado.',
        color: 'warning',
        duration: 0,
        actions: [{ label: 'Recarregar', color: 'warning', variant: 'outline', onClick: reload }],
      })
      return
    }
    form.value?.setErrors(apiFieldErrors(result.error))
    toast.add({ title: apiErrorMessage(result.error), color: 'error' })
    return
  }

  const saved = result.data
  version.value = saved.version
  Object.assign(state, fromProduct(saved))
  savedSnapshot.value = JSON.stringify(state)
  toast.add({ title: props.product ? 'Produto atualizado' : 'Produto cadastrado', color: 'success' })
  if (!props.product) await navigateTo(`/app/products/${saved.id}`)
}

async function reload() {
  if (!props.product) return
  savedSnapshot.value = JSON.stringify(state)
  await refreshNuxtData(`product-${props.product.id}`)
}

// Avisa antes de sair da tela com alterações não salvas.
onBeforeRouteLeave(() => {
  if (dirty.value && !saving.value && !window.confirm('Descartar as alterações não salvas?')) return false
})
function beforeUnload(e: BeforeUnloadEvent) {
  if (dirty.value) e.preventDefault()
}
onMounted(() => window.addEventListener('beforeunload', beforeUnload))
onUnmounted(() => window.removeEventListener('beforeunload', beforeUnload))

const title = computed(() => props.product ? props.product.displayName : 'Novo produto')
const categoryOptions = toOptions(productCategoryLabels)
const stockUnitOptions = toOptions(stockUnitLabels)
const originOptions = Object.entries(productOriginLabels).map(([value, label]) => ({ value: Number(value), label }))
</script>

<template>
  <UDashboardPanel id="product-form">
    <template #header>
      <AppNavbar :title="props.product ? title : undefined">
        <template #right>
          <template v-if="canEdit">
            <UBadge v-if="dirty" label="Não salvo" color="warning" variant="subtle" class="hidden sm:inline-flex" />
            <UButton label="Cancelar" color="neutral" variant="ghost" to="/app/products" class="hidden sm:inline-flex" />
            <UButton type="submit" form="product-form" label="Salvar" icon="i-lucide-check" :loading="saving" />
          </template>
          <UBadge v-else label="Somente consulta" color="neutral" variant="subtle" />
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <UForm
        id="product-form"
        ref="form"
        :schema="schema"
        :state="state"
        :disabled="!canEdit"
        class="mx-auto w-full max-w-4xl space-y-6"
        @submit="onSubmit"
      >
        <!-- Identificação -->
        <UPageCard title="Identificação" description="Cada combinação de nome, variedade e classificação é um produto: o preço muda entre elas." variant="subtle">
          <div class="grid grid-cols-1 sm:grid-cols-6 gap-4">
            <UFormField label="Nome" name="name" required class="sm:col-span-2">
              <UInput v-model="state.name" placeholder="Tomate" class="w-full" autofocus />
            </UFormField>
            <UFormField label="Variedade" name="variety" class="sm:col-span-2">
              <UInput v-model="state.variety" placeholder="Italiano" class="w-full" />
            </UFormField>
            <UFormField label="Classificação" name="grade" class="sm:col-span-2">
              <UInput v-model="state.grade" placeholder="Extra AA" class="w-full" />
            </UFormField>
            <UFormField label="Categoria" name="category" class="sm:col-span-2">
              <USelect v-model="state.category" :items="categoryOptions" class="w-full" />
            </UFormField>
            <UFormField label="Código" name="code" help="Opcional: atalho para digitar no balcão" class="sm:col-span-2">
              <UInput v-model="state.code" placeholder="TOM-ITA" class="w-full font-mono" @update:model-value="state.code = String($event).toUpperCase()" />
            </UFormField>
            <UFormField label="Situação" name="active" class="sm:col-span-2">
              <USwitch v-model="state.active" :label="state.active ? 'Ativo' : 'Inativo'" class="mt-1.5" />
            </UFormField>
          </div>
          <p v-if="preview" class="text-sm text-muted">
            Aparece como: <span class="font-semibold text-highlighted">{{ preview }}</span>
          </p>
        </UPageCard>

        <!-- Unidade e embalagens -->
        <UPageCard title="Unidade e embalagens" description="Como o produto é contado no estoque e agrupado na venda." variant="subtle">
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <UFormField label="Unidade de estoque" name="stockUnit" help="O que se conta no box">
              <USelect v-model="state.stockUnit" :items="stockUnitOptions" class="w-full" />
            </UFormField>
            <UFormField
              v-if="!isKg"
              label="Peso médio da unidade (kg)"
              name="nominalWeightKg"
              :help="`Ex.: ${unitShort} de tomate ≈ 18 kg. Cada lote poderá ter o seu.`"
            >
              <UInput v-model="state.nominalWeightKg" inputmode="decimal" placeholder="18" class="w-full">
                <template #trailing>
                  <span class="text-xs text-muted">kg</span>
                </template>
              </UInput>
            </UFormField>
            <UFormField label="Venda a quilo" name="sellsByWeight" :help="isKg ? 'A unidade já é o quilo' : 'Preço por kg × peso médio'">
              <USwitch v-model="state.sellsByWeight" label="Pode vender a quilo" class="mt-1.5" />
            </UFormField>
          </div>

          <USeparator label="Embalagens" class="my-2" />
          <p class="text-sm text-muted -mt-1">
            Pilha, pallet, caixa de 30... Informe quantas unidades cabem; deixe em branco se variar (a quantidade é informada na venda).
          </p>

          <div v-for="(pack, i) in state.packagings" :key="pack.id ?? `new-${i}`" class="grid grid-cols-[1fr_1fr_auto] gap-3 items-start">
            <UFormField label="Nome" :name="`packagings.${i}.name`" required>
              <UInput v-model="pack.name" placeholder="Pilha" class="w-full" />
            </UFormField>
            <UFormField label="Quantidade" :name="`packagings.${i}.quantity`">
              <UInput v-model="pack.quantity" inputmode="decimal" placeholder="varia" class="w-full">
                <template #trailing>
                  <span class="text-xs text-muted">{{ unitShort }}</span>
                </template>
              </UInput>
            </UFormField>
            <UButton
              v-if="canEdit"
              icon="i-lucide-trash-2"
              color="error"
              variant="ghost"
              class="mt-6"
              :aria-label="`Remover ${pack.name || 'embalagem'}`"
              @click="state.packagings.splice(i, 1)"
            />
          </div>

          <div v-if="canEdit && state.packagings.length < 10" class="flex flex-wrap gap-2">
            <UButton label="Adicionar embalagem" icon="i-lucide-plus" color="neutral" variant="outline" size="sm" @click="addPackaging()" />
            <UButton
              v-for="s in suggestions"
              :key="s"
              :label="s"
              icon="i-lucide-plus"
              color="neutral"
              variant="ghost"
              size="sm"
              @click="addPackaging(s)"
            />
          </div>
        </UPageCard>

        <!-- Fiscal -->
        <UPageCard title="Fiscal" description="Usado na nota fiscal. Confirme os códigos com o contador." variant="subtle">
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <UFormField label="NCM" name="ncm" help="Ex.: tomate 0702.00.00">
              <UInput v-model="state.ncm" placeholder="0000.00.00" class="w-full font-mono" @blur="state.ncm = formatNcm(state.ncm)" />
            </UFormField>
            <UFormField label="Origem da mercadoria" name="origin" class="sm:col-span-2">
              <USelect v-model="state.origin" :items="originOptions" class="w-full" />
            </UFormField>
          </div>
        </UPageCard>

        <!-- Observações -->
        <UPageCard title="Observações" variant="subtle">
          <UFormField name="notes">
            <UTextarea v-model="state.notes" :rows="3" class="w-full" placeholder="Época de safra, cuidados de armazenamento..." />
          </UFormField>
        </UPageCard>

        <div v-if="canEdit" class="flex justify-end gap-2 pb-6">
          <UButton label="Cancelar" color="neutral" variant="ghost" to="/app/products" />
          <UButton type="submit" label="Salvar produto" icon="i-lucide-check" :loading="saving" />
        </div>
      </UForm>
    </template>
  </UDashboardPanel>
</template>
