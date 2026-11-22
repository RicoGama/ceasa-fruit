<script setup lang="ts">
// Blocos da pessoa que fornecedor e cliente têm iguais: Contato, Endereço,
// Pagamento, Fiscal e Observações. O slot "middle" entra entre Endereço e
// Pagamento (ex.: o bloco Comercial do fornecedor).
//
// O state é o objeto reativo do formulário (o mesmo que o UForm valida):
// este componente altera os campos dele diretamente.
const props = defineProps<{
  state: PartyState
  paymentDescription: string
  notesPlaceholder?: string
}>()

const toast = useToast()

// --- CEP: preenche o endereço sozinho ----------------------------------------
const cepLoading = ref(false)
let lastCep = onlyDigits(props.state.address.postalCode)

watch(() => onlyDigits(props.state.address.postalCode), async (cep) => {
  if (cep.length !== 8 || cep === lastCep) return
  lastCep = cep
  cepLoading.value = true
  const found = await lookupCep(cep)
  cepLoading.value = false
  if (!found) {
    toast.add({ title: 'CEP não encontrado', description: 'Preencha o endereço manualmente.', color: 'warning' })
    return
  }
  const a = props.state.address
  a.postalCode = formatPostalCode(cep)
  a.street = found.street || a.street
  a.district = found.district || a.district
  a.city = found.city
})

function addContact() {
  props.state.contacts.push({ name: '', role: '', phone: '', isWhatsapp: false, email: '' })
}

const taxpayerOptions = toOptions(taxpayerTypeLabels)
const pixKeyTypeOptions = toOptions(pixKeyTypeLabels)
const bankAccountTypeOptions = toOptions(bankAccountTypeLabels)
</script>

<template>
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
        <UInput v-model="contact.role" placeholder="Dono, gerente, comprador..." class="w-full" />
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
        <UInput v-model="state.address.complement" placeholder="Loja, galpão, km..." class="w-full" />
      </UFormField>
    </div>
  </UPageCard>

  <slot name="middle" />

  <!-- Pagamento -->
  <UPageCard title="Pagamento" :description="paymentDescription" variant="subtle">
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
      <UTextarea v-model="state.notes" :rows="4" class="w-full" :placeholder="notesPlaceholder" />
    </UFormField>
  </UPageCard>
</template>
