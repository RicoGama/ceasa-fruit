<script setup lang="ts">
import { z } from 'zod'
import type { AuthFormField, FormSubmitEvent } from '@nuxt/ui'

definePageMeta({ layout: 'auth' })
useHead({ title: 'Entrar · CEASA Fruit' })

const auth = useAuthStore()
const route = useRoute()
const errorMessage = ref('')

const fields: AuthFormField[] = [
  { name: 'email', type: 'email', label: 'E-mail', placeholder: 'voce@empresa.com.br', required: true },
  { name: 'password', type: 'password', label: 'Senha', required: true },
]

const schema = z.object({
  email: z.email('E-mail inválido'),
  password: z.string().min(1, 'Informe a senha'),
})
type Schema = z.output<typeof schema>

async function onSubmit(event: FormSubmitEvent<Schema>) {
  errorMessage.value = ''
  try {
    const me = await auth.login(event.data)
    // Com várias organizações e nenhuma escolhida, o middleware manda para a seleção.
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/app'
    await navigateTo(me.currentOrganizationId == null ? '/app/select-organization' : redirect)
  }
  catch (error) {
    errorMessage.value = apiErrorMessage(error)
  }
}
</script>

<template>
  <UAuthForm
    title="Entrar"
    description="Acesse o painel da sua empresa."
    icon="i-lucide-log-in"
    :fields="fields"
    :schema="schema"
    :submit="{ label: 'Entrar', block: true }"
    loading-auto
    @submit="onSubmit"
  >
    <template #validation>
      <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" />
    </template>
    <template #password-hint>
      <ULink to="/forgot-password" class="text-primary font-medium" tabindex="-1">Esqueci minha senha</ULink>
    </template>
    <template #footer>
      Ainda não tem conta?
      <ULink to="/signup" class="text-primary font-medium">Criar conta</ULink>
    </template>
  </UAuthForm>
</template>
