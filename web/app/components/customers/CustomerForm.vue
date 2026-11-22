<script setup lang="ts">
import { z } from 'zod'
import type { FormSubmitEvent } from '@nuxt/ui'
import type { Customer, CustomerFields, PartyMatch } from '~/types'

// Tela completa de cadastro/edição de cliente (mesmo desenho do fornecedor).
// customer = null -> novo; preenchido -> edição.
//
// Ainda não tem limite de crédito, prazo, tabela de preço nem entrega: isso
// espera as respostas de vendas e financeiro (documento da reunião).
const props = defineProps<{ customer: Customer | null }>()

const api = useApi()
const toast = useToast()
const auth = useAuthStore()
const canEdit = computed(() => auth.can('customers.edit'))
const form = useTemplateRef('form')
const saving = ref(false)

// --- Estado do formulário ------------------------------------------------------
// Pessoa (comum a fornecedor e cliente: utils/party-form.ts) + campos de cliente.

function emptyState() {
  return {
    ...emptyPartyState(),
    customerType: 'retailer' as NonNullable<CustomerFields['customerType']>,
    ceasaPavilion: '',
    ceasaBox: '',
    status: 'active' as NonNullable<CustomerFields['status']>,
    blockedReason: '',
  }
}
type State = ReturnType<typeof emptyState>

function fromCustomer(c: Customer): State {
  return {
    ...partyStateFrom(c),
    customerType: c.customerType,
    ceasaPavilion: c.ceasaPavilion ?? '',
    ceasaBox: c.ceasaBox ?? '',
    status: c.status,
    blockedReason: c.blockedReason ?? '',
  }
}

const state = reactive<State>(props.customer ? fromCustomer(props.customer) : emptyState())
// Versão que esta tela leu: vai junto ao salvar (409 se alguém salvou no meio).
const version = ref(props.customer?.version ?? 0)
const savedSnapshot = ref(JSON.stringify(state))
const dirty = computed(() => JSON.stringify(state) !== savedSnapshot.value)

// Cliente novo em cima de uma pessoa que já existe (ex.: um fornecedor com o mesmo CNPJ).
const reusing = ref<PartyMatch | null>(null)
function useExisting(match: PartyMatch) {
  reusing.value = match
  Object.assign(state, partyStateFrom(match))
}

// --- Validação ------------------------------------------------------------------
const schema = partySchema.extend({
  customerType: z.string(),
  ceasaPavilion: textField(30),
  ceasaBox: textField(30),
  status: z.string(),
  blockedReason: textField(500),
}).superRefine((v, ctx) => {
  checkPartyRules(v, ctx)
  if (v.status === 'blocked' && !v.blockedReason.trim()) {
    ctx.addIssue({ code: 'custom', path: ['blockedReason'], message: 'Informe o motivo do bloqueio' })
  }
})

// --- Envio ----------------------------------------------------------------------
function toBody(v: State): CustomerFields {
  const t = blankToUndefined
  return {
    ...partyBody(v),
    customerType: v.customerType,
    ceasaPavilion: t(v.ceasaPavilion),
    ceasaBox: t(v.ceasaBox),
    status: v.status,
    blockedReason: v.status === 'blocked' ? t(v.blockedReason) : undefined,
  }
}

async function onSubmit(_event: FormSubmitEvent<z.output<typeof schema>>) {
  saving.value = true
  const body = toBody(state)
  const result = props.customer
    ? await api.PUT('/customers/{id}', { params: { path: { id: props.customer.id } }, body: { ...body, version: version.value } })
    : await api.POST('/customers', {
        body: reusing.value ? { ...body, partyId: reusing.value.id, partyVersion: reusing.value.version } : body,
      })
  saving.value = false

  if (result.error) {
    if (isStaleError(result.error)) {
      toast.add({
        title: apiErrorMessage(result.error),
        description: props.customer
          ? 'Recarregue para ver a versão atual. O que você mudou nesta tela será descartado.'
          : 'Alguém alterou o cadastro que você está aproveitando. Saia e comece de novo para ver a versão atual.',
        color: 'warning',
        duration: 0,
        actions: props.customer ? [{ label: 'Recarregar', color: 'warning', variant: 'outline', onClick: reload }] : undefined,
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
  Object.assign(state, fromCustomer(saved))
  savedSnapshot.value = JSON.stringify(state)
  toast.add({ title: props.customer ? 'Cliente atualizado' : 'Cliente cadastrado', color: 'success' })
  if (!props.customer) await navigateTo(`/app/customers/${saved.id}`)
}

async function reload() {
  if (!props.customer) return
  savedSnapshot.value = JSON.stringify(state) // descarta: não pergunta de novo
  await refreshNuxtData(`customer-${props.customer.id}`)
}

// Tempo real: outra pessoa alterou este cadastro (pelo cliente ou pelo fornecedor).
const { remoteChange } = useLiveRecord({
  topic: 'customers',
  id: () => props.customer?.id,
  version: () => version.value,
  dirty: () => dirty.value,
  fetchVersion: async () => {
    if (!props.customer) return undefined
    const { data, response } = await api.GET('/customers/{id}', { params: { path: { id: props.customer.id } } })
    return response.status === 404 ? undefined : data?.version
  },
  reload,
})

useUnsavedChangesGuard(() => dirty.value && !saving.value)

const title = computed(() => props.customer ? displayName(props.customer) : 'Novo cliente')
const customerTypeOptions = toOptions(customerTypeLabels)
const statusOptions = toOptions(supplierStatusLabels)
</script>

<template>
  <UDashboardPanel id="customer-form">
    <template #header>
      <AppNavbar :title="props.customer ? title : undefined">
        <template #right>
          <AuditHistoryButton v-if="props.customer" entity-type="party" :entity-id="props.customer.id" :title="title" />
          <template v-if="canEdit">
            <UBadge v-if="dirty" label="Não salvo" color="warning" variant="subtle" class="hidden sm:inline-flex" />
            <UButton label="Cancelar" color="neutral" variant="ghost" to="/app/customers" class="hidden sm:inline-flex" />
            <UButton type="submit" form="customer-form" label="Salvar" icon="i-lucide-check" :loading="saving" />
          </template>
          <UBadge v-else label="Somente consulta" color="neutral" variant="subtle" />
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <UForm
        id="customer-form"
        ref="form"
        :schema="schema"
        :state="state"
        :disabled="!canEdit"
        class="flex flex-col gap-6 w-full lg:max-w-4xl mx-auto"
        @submit="onSubmit"
      >
        <RemoteChangeAlert :kind="remoteChange" back-to="/app/customers" @reload="reload" />

        <PartyIdentity
          :state="state"
          role="customer"
          description="Quem é o cliente. Só o nome é obrigatório."
          document-help="Feirante e consumidor podem ficar sem CPF"
          :lookup="!props.customer && canEdit"
          :reusing="reusing"
          :shared-with="props.customer?.alsoSupplier ? { role: 'supplier', id: props.customer.id } : null"
          :editable="canEdit"
          @use-existing="useExisting"
        >
          <UFormField label="Tipo de cliente" name="customerType">
            <USelect v-model="state.customerType" :items="customerTypeOptions" class="w-full" />
          </UFormField>
          <UFormField label="Situação" name="status">
            <USelect v-model="state.status" :items="statusOptions" class="w-full" />
          </UFormField>
          <UFormField v-if="state.status === 'blocked'" label="Motivo do bloqueio" name="blockedReason" required class="sm:col-span-2">
            <UInput v-model="state.blockedReason" placeholder="Ex.: cheque devolvido" class="w-full" />
          </UFormField>
          <div class="grid grid-cols-2 gap-4 sm:col-span-2 sm:w-1/2 sm:pr-2">
            <UFormField label="Pavilhão na CEASA" name="ceasaPavilion" help="Se ele tem box">
              <UInput v-model="state.ceasaPavilion" placeholder="Ex.: 21" class="w-full" />
            </UFormField>
            <UFormField label="Box" name="ceasaBox">
              <UInput v-model="state.ceasaBox" placeholder="Ex.: 14" class="w-full" />
            </UFormField>
          </div>
        </PartyIdentity>

        <PartySections
          :state="state"
          payment-description="Dados bancários do cliente, para devoluções e para conferir de quem veio um PIX."
          notes-placeholder="Horário que costuma comprar, preferências, quem pode retirar mercadoria..."
        />

        <div v-if="canEdit" class="flex justify-end gap-2 pb-6">
          <UButton label="Cancelar" color="neutral" variant="ghost" to="/app/customers" />
          <UButton type="submit" label="Salvar cliente" icon="i-lucide-check" :loading="saving" />
        </div>
      </UForm>
    </template>
  </UDashboardPanel>
</template>
