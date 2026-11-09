<script setup lang="ts">
import { z } from 'zod'
import type { AuthFormField, FormSubmitEvent } from '@nuxt/ui'

definePageMeta({ layout: 'auth' })
useHead({ title: 'Criar conta · CEASA Fruit' })

const auth = useAuthStore()
const errorMessage = ref('')

const fields: AuthFormField[] = [
  { name: 'name', type: 'text', label: 'Seu nome', required: true },
  { name: 'organizationName', type: 'text', label: 'Nome da empresa', placeholder: 'Distribuidora Boa Safra', required: true },
  { name: 'email', type: 'email', label: 'E-mail', placeholder: 'voce@empresa.com.br', required: true },
  { name: 'password', type: 'password', label: 'Senha', description: 'Pelo menos 8 caracteres', required: true },
]

const schema = z.object({
  name: z.string().trim().min(2, 'Informe seu nome'),
  organizationName: z.string().trim().min(2, 'Informe o nome da empresa'),
  email: z.email('E-mail inválido'),
  password: z.string().min(8, 'A senha precisa de pelo menos 8 caracteres').max(128),
})
type Schema = z.output<typeof schema>

async function onSubmit(event: FormSubmitEvent<Schema>) {
  errorMessage.value = ''
  try {
    await auth.signup(event.data)
    await navigateTo('/app')
  }
  catch (error) {
    errorMessage.value = apiErrorMessage(error)
  }
}
</script>

<template>
  <UAuthForm
    title="Criar conta"
    description="Cadastre sua empresa e comece a usar."
    icon="i-lucide-building-2"
    :fields="fields"
    :schema="schema"
    :submit="{ label: 'Criar conta', block: true }"
    loading-auto
    @submit="onSubmit"
  >
    <template #validation>
      <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" />
    </template>
    <template #footer>
      Já tem conta?
      <ULink to="/login" class="text-primary font-medium">Entrar</ULink>
    </template>
  </UAuthForm>
</template>
