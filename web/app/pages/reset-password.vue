<script setup lang="ts">
import { z } from 'zod'
import type { FormSubmitEvent } from '@nuxt/ui'

definePageMeta({ layout: 'auth' })
useHead({ title: 'Nova senha · CEASA Fruit' })

const api = useApi()
const toast = useToast()
const route = useRoute()
const errorMessage = ref('')
const loading = ref(false)

// O token vem no link do e-mail: /reset-password?token=...
const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))
// Convite de usuário novo (?invite=1): mesma tela, outros textos.
const isInvite = computed(() => route.query.invite === '1')

const schema = z.object({
  password: z.string().min(8, 'A senha precisa de pelo menos 8 caracteres').max(128),
  confirm: z.string(),
}).refine(v => v.password === v.confirm, { path: ['confirm'], message: 'As senhas não conferem' })
const state = reactive({ password: '', confirm: '' })

async function onSubmit(event: FormSubmitEvent<z.output<typeof schema>>) {
  errorMessage.value = ''
  loading.value = true
  const { error } = await api.POST('/auth/password/reset', {
    body: { token: token.value, password: event.data.password },
  })
  loading.value = false
  if (error) {
    errorMessage.value = apiErrorMessage(error)
    return
  }
  toast.add({
    title: isInvite.value ? 'Senha criada' : 'Senha alterada',
    description: 'Entre com seu e-mail e a nova senha.',
    color: 'success',
  })
  await navigateTo('/login')
}
</script>

<template>
  <div v-if="!token">
    <AuthTitle title="Link incompleto" description="Abra o link exatamente como chegou no e-mail, ou peça um novo." />
    <UButton label="Pedir novo link" to="/forgot-password" block class="mt-10 h-14 rounded-[8px] hover:bg-(--auth-green-dark) text-lg font-bold" />
  </div>

  <div v-else>
    <AuthTitle
      :title="isInvite ? 'Criar sua senha' : 'Criar nova senha'"
      :description="isInvite
        ? 'Escolha a senha que você vai usar para entrar.'
        : 'Depois de salvar, você vai entrar de novo em todos os aparelhos.'"
    />

    <UForm :schema="schema" :state="state" class="mt-6 sm:mt-7" @submit="onSubmit">
      <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" class="mb-6">
        <template v-if="errorMessage.includes('expirou')" #actions>
          <UButton label="Pedir novo link" to="/forgot-password" size="xs" color="error" variant="outline" />
        </template>
      </UAlert>
      <UFormField name="password" label="Nova senha" hint="Pelo menos 8 caracteres" :ui="{ ...authFieldUi, hint: 'text-sm text-muted' }">
        <AuthPasswordInput v-model="state.password" placeholder="Digite a nova senha" autocomplete="new-password" />
      </UFormField>
      <UFormField name="confirm" label="Repita a nova senha" class="mt-6" :ui="authFieldUi">
        <AuthPasswordInput v-model="state.confirm" placeholder="Digite de novo" autocomplete="new-password" />
      </UFormField>
      <UButton
        type="submit"
        :label="isInvite ? 'Criar senha' : 'Salvar nova senha'"
        block
        :loading="loading"
        class="mt-9 h-14 rounded-[8px] hover:bg-(--auth-green-dark) text-lg font-bold sm:text-xl"
      />
    </UForm>
  </div>
</template>
