<script setup lang="ts">
import { z } from 'zod'
import type { FormSubmitEvent } from '@nuxt/ui'
import type { City, Company } from '~/types'

definePageMeta({ layout: 'dashboard' })
useHead({ title: 'Dados da empresa · CEASA Fruit' })

const api = useApi()
const toast = useToast()
const auth = useAuthStore()

// Dados da empresa dona deste servidor. Saem nas notas fiscais, boletos e
// documentos impressos, por isso só o admin altera.
const { data, error, refresh } = await useAsyncData('company', async () => {
  const { data, error } = await api.GET('/company')
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
})

function toState(c?: Company | null) {
  return {
    name: c?.name ?? '',
    tradeName: c?.tradeName ?? '',
    document: c?.document ? formatDocument(c.document) : '',
    stateRegistration: c?.stateRegistration ?? '',
    taxRegime: c?.taxRegime as 'simples' | 'presumido' | 'real' | undefined,
    phone: formatPhone(c?.phone),
    email: c?.email ?? '',
    postalCode: formatPostalCode(c?.postalCode),
    street: c?.street ?? '',
    number: c?.number ?? '',
    complement: c?.complement ?? '',
    district: c?.district ?? '',
    city: (c?.city ?? null) as City | null,
    // Versão lida (concorrência otimista): vai junto ao salvar.
    version: c?.version ?? 0,
  }
}
type State = ReturnType<typeof toState>

const state = reactive<State>(toState(data.value))
watch(data, c => Object.assign(state, toState(c)))

const schema = z.object({
  name: z.string().trim().min(2, 'Informe a razão social'),
  document: z.string().refine(v => !v || (onlyDigits(v).length === 14 && isValidDocument(v)), 'CNPJ inválido'),
  email: z.union([z.literal(''), z.email('E-mail inválido')]),
  postalCode: z.string().refine(v => !v || onlyDigits(v).length === 8, 'CEP inválido'),
})

const taxRegimeItems = toOptions(taxRegimeLabels)

// --- CEP: preenche o endereço sozinho ----------------------------------------
const cepLoading = ref(false)
let lastCep = onlyDigits(state.postalCode)
watch(() => onlyDigits(state.postalCode), async (cep) => {
  if (cep.length !== 8 || cep === lastCep) return
  lastCep = cep
  cepLoading.value = true
  const found = await lookupCep(cep)
  cepLoading.value = false
  if (!found) {
    toast.add({ title: 'CEP não encontrado', description: 'Preencha o endereço manualmente.', color: 'warning' })
    return
  }
  state.postalCode = formatPostalCode(cep)
  state.street = found.street || state.street
  state.district = found.district || state.district
  state.city = found.city
})

// --- Salvar ---------------------------------------------------------------------
const formRef = useTemplateRef('formRef')
const saving = ref(false)
// '' vira undefined: a API trata campo ausente como "não informado".
const t = (s?: string | null) => (s?.trim() ? s.trim() : undefined)

async function onSubmit(_: FormSubmitEvent<unknown>) {
  saving.value = true
  const { error } = await api.PUT('/company', {
    body: {
      name: state.name.trim(),
      tradeName: t(state.tradeName),
      document: t(state.document),
      stateRegistration: t(state.stateRegistration),
      taxRegime: state.taxRegime,
      phone: t(state.phone),
      email: t(state.email),
      postalCode: t(state.postalCode),
      street: t(state.street),
      number: t(state.number),
      complement: t(state.complement),
      district: t(state.district),
      cityId: state.city?.id,
      version: state.version,
    },
  })
  saving.value = false
  if (error) {
    if (isStaleError(error)) {
      toast.add({
        title: apiErrorMessage(error),
        description: 'Recarregue para ver os dados atuais. O que você mudou nesta tela será descartado.',
        color: 'warning',
        duration: 0,
        actions: [{ label: 'Recarregar', color: 'warning', variant: 'outline', onClick: () => { refresh() } }],
      })
      return
    }
    const fieldErrors = apiFieldErrors(error)
    if (fieldErrors.length) formRef.value?.setErrors(fieldErrors)
    toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: 'Dados da empresa salvos', color: 'success' })
  await Promise.all([refresh(), auth.fetchMe()]) // o nome aparece no menu lateral
}
</script>

<template>
  <UDashboardPanel id="company">
    <template #header>
      <AppNavbar>
        <template #right>
          <UButton type="submit" form="company-form" label="Salvar" icon="i-lucide-check" :loading="saving" />
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <UAlert
        v-if="error"
        color="error"
        variant="subtle"
        icon="i-lucide-circle-alert"
        title="Não foi possível carregar os dados da empresa"
        :description="error.message"
      />

      <UForm
        v-else
        id="company-form"
        ref="formRef"
        :schema="schema"
        :state="state"
        class="mx-auto w-full max-w-4xl space-y-6"
        @submit="onSubmit"
      >
        <UPageCard title="Identificação" description="Aparece nas notas fiscais, boletos e documentos impressos.">
          <div class="grid gap-4 sm:grid-cols-6">
            <UFormField label="Razão social" name="name" required class="sm:col-span-4">
              <UInput v-model="state.name" class="w-full" />
            </UFormField>
            <UFormField label="Nome fantasia" name="tradeName" class="sm:col-span-2">
              <UInput v-model="state.tradeName" class="w-full" />
            </UFormField>
            <UFormField label="CNPJ" name="document" class="sm:col-span-2">
              <UInput
                v-model="state.document"
                placeholder="00.000.000/0000-00"
                class="w-full font-mono"
                @blur="state.document = formatDocument(state.document)"
              />
            </UFormField>
            <UFormField label="Inscrição estadual" name="stateRegistration" class="sm:col-span-2">
              <UInput v-model="state.stateRegistration" class="w-full" />
            </UFormField>
            <UFormField label="Regime tributário" name="taxRegime" class="sm:col-span-2">
              <USelect v-model="state.taxRegime" :items="taxRegimeItems" placeholder="Selecione" class="w-full" />
            </UFormField>
            <UFormField label="Telefone" name="phone" class="sm:col-span-2">
              <UInput v-model="state.phone" type="tel" class="w-full" @blur="state.phone = formatPhone(state.phone)" />
            </UFormField>
            <UFormField label="E-mail" name="email" class="sm:col-span-4">
              <UInput v-model="state.email" type="email" class="w-full" />
            </UFormField>
          </div>
        </UPageCard>

        <UPageCard title="Endereço">
          <div class="grid gap-4 sm:grid-cols-6">
            <UFormField label="CEP" name="postalCode" class="sm:col-span-2">
              <UInput v-model="state.postalCode" placeholder="00000-000" class="w-full" :loading="cepLoading" />
            </UFormField>
            <UFormField label="Rua" name="street" class="sm:col-span-4">
              <UInput v-model="state.street" class="w-full" />
            </UFormField>
            <UFormField label="Número" name="number" class="sm:col-span-2">
              <UInput v-model="state.number" class="w-full" />
            </UFormField>
            <UFormField label="Complemento" name="complement" class="sm:col-span-4">
              <UInput v-model="state.complement" placeholder="Pavilhão, box..." class="w-full" />
            </UFormField>
            <UFormField label="Bairro" name="district" class="sm:col-span-3">
              <UInput v-model="state.district" class="w-full" />
            </UFormField>
            <UFormField label="Município" name="cityId" class="sm:col-span-3">
              <CityInput v-model="state.city" />
            </UFormField>
          </div>
        </UPageCard>
      </UForm>
    </template>
  </UDashboardPanel>
</template>
