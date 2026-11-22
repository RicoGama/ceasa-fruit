<script setup lang="ts">
import { z } from 'zod'
import type { FormSubmitEvent } from '@nuxt/ui'
import type { PartyMatch, Supplier, SupplierFields } from '~/types'

// Tela completa de cadastro/edição de fornecedor.
// supplier = null -> novo; preenchido -> edição.
const props = defineProps<{ supplier: Supplier | null }>()

const api = useApi()
const toast = useToast()
const auth = useAuthStore()
// Sem suppliers.edit, a tela é só para consulta.
const canEdit = computed(() => auth.can('suppliers.edit'))

// Abas (só no fornecedor já cadastrado). A aba fica na URL (?aba=produtos):
// o botão voltar e um link copiado abrem na aba certa.
const route = useRoute()
const router = useRouter()
const tab = computed({
  get: () => (props.supplier && route.query.aba === 'produtos' ? 'produtos' : 'cadastro'),
  set: v => router.replace({ query: { ...route.query, aba: v === 'produtos' ? 'produtos' : undefined } }),
})
const productCount = ref<number>()
const tabs = computed(() => [
  { label: 'Cadastro', value: 'cadastro', icon: 'i-lucide-id-card' },
  { label: 'Produtos', value: 'produtos', icon: 'i-lucide-apple', badge: productCount.value },
])
const form = useTemplateRef('form')
const saving = ref(false)

// --- Estado do formulário ------------------------------------------------------
// Pessoa (comum a fornecedor e cliente: utils/party-form.ts) + campos de fornecedor.

function emptyState() {
  return {
    ...emptyPartyState(),
    supplierType: 'producer' as NonNullable<SupplierFields['supplierType']>,
    defaultPurchaseMode: 'direct' as NonNullable<SupplierFields['defaultPurchaseMode']>,
    consignmentCommission: null as number | null,
    paymentTermDays: 0,
    ceasaPavilion: '',
    ceasaBox: '',
    status: 'active' as NonNullable<SupplierFields['status']>,
    blockedReason: '',
  }
}
type State = ReturnType<typeof emptyState>

// Converte o fornecedor que veio da API para o estado da tela.
function fromSupplier(s: Supplier): State {
  return {
    ...partyStateFrom(s),
    supplierType: s.supplierType,
    defaultPurchaseMode: s.defaultPurchaseMode,
    consignmentCommission: decimalToNumber(s.consignmentCommission),
    paymentTermDays: s.paymentTermDays,
    ceasaPavilion: s.ceasaPavilion ?? '',
    ceasaBox: s.ceasaBox ?? '',
    status: s.status,
    blockedReason: s.blockedReason ?? '',
  }
}

const state = reactive<State>(props.supplier ? fromSupplier(props.supplier) : emptyState())
// Versão do cadastro que esta tela leu (concorrência otimista): vai junto ao
// salvar; se outra pessoa salvou no meio, a API responde 409.
const version = ref(props.supplier?.version ?? 0)

// "Alterações não salvas": compara o estado atual com o último salvo.
const savedSnapshot = ref(JSON.stringify(state))
const dirty = computed(() => JSON.stringify(state) !== savedSnapshot.value)

// Cadastro novo em cima de uma pessoa que já existe (ex.: um cliente com o
// mesmo CNPJ). Os dados dela vêm para a tela e o fornecedor nasce em cima dela.
const reusing = ref<PartyMatch | null>(null)
function useExisting(match: PartyMatch) {
  reusing.value = match
  Object.assign(state, partyStateFrom(match))
}

// --- Validação (mesmas regras da API, mensagens em português) ---------------

const schema = partySchema.extend({
  supplierType: z.string(),
  defaultPurchaseMode: z.string(),
  consignmentCommission: z.number().min(0, 'Mínimo 0%').max(100, 'Máximo 100%').nullable(),
  paymentTermDays: z.number().int().min(0, 'Mínimo 0').max(365, 'Máximo de 365 dias'),
  ceasaPavilion: textField(30),
  ceasaBox: textField(30),
  status: z.string(),
  blockedReason: textField(500),
}).superRefine((v, ctx) => {
  checkPartyRules(v, ctx)
  if (v.defaultPurchaseMode === 'consignment' && v.consignmentCommission == null) {
    ctx.addIssue({ code: 'custom', path: ['consignmentCommission'], message: 'Informe a comissão da consignação' })
  }
  if (v.status === 'blocked' && !v.blockedReason.trim()) {
    ctx.addIssue({ code: 'custom', path: ['blockedReason'], message: 'Informe o motivo do bloqueio' })
  }
})

// --- Envio ----------------------------------------------------------------------

function toBody(v: State): SupplierFields {
  const t = blankToUndefined
  return {
    ...partyBody(v),
    supplierType: v.supplierType,
    defaultPurchaseMode: v.defaultPurchaseMode,
    consignmentCommission: numberToDecimal(v.consignmentCommission, 4),
    paymentTermDays: v.paymentTermDays,
    ceasaPavilion: t(v.ceasaPavilion),
    ceasaBox: t(v.ceasaBox),
    status: v.status,
    blockedReason: v.status === 'blocked' ? t(v.blockedReason) : undefined,
  }
}

async function onSubmit(_event: FormSubmitEvent<z.output<typeof schema>>) {
  saving.value = true
  const body = toBody(state)
  const result = props.supplier
    ? await api.PUT('/suppliers/{id}', { params: { path: { id: props.supplier.id } }, body: { ...body, version: version.value } })
    : await api.POST('/suppliers', {
        body: reusing.value ? { ...body, partyId: reusing.value.id, partyVersion: reusing.value.version } : body,
      })
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
    if (result.response.status === 409) {
      form.value?.setErrors([{ name: 'document', message: apiErrorMessage(result.error) }])
    }
    else {
      form.value?.setErrors(apiFieldErrors(result.error))
    }
    toast.add({ title: apiErrorMessage(result.error), color: 'error' })
    return
  }

  const saved = result.data
  version.value = saved.version
  Object.assign(state, fromSupplier(saved))
  savedSnapshot.value = JSON.stringify(state)
  toast.add({ title: props.supplier ? 'Fornecedor atualizado' : 'Fornecedor cadastrado', color: 'success' })
  if (!props.supplier) await navigateTo(`/app/suppliers/${saved.id}`)
}

// Busca de novo o cadastro (a página recria o formulário com a versão nova).
async function reload() {
  if (!props.supplier) return
  savedSnapshot.value = JSON.stringify(state) // descarta: não pergunta de novo
  await refreshNuxtData(`supplier-${props.supplier.id}`)
}

// Tempo real: outra pessoa alterou ou excluiu este cadastro.
const { remoteChange } = useLiveRecord({
  topic: 'suppliers',
  id: () => props.supplier?.id,
  version: () => version.value,
  dirty: () => dirty.value,
  fetchVersion: async () => {
    if (!props.supplier) return undefined
    const { data, response } = await api.GET('/suppliers/{id}', { params: { path: { id: props.supplier.id } } })
    return response.status === 404 ? undefined : data?.version
  },
  reload,
})

// Avisa antes de sair da tela com alterações não salvas.
useUnsavedChangesGuard(() => dirty.value && !saving.value)

const title = computed(() => props.supplier ? displayName(props.supplier) : 'Novo fornecedor')

const supplierTypeOptions = toOptions(supplierTypeLabels)
const purchaseModeOptions = toOptions(purchaseModeLabels)
const statusOptions = toOptions(supplierStatusLabels)
</script>

<template>
  <UDashboardPanel id="supplier-form">
    <template #header>
      <AppNavbar :title="props.supplier ? title : undefined">
        <template #right>
          <AuditHistoryButton v-if="props.supplier" entity-type="party" :entity-id="props.supplier.id" :title="title" />
          <template v-if="canEdit && tab === 'cadastro'">
            <UBadge v-if="dirty" label="Não salvo" color="warning" variant="subtle" class="hidden sm:inline-flex" />
            <UButton label="Cancelar" color="neutral" variant="ghost" to="/app/suppliers" class="hidden sm:inline-flex" />
            <UButton type="submit" form="supplier-form" label="Salvar" icon="i-lucide-check" :loading="saving" />
          </template>
          <UBadge v-else-if="!canEdit" label="Somente consulta" color="neutral" variant="subtle" />
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <UTabs
        v-if="props.supplier"
        v-model="tab"
        :items="tabs"
        :content="false"
        variant="link"
        class="mx-auto w-full lg:max-w-4xl"
      />

      <UForm
        v-show="tab === 'cadastro'"
        id="supplier-form"
        ref="form"
        :schema="schema"
        :state="state"
        :disabled="!canEdit"
        class="flex flex-col gap-6 w-full lg:max-w-4xl mx-auto"
        @submit="onSubmit"
      >
        <RemoteChangeAlert :kind="remoteChange" back-to="/app/suppliers" @reload="reload" />
        <PartyIdentity
          :state="state"
          role="supplier"
          description="Quem é o fornecedor. Só o nome é obrigatório."
          document-help="Produtor rural pode ficar sem CPF por enquanto"
          :lookup="!props.supplier && canEdit"
          :reusing="reusing"
          :shared-with="props.supplier?.alsoCustomer ? { role: 'customer', id: props.supplier.id } : null"
          :editable="canEdit"
          @use-existing="useExisting"
        >
          <UFormField label="Tipo de fornecedor" name="supplierType">
            <USelect v-model="state.supplierType" :items="supplierTypeOptions" class="w-full" />
          </UFormField>
          <UFormField label="Situação" name="status">
            <USelect v-model="state.status" :items="statusOptions" class="w-full" />
          </UFormField>
          <UFormField v-if="state.status === 'blocked'" label="Motivo do bloqueio" name="blockedReason" required class="sm:col-span-2">
            <UInput v-model="state.blockedReason" placeholder="Ex.: mercadoria fora do padrão" class="w-full" />
          </UFormField>
        </PartyIdentity>

        <PartySections
          :state="state"
          payment-description="Para onde você paga este fornecedor."
          notes-placeholder="Qualidade, produtos que fornece, época de safra..."
        >
          <template #middle>
            <!-- Comercial -->
            <UPageCard title="Comercial" description="Como você compra deste fornecedor. Cada compra pode mudar." variant="subtle">
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <UFormField label="Modalidade padrão" name="defaultPurchaseMode">
                  <URadioGroup v-model="state.defaultPurchaseMode" :items="purchaseModeOptions" orientation="horizontal" />
                </UFormField>
                <UFormField
                  label="Comissão da consignação (%)"
                  name="consignmentCommission"
                  :required="state.defaultPurchaseMode === 'consignment'"
                  help="Percentual que fica com você ao vender"
                >
                  <UInputNumber
                    v-model="state.consignmentCommission"
                    :min="0"
                    :max="100"
                    :step="0.5"
                    :format-options="{ maximumFractionDigits: 2 }"
                    :disabled="state.defaultPurchaseMode !== 'consignment'"
                    class="w-full"
                  />
                </UFormField>
                <UFormField label="Prazo de pagamento padrão (dias)" name="paymentTermDays" help="0 = à vista">
                  <UInputNumber v-model="state.paymentTermDays" :min="0" :max="365" class="w-full" />
                </UFormField>
                <div class="grid grid-cols-2 gap-4">
                  <UFormField label="Pavilhão na CEASA" name="ceasaPavilion">
                    <UInput v-model="state.ceasaPavilion" placeholder="Ex.: 21" class="w-full" />
                  </UFormField>
                  <UFormField label="Box" name="ceasaBox">
                    <UInput v-model="state.ceasaBox" placeholder="Ex.: 14" class="w-full" />
                  </UFormField>
                </div>
              </div>
            </UPageCard>
          </template>
        </PartySections>

        <div v-if="canEdit" class="flex justify-end gap-2 pb-6">
          <UButton label="Cancelar" color="neutral" variant="ghost" to="/app/suppliers" />
          <UButton type="submit" label="Salvar fornecedor" icon="i-lucide-check" :loading="saving" />
        </div>
      </UForm>

      <!-- v-show (e não v-if): carrega junto, para a aba já mostrar a quantidade -->
      <SuppliersSupplierProducts
        v-if="props.supplier"
        v-show="tab === 'produtos'"
        :supplier="props.supplier"
        @count="productCount = $event"
      />
    </template>
  </UDashboardPanel>
</template>
