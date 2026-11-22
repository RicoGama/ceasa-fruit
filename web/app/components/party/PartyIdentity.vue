<script setup lang="ts">
import type { PartyMatch } from '~/types'

// Bloco "Identificação" de fornecedor e cliente: tipo de pessoa, CPF/CNPJ,
// nome e apelido. Os campos do papel (tipo, situação...) entram pelo slot.
//
// Ao cadastrar (lookup = true), confere se o CPF/CNPJ já existe. Na CEASA o
// mesmo CNPJ costuma ser fornecedor E cliente: em vez de duplicar, a tela
// oferece "usar este cadastro" e o novo papel nasce em cima da mesma pessoa.
const props = defineProps<{
  state: PartyState
  role: 'supplier' | 'customer'
  description: string
  // Procura o CPF/CNPJ ao sair do campo (só no cadastro novo).
  lookup?: boolean
  // Pessoa já cadastrada que este formulário está aproveitando.
  reusing?: PartyMatch | null
  documentHelp?: string
  // Cadastro já salvo que também tem o outro papel (cliente que também é
  // fornecedor): avisa que os dados da pessoa são compartilhados.
  sharedWith?: { role: 'supplier' | 'customer', id: number } | null
}>()

const emit = defineEmits<{ 'use-existing': [match: PartyMatch] }>()

const api = useApi()
const auth = useAuthStore()
const isCompany = computed(() => props.state.personType === 'company')
const personTypeOptions = toOptions(personTypeLabels)

const roleNoun = { supplier: 'fornecedor', customer: 'cliente' } as const
const rolePath = { supplier: '/app/suppliers', customer: '/app/customers' } as const

// --- "Este CNPJ já está cadastrado?" -------------------------------------------
const found = ref<PartyMatch | null>(null)
const searching = ref(false)
let lastSearched = ''

async function onDocumentBlur() {
  props.state.document = formatDocument(props.state.document)
  if (!props.lookup || props.reusing) return
  const doc = normalizeDocument(props.state.document)
  if (doc === lastSearched) return
  lastSearched = doc
  found.value = null
  if (!isValidDocument(doc)) return

  searching.value = true
  const { data } = await api.GET('/parties/by-document/{document}', { params: { path: { document: doc } } })
  searching.value = false
  // 404 = CPF/CNPJ livre. Outros erros (sem rede...) não atrapalham o cadastro:
  // se for repetido, a API recusa ao salvar.
  if (data && normalizeDocument(props.state.document) === doc) found.value = data
}

// Já tem este papel (ex.: já é cliente): não dá para cadastrar de novo.
const alreadyHasRole = computed(() => !!found.value
  && (props.role === 'customer' ? found.value.isCustomer : found.value.isSupplier))
// Papel que a pessoa já tem (para o texto do aviso).
const existingRole = computed(() => found.value?.isSupplier ? 'fornecedor' : 'cliente')
const otherRole = computed<'supplier' | 'customer'>(() => props.role === 'customer' ? 'supplier' : 'customer')

function useExisting() {
  if (!found.value) return
  emit('use-existing', found.value)
  found.value = null
}
</script>

<template>
  <UPageCard title="Identificação" :description="description" variant="subtle">
    <!-- Aproveitando um cadastro: deixa claro que os dados são compartilhados -->
    <UAlert
      v-if="reusing"
      color="info"
      variant="subtle"
      icon="i-lucide-link"
      :title="`Usando o cadastro de ${reusing.isSupplier ? 'fornecedor' : 'cliente'} de ${displayName(reusing)}`"
      :description="`Nome, endereço, contatos e pagamento são os mesmos nos dois cadastros: o que você mudar aqui muda lá também. Agora preencha só o que é de ${roleNoun[role]}.`"
    />

    <!-- Cadastro salvo que também é do outro papel -->
    <UAlert
      v-else-if="sharedWith"
      color="neutral"
      variant="subtle"
      icon="i-lucide-link"
      :title="`Também é ${roleNoun[sharedWith.role]}`"
      :description="`Nome, endereço, contatos e pagamento são os mesmos no cadastro de ${roleNoun[sharedWith.role]}: o que você mudar aqui muda lá também.`"
      :actions="auth.can(sharedWith.role === 'supplier' ? 'suppliers.view' : 'customers.view')
        ? [{ label: `Abrir o ${roleNoun[sharedWith.role]}`, color: 'neutral', variant: 'outline', to: `${rolePath[sharedWith.role]}/${sharedWith.id}` }]
        : []"
    />

    <!-- CPF/CNPJ já cadastrado -->
    <UAlert
      v-else-if="found && alreadyHasRole"
      color="warning"
      variant="subtle"
      icon="i-lucide-circle-alert"
      :title="`Este ${isCompany ? 'CNPJ' : 'CPF'} já é ${roleNoun[role]}: ${displayName(found)}`"
      description="Abra o cadastro que já existe em vez de criar outro."
      :actions="[{ label: 'Abrir cadastro', color: 'warning', variant: 'outline', to: `${rolePath[role]}/${found.id}` }]"
    />
    <UAlert
      v-else-if="found"
      color="info"
      variant="subtle"
      icon="i-lucide-user-round-search"
      :title="`Este ${isCompany ? 'CNPJ' : 'CPF'} já está cadastrado como ${existingRole}: ${displayName(found)}`"
      :description="`Use o mesmo cadastro para não repetir nome, endereço e contatos. Ele passa a ser ${existingRole} e ${roleNoun[role]}.`"
      :actions="[
        { label: 'Usar este cadastro', color: 'info', icon: 'i-lucide-link', onClick: useExisting },
        { label: `Ver o ${existingRole}`, color: 'info', variant: 'ghost', to: `${rolePath[otherRole]}/${found.id}`, target: '_blank' },
      ]"
    />

    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
      <UFormField label="Tipo de pessoa" name="personType">
        <URadioGroup v-model="state.personType" :items="personTypeOptions" orientation="horizontal" :disabled="!!reusing" />
      </UFormField>
      <UFormField :label="isCompany ? 'CNPJ' : 'CPF'" name="document" :help="isCompany ? undefined : documentHelp">
        <UInput
          v-model="state.document"
          :placeholder="isCompany ? '00.000.000/0000-00' : '000.000.000-00'"
          :loading="searching"
          :disabled="!!reusing"
          class="w-full"
          @blur="onDocumentBlur"
        />
      </UFormField>
      <UFormField :label="isCompany ? 'Razão social' : 'Nome completo'" name="legalName" required>
        <UInput v-model="state.legalName" class="w-full" />
      </UFormField>
      <UFormField :label="isCompany ? 'Nome fantasia' : 'Apelido'" name="tradeName" help="Como ele é conhecido na CEASA">
        <UInput v-model="state.tradeName" :placeholder="isCompany ? '' : 'Ex.: Zé do Tomate'" class="w-full" />
      </UFormField>
      <slot />
    </div>
  </UPageCard>
</template>
