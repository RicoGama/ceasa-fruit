<script setup lang="ts">
import { z } from 'zod'
import type { AuthFormField, FormSubmitEvent } from '@nuxt/ui'

definePageMeta({ layout: 'auth' })
useHead({ title: 'Esqueci minha senha · CEASA Fruit' })

const api = useApi()
const sent = ref(false)
const errorMessage = ref('')

const fields: AuthFormField[] = [
  { name: 'email', type: 'email', label: 'E-mail', placeholder: 'voce@empresa.com.br', required: true },
]
const schema = z.object({ email: z.email('E-mail inválido') })

async function onSubmit(event: FormSubmitEvent<z.output<typeof schema>>) {
  errorMessage.value = ''
  const { error } = await api.POST('/auth/password/forgot', { body: event.data })
  if (error) {
    errorMessage.value = apiErrorMessage(error)
    return
  }
  // A API responde igual exista o e-mail ou não (para não revelar contas).
  sent.value = true
}
</script>

<template>
  <div v-if="sent" class="space-y-4 text-center">
    <UIcon name="i-lucide-mail-check" class="size-10 text-primary mx-auto" />
    <h1 class="text-xl font-semibold text-highlighted">
      Verifique seu e-mail
    </h1>
    <p class="text-sm text-muted">
      Se houver uma conta com esse e-mail, enviamos um link para criar uma nova senha.
      Ele vale por 1 hora. Confira também a caixa de spam.
    </p>
    <UButton label="Voltar para o login" to="/login" variant="subtle" block />
  </div>

  <UAuthForm
    v-else
    title="Esqueci minha senha"
    description="Informe seu e-mail e enviaremos um link para criar uma nova senha."
    icon="i-lucide-key-round"
    :fields="fields"
    :schema="schema"
    :submit="{ label: 'Enviar link', block: true }"
    loading-auto
    @submit="onSubmit"
  >
    <template #validation>
      <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" />
    </template>
    <template #footer>
      Lembrou?
      <ULink to="/login" class="text-primary font-medium">Entrar</ULink>
    </template>
  </UAuthForm>
</template>
