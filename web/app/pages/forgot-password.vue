<script setup lang="ts">
import { z } from 'zod'
import type { FormSubmitEvent } from '@nuxt/ui'

definePageMeta({ layout: 'auth' })
useHead({ title: 'Esqueci minha senha · CEASA Fruit' })

const api = useApi()
const sent = ref(false)
const loading = ref(false)
const errorMessage = ref('')

const schema = z.object({ email: z.email('E-mail inválido') })
const state = reactive({ email: '' })

async function onSubmit(event: FormSubmitEvent<z.output<typeof schema>>) {
  errorMessage.value = ''
  loading.value = true
  const { error } = await api.POST('/auth/password/forgot', { body: event.data })
  loading.value = false
  if (error) {
    errorMessage.value = apiErrorMessage(error)
    return
  }
  // A API responde igual exista o e-mail ou não (para não revelar contas).
  sent.value = true
}
</script>

<template>
  <div v-if="sent">
    <AuthTitle
      title="Verifique seu e-mail"
      description="Se houver uma conta com esse e-mail, enviamos um link para criar uma nova senha. Ele vale por 1 hora. Confira também a caixa de spam."
    />
    <UButton label="Voltar para o login" to="/login" block class="mt-10 h-14 rounded-[8px] hover:bg-(--auth-green-dark) text-lg font-bold" />
  </div>

  <div v-else>
    <AuthTitle title="Esqueci minha senha" description="Informe seu e-mail e enviaremos um link para criar uma nova senha." />

    <UForm :schema="schema" :state="state" class="mt-6 sm:mt-7" @submit="onSubmit">
      <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" class="mb-6" />
      <UFormField name="email" label="E-mail" :ui="authFieldUi">
        <UInput v-model="state.email" type="email" placeholder="seu@email.com" autocomplete="username" size="xl" :ui="authInputUi" />
      </UFormField>
      <UButton type="submit" label="Enviar link" block :loading="loading" class="mt-9 h-14 rounded-[8px] hover:bg-(--auth-green-dark) text-lg font-bold sm:text-xl" />
    </UForm>

    <p class="mt-7 text-center text-base text-muted sm:text-lg">
      Lembrou?
      <ULink to="/login" class="font-bold text-highlighted hover:underline">Entrar</ULink>
    </p>
  </div>
</template>
