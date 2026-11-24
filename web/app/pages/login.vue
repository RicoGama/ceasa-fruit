<script setup lang="ts">
import { z } from 'zod'
import type { FormSubmitEvent } from '@nuxt/ui'

definePageMeta({ layout: 'auth' })
useHead({ title: 'Entrar · CEASA Fruit' })

const auth = useAuthStore()
const route = useRoute()
const errorMessage = ref('')
const loading = ref(false)
const requestOpen = ref(false)

const schema = z.object({
  email: z.email('E-mail inválido'),
  password: z.string().min(1, 'Informe a senha'),
})
type Schema = z.output<typeof schema>
const state = reactive({ email: '', password: '' })

async function onSubmit(event: FormSubmitEvent<Schema>) {
  errorMessage.value = ''
  loading.value = true
  try {
    await auth.login(event.data)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/app'
    await navigateTo(redirect)
  }
  catch (error) {
    errorMessage.value = apiErrorMessage(error)
  }
  finally {
    loading.value = false
  }
}
</script>

<template>
  <div>
    <AuthTitle title="Acesse sua conta" description="Entre com seu e-mail e senha." />

    <UForm :schema="schema" :state="state" class="mt-6 sm:mt-7" @submit="onSubmit">
      <UAlert
        v-if="errorMessage"
        color="error"
        variant="subtle"
        icon="i-lucide-circle-alert"
        :title="errorMessage"
        class="mb-6"
      />

      <UFormField name="email" label="E-mail" :ui="authFieldUi">
        <UInput
          v-model="state.email"
          type="email"
          placeholder="seu@email.com"
          autocomplete="username"
          size="xl"
          :ui="authInputUi"
        />
      </UFormField>

      <UFormField name="password" label="Senha" class="mt-6" :ui="authFieldUi">
        <AuthPasswordInput v-model="state.password" placeholder="Digite sua senha" autocomplete="current-password" />
      </UFormField>

      <div class="mt-4 text-right">
        <ULink to="/forgot-password" class="text-sm font-bold text-highlighted hover:underline sm:text-base">
          Esqueci minha senha
        </ULink>
      </div>

      <UButton
        type="submit"
        label="Entrar"
        block
        :loading="loading"
        class="mt-9 h-14 rounded-[8px] text-lg font-bold sm:text-[1.35rem]"
      />
    </UForm>

    <p class="mt-7 text-center text-base text-muted sm:text-lg">
      Ainda não tem acesso?
      <button type="button" class="font-bold text-highlighted hover:underline" @click="requestOpen = true">
        Solicitar acesso
      </button>
    </p>

    <!-- Não há cadastro aberto: quem dá acesso é o administrador, por convite. -->
    <UModal v-model:open="requestOpen" title="Solicitar acesso" :ui="{ content: 'auth-skin' }">
      <template #body>
        <p class="text-muted">
          O acesso ao sistema é liberado pelo administrador da empresa. Peça a ele
          um convite: você vai receber um e-mail com o link para criar a sua senha.
        </p>
      </template>
      <template #footer>
        <UButton label="Entendi" block size="lg" class="h-12 rounded-lg font-semibold" @click="requestOpen = false" />
      </template>
    </UModal>
  </div>
</template>
