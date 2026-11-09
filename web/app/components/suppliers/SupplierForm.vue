<script setup lang="ts">
import { z } from 'zod'
import type { FormSubmitEvent } from '@nuxt/ui'
import type { City, Supplier, SupplierFields } from '~/types'

// Tela completa de cadastro/edição de fornecedor.
// supplier = null -> novo; preenchido -> edição.
const props = defineProps<{ supplier: Supplier | null }>()

const api = useApi()
const toast = useToast()
const form = useTemplateRef('form')
const saving = ref(false)

// --- Estado do formulário ------------------------------------------------------
// Campos de texto usam '' (vazio) na tela; na hora de enviar viram undefined.

interface ContactRow { name: string, role: string, phone: string, isWhatsapp: boolean, email: string }

function emptyState() {
  return {
    personType: 'company' as 'company' | 'individual',
    document: '',
    legalName: '',
    tradeName: '',
    stateRegistration: '',
    taxpayerType: 'non_taxpayer' as SupplierFields['taxpayerType'],
    email: '',
    phone: '',
    whatsapp: '',
    address: { postalCode: '', street: '', number: '', complement: '', district: '', city: null as City | null },
    payment: {
      pixKeyType: undefined as NonNullable<SupplierFields['payment']>['pixKeyType'],
      pixKey: '',
      bank: '',
      bankBranch: '',
      bankAccount: '',
      bankAccountType: undefined as NonNullable<SupplierFields['payment']>['bankAccountType'],
      bankAccountHolder: '',
      bankAccountHolderDocument: '',
    },
    contacts: [] as ContactRow[],
    notes: '',
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
  const v = (x?: string | null) => x ?? ''
  return {
    personType: s.personType,
    document: s.document ? formatDocument(s.document) : '',
    legalName: s.legalName,
    tradeName: v(s.tradeName),
    stateRegistration: v(s.stateRegistration),
    taxpayerType: s.taxpayerType,
    email: v(s.email),
    phone: formatPhone(s.phone),
    whatsapp: formatPhone(s.whatsapp),
    address: {
      postalCode: formatPostalCode(s.address.postalCode),
      street: v(s.address.street),
      number: v(s.address.number),
      complement: v(s.address.complement),
      district: v(s.address.district),
      city: s.address.city,
    },
    payment: {
      pixKeyType: s.payment.pixKeyType ?? undefined,
      pixKey: v(s.payment.pixKey),
      bank: v(s.payment.bank),
      bankBranch: v(s.payment.bankBranch),
      bankAccount: v(s.payment.bankAccount),
      bankAccountType: s.payment.bankAccountType ?? undefined,
      bankAccountHolder: v(s.payment.bankAccountHolder),
      bankAccountHolderDocument: s.payment.bankAccountHolderDocument ? formatDocument(s.payment.bankAccountHolderDocument) : '',
    },
    contacts: s.contacts.map(c => ({
      name: c.name, role: v(c.role), phone: formatPhone(c.phone), isWhatsapp: c.isWhatsapp, email: v(c.email),
    })),
    notes: v(s.notes),
    supplierType: s.supplierType,
    defaultPurchaseMode: s.defaultPurchaseMode,
    consignmentCommission: s.consignmentCommission,
    paymentTermDays: s.paymentTermDays,
    ceasaPavilion: v(s.ceasaPavilion),
    ceasaBox: v(s.ceasaBox),
    status: s.status,
    blockedReason: v(s.blockedReason),
  }
}

const state = reactive<State>(props.supplier ? fromSupplier(props.supplier) : emptyState())

// "Alterações não salvas": compara o estado atual com o último salvo.
const savedSnapshot = ref(JSON.stringify(state))
const dirty = computed(() => JSON.stringify(state) !== savedSnapshot.value)

// Rótulos que mudam com o tipo de pessoa.
const isCompany = computed(() => state.personType === 'company')

// --- Validação (mesmas regras da API, mensagens em português) ---------------

const text = (max: number) => z.string().max(max, `Máximo de ${max} caracteres`)
const phoneField = text(20).refine(v => !v || (onlyDigits(v).length >= 8 && onlyDigits(v).length <= 13), 'Telefone inválido')
const emailField = z.union([z.literal(''), z.email('E-mail inválido')])

const schema = z.object({
  personType: z.enum(['company', 'individual']),
  document: z.string().refine(v => !v || isValidDocument(v), 'CPF ou CNPJ inválido'),
  legalName: z.string().trim().min(2, 'Informe pelo menos 2 caracteres').max(150, 'Máximo de 150 caracteres'),
  tradeName: text(150),
  stateRegistration: text(20),
  taxpayerType: z.string(),
  email: emailField,
  phone: phoneField,
  whatsapp: phoneField,
  address: z.object({
    postalCode: z.string().refine(v => !v || onlyDigits(v).length === 8, 'CEP inválido'),
    street: text(150),
    number: text(20),
    complement: text(100),
    district: text(100),
    city: z.any(),
  }),
  payment: z.object({
    pixKeyType: z.string().optional(),
    pixKey: text(100),
    bank: text(100),
    bankBranch: text(20),
    bankAccount: text(30),
    bankAccountType: z.string().optional(),
    bankAccountHolder: text(150),
    bankAccountHolderDocument: z.string().refine(v => !v || isValidDocument(v), 'CPF ou CNPJ inválido'),
  }),
  contacts: z.array(z.object({
    name: z.string().trim().min(1, 'Informe o nome').max(100),
    role: text(60),
    phone: phoneField,
    isWhatsapp: z.boolean(),
    email: emailField,
  })).max(20, 'Máximo de 20 contatos'),
  notes: text(2000),
  supplierType: z.string(),
  defaultPurchaseMode: z.string(),
  consignmentCommission: z.number().min(0, 'Mínimo 0%').max(100, 'Máximo 100%').nullable(),
  paymentTermDays: z.number().int().min(0, 'Mínimo 0').max(365, 'Máximo de 365 dias'),
  ceasaPavilion: text(30),
  ceasaBox: text(30),
  status: z.string(),
  blockedReason: text(500),
}).superRefine((v, ctx) => {
  // Regras que dependem de mais de um campo.
  const doc = normalizeDocument(v.document)
  if (doc && v.personType === 'company' && doc.length !== 14) {
    ctx.addIssue({ code: 'custom', path: ['document'], message: 'Pessoa jurídica precisa de CNPJ' })
  }
  if (doc && v.personType === 'individual' && doc.length !== 11) {
    ctx.addIssue({ code: 'custom', path: ['document'], message: 'Pessoa física precisa de CPF' })
  }
  if (v.defaultPurchaseMode === 'consignment' && v.consignmentCommission == null) {
    ctx.addIssue({ code: 'custom', path: ['consignmentCommission'], message: 'Informe a comissão da consignação' })
  }
  if (v.status === 'blocked' && !v.blockedReason.trim()) {
    ctx.addIssue({ code: 'custom', path: ['blockedReason'], message: 'Informe o motivo do bloqueio' })
  }
  if (v.payment.pixKeyType && !v.payment.pixKey.trim()) {
    ctx.addIssue({ code: 'custom', path: ['payment', 'pixKey'], message: 'Informe a chave PIX' })
  }
  if (!v.payment.pixKeyType && v.payment.pixKey.trim()) {
    ctx.addIssue({ code: 'custom', path: ['payment', 'pixKeyType'], message: 'Informe o tipo da chave' })
  }
})

// --- CEP: preenche o endereço sozinho ----------------------------------------
const cepLoading = ref(false)
let lastCep = onlyDigits(state.address.postalCode)

watch(() => onlyDigits(state.address.postalCode), async (cep) => {
  if (cep.length !== 8 || cep === lastCep) return
  lastCep = cep
  cepLoading.value = true
  const found = await lookupCep(cep)
  cepLoading.value = false
  if (!found) {
    toast.add({ title: 'CEP não encontrado', description: 'Preencha o endereço manualmente.', color: 'warning' })
    return
  }
  state.address.postalCode = formatPostalCode(cep)
  state.address.street = found.street || state.address.street
  state.address.district = found.district || state.address.district
  state.address.city = found.city
})

// --- Contatos -------------------------------------------------------------------
function addContact() {
  state.contacts.push({ name: '', role: '', phone: '', isWhatsapp: false, email: '' })
}

// --- Envio ----------------------------------------------------------------------

// '' vira undefined: a API trata campo ausente como "não informado" (NULL).
const t = (s?: string | null) => (s?.trim() ? s.trim() : undefined)

function toBody(v: State): SupplierFields {
  return {
    personType: v.personType,
    document: t(v.document),
    legalName: v.legalName.trim(),
    tradeName: t(v.tradeName),
    stateRegistration: t(v.stateRegistration),
    taxpayerType: v.taxpayerType,
    email: t(v.email),
    phone: t(v.phone),
    whatsapp: t(v.whatsapp),
    address: {
      postalCode: t(v.address.postalCode),
      street: t(v.address.street),
      number: t(v.address.number),
      complement: t(v.address.complement),
      district: t(v.address.district),
      cityId: v.address.city?.id,
    },
    payment: {
      pixKeyType: v.payment.pixKeyType || undefined,
      pixKey: t(v.payment.pixKey),
      bank: t(v.payment.bank),
      bankBranch: t(v.payment.bankBranch),
      bankAccount: t(v.payment.bankAccount),
      bankAccountType: v.payment.bankAccountType || undefined,
      bankAccountHolder: t(v.payment.bankAccountHolder),
      bankAccountHolderDocument: t(v.payment.bankAccountHolderDocument),
    },
    contacts: v.contacts.map(c => ({
      name: c.name.trim(), role: t(c.role), phone: t(c.phone), isWhatsapp: c.isWhatsapp, email: t(c.email),
    })),
    notes: t(v.notes),
    supplierType: v.supplierType,
    defaultPurchaseMode: v.defaultPurchaseMode,
    consignmentCommission: v.consignmentCommission ?? undefined,
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
    ? await api.PUT('/suppliers/{id}', { params: { path: { id: props.supplier.id } }, body })
    : await api.POST('/suppliers', { body })
  saving.value = false

  if (result.error) {
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
  Object.assign(state, fromSupplier(saved))
  savedSnapshot.value = JSON.stringify(state)
  toast.add({ title: props.supplier ? 'Fornecedor atualizado' : 'Fornecedor cadastrado', color: 'success' })
  if (!props.supplier) await navigateTo(`/app/suppliers/${saved.id}`)
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

const title = computed(() => props.supplier ? displayName(props.supplier) : 'Novo fornecedor')

const personTypeOptions = toOptions(personTypeLabels)
const supplierTypeOptions = toOptions(supplierTypeLabels)
const purchaseModeOptions = toOptions(purchaseModeLabels)
const statusOptions = toOptions(supplierStatusLabels)
const taxpayerOptions = toOptions(taxpayerTypeLabels)
const pixKeyTypeOptions = toOptions(pixKeyTypeLabels)
const bankAccountTypeOptions = toOptions(bankAccountTypeLabels)
</script>

<template>
  <UDashboardPanel id="supplier-form">
    <template #header>
      <UDashboardNavbar :title="title">
        <template #leading>
          <UButton icon="i-lucide-arrow-left" color="neutral" variant="ghost" to="/app/suppliers" aria-label="Voltar" />
        </template>
        <template #right>
          <UBadge v-if="dirty" label="Não salvo" color="warning" variant="subtle" class="hidden sm:inline-flex" />
          <UButton label="Cancelar" color="neutral" variant="ghost" to="/app/suppliers" class="hidden sm:inline-flex" />
          <UButton type="submit" form="supplier-form" label="Salvar" icon="i-lucide-check" :loading="saving" />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <UForm
        id="supplier-form"
        ref="form"
        :schema="schema"
        :state="state"
        class="flex flex-col gap-6 w-full lg:max-w-4xl mx-auto"
        @submit="onSubmit"
      >
        <!-- Identificação -->
        <UPageCard title="Identificação" description="Quem é o fornecedor. Só o nome é obrigatório." variant="subtle">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <UFormField label="Tipo de pessoa" name="personType">
              <URadioGroup v-model="state.personType" :items="personTypeOptions" orientation="horizontal" />
            </UFormField>
            <UFormField :label="isCompany ? 'CNPJ' : 'CPF'" name="document" :help="isCompany ? undefined : 'Produtor rural pode ficar sem CPF por enquanto'">
              <UInput
                v-model="state.document"
                :placeholder="isCompany ? '00.000.000/0000-00' : '000.000.000-00'"
                class="w-full"
                @blur="state.document = formatDocument(state.document)"
              />
            </UFormField>
            <UFormField :label="isCompany ? 'Razão social' : 'Nome completo'" name="legalName" required>
              <UInput v-model="state.legalName" class="w-full" />
            </UFormField>
            <UFormField :label="isCompany ? 'Nome fantasia' : 'Apelido'" name="tradeName" help="Como ele é conhecido na CEASA">
              <UInput v-model="state.tradeName" :placeholder="isCompany ? '' : 'Ex.: Zé do Tomate'" class="w-full" />
            </UFormField>
            <UFormField label="Tipo de fornecedor" name="supplierType">
              <USelect v-model="state.supplierType" :items="supplierTypeOptions" class="w-full" />
            </UFormField>
            <UFormField label="Situação" name="status">
              <USelect v-model="state.status" :items="statusOptions" class="w-full" />
            </UFormField>
            <UFormField v-if="state.status === 'blocked'" label="Motivo do bloqueio" name="blockedReason" required class="sm:col-span-2">
              <UInput v-model="state.blockedReason" placeholder="Ex.: mercadoria fora do padrão" class="w-full" />
            </UFormField>
          </div>
        </UPageCard>

        <!-- Contato -->
        <UPageCard title="Contato" variant="subtle">
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <UFormField label="Telefone" name="phone">
              <UInput v-model="state.phone" type="tel" placeholder="(21) 99999-0000" class="w-full" @blur="state.phone = formatPhone(state.phone)" />
            </UFormField>
            <UFormField label="WhatsApp" name="whatsapp">
              <UInput v-model="state.whatsapp" type="tel" placeholder="(21) 99999-0000" class="w-full" @blur="state.whatsapp = formatPhone(state.whatsapp)" />
            </UFormField>
            <UFormField label="E-mail" name="email">
              <UInput v-model="state.email" type="email" class="w-full" />
            </UFormField>
          </div>

          <USeparator label="Outros contatos" class="my-2" />

          <div v-for="(contact, i) in state.contacts" :key="i" class="grid grid-cols-1 sm:grid-cols-[1fr_1fr_1fr_auto] gap-3 items-start border-b border-default pb-4">
            <UFormField label="Nome" :name="`contacts.${i}.name`" required>
              <UInput v-model="contact.name" class="w-full" />
            </UFormField>
            <UFormField label="Função" :name="`contacts.${i}.role`">
              <UInput v-model="contact.role" placeholder="Dono, gerente, motorista..." class="w-full" />
            </UFormField>
            <UFormField label="Telefone" :name="`contacts.${i}.phone`">
              <UInput v-model="contact.phone" type="tel" class="w-full" @blur="contact.phone = formatPhone(contact.phone)" />
            </UFormField>
            <div class="flex items-center gap-3 sm:pt-7">
              <UCheckbox v-model="contact.isWhatsapp" label="WhatsApp" />
              <UButton icon="i-lucide-trash-2" color="error" variant="ghost" :aria-label="`Remover contato ${i + 1}`" @click="state.contacts.splice(i, 1)" />
            </div>
          </div>
          <div>
            <UButton label="Adicionar contato" icon="i-lucide-plus" color="neutral" variant="outline" size="sm" @click="addContact" />
          </div>
        </UPageCard>

        <!-- Endereço -->
        <UPageCard title="Endereço" description="Digite o CEP para preencher o resto automaticamente." variant="subtle">
          <div class="grid grid-cols-1 sm:grid-cols-6 gap-4">
            <UFormField label="CEP" name="address.postalCode" class="sm:col-span-2">
              <UInput v-model="state.address.postalCode" placeholder="00000-000" :loading="cepLoading" class="w-full" />
            </UFormField>
            <UFormField label="Município" name="address.city" class="sm:col-span-4">
              <CityInput v-model="state.address.city" />
            </UFormField>
            <UFormField label="Logradouro" name="address.street" class="sm:col-span-4">
              <UInput v-model="state.address.street" class="w-full" />
            </UFormField>
            <UFormField label="Número" name="address.number" class="sm:col-span-2">
              <UInput v-model="state.address.number" placeholder="Ex.: 120 ou s/n" class="w-full" />
            </UFormField>
            <UFormField label="Bairro" name="address.district" class="sm:col-span-3">
              <UInput v-model="state.address.district" class="w-full" />
            </UFormField>
            <UFormField label="Complemento" name="address.complement" class="sm:col-span-3">
              <UInput v-model="state.address.complement" placeholder="Sítio, km, galpão..." class="w-full" />
            </UFormField>
          </div>
        </UPageCard>

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

        <!-- Financeiro -->
        <UPageCard title="Pagamento" description="Para onde você paga este fornecedor." variant="subtle">
          <div class="grid grid-cols-1 sm:grid-cols-[12rem_1fr] gap-4">
            <UFormField label="Tipo de chave PIX" name="payment.pixKeyType">
              <USelect v-model="state.payment.pixKeyType" :items="pixKeyTypeOptions" placeholder="Selecione" class="w-full" />
            </UFormField>
            <UFormField label="Chave PIX" name="payment.pixKey">
              <UInput v-model="state.payment.pixKey" class="w-full" />
            </UFormField>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-4 gap-4">
            <UFormField label="Banco" name="payment.bank" class="sm:col-span-2">
              <UInput v-model="state.payment.bank" placeholder="Ex.: 001 - Banco do Brasil" class="w-full" />
            </UFormField>
            <UFormField label="Agência" name="payment.bankBranch">
              <UInput v-model="state.payment.bankBranch" class="w-full" />
            </UFormField>
            <UFormField label="Conta" name="payment.bankAccount">
              <UInput v-model="state.payment.bankAccount" class="w-full" />
            </UFormField>
            <UFormField label="Tipo de conta" name="payment.bankAccountType" class="sm:col-span-2">
              <USelect v-model="state.payment.bankAccountType" :items="bankAccountTypeOptions" placeholder="Selecione" class="w-full" />
            </UFormField>
            <UFormField label="Titular (se for outra pessoa)" name="payment.bankAccountHolder" class="sm:col-span-2">
              <UInput v-model="state.payment.bankAccountHolder" class="w-full" />
            </UFormField>
            <UFormField label="CPF/CNPJ do titular" name="payment.bankAccountHolderDocument" class="sm:col-span-2">
              <UInput
                v-model="state.payment.bankAccountHolderDocument"
                class="w-full"
                @blur="state.payment.bankAccountHolderDocument = formatDocument(state.payment.bankAccountHolderDocument)"
              />
            </UFormField>
          </div>
        </UPageCard>

        <!-- Fiscal -->
        <UPageCard title="Fiscal" description="Opcional por enquanto. Vai importar quando o sistema emitir notas." variant="subtle">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <UFormField label="Inscrição estadual" name="stateRegistration" help="Ou inscrição de produtor rural; ISENTO se não tiver">
              <UInput v-model="state.stateRegistration" class="w-full" />
            </UFormField>
            <UFormField label="Contribuinte do ICMS" name="taxpayerType">
              <USelect v-model="state.taxpayerType" :items="taxpayerOptions" class="w-full" />
            </UFormField>
          </div>
        </UPageCard>

        <!-- Observações -->
        <UPageCard title="Observações" variant="subtle">
          <UFormField name="notes">
            <UTextarea v-model="state.notes" :rows="4" class="w-full" placeholder="Qualidade, produtos que fornece, época de safra..." />
          </UFormField>
        </UPageCard>

        <div class="flex justify-end gap-2 pb-6">
          <UButton label="Cancelar" color="neutral" variant="ghost" to="/app/suppliers" />
          <UButton type="submit" label="Salvar fornecedor" icon="i-lucide-check" :loading="saving" />
        </div>
      </UForm>
    </template>
  </UDashboardPanel>
</template>
