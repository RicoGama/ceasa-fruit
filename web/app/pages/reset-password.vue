<script setup lang="ts">
import { z } from 'zod'
import type { AuthFormField, FormSubmitEvent } from '@nuxt/ui'

definePageMeta({ layout: 'auth' })
useHead({ title: 'Nova senha · CEASA Fruit' })

const api = useApi()
const toast = useToast()
const route = useRoute()
const errorMessage = ref('')

// O token vem no link do e-mail: /reset-password?token=...
const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))

const fields: AuthFormField[] = [
  { name: 'password', type: 'password', label: 'Nova senha', description: 'Pelo menos 8 caracteres', required: true },
  { name: 'confirm', type: 'password', label: 'Repita a nova senha', required: true },
]

const schema = z.object({
  password: z.string().min(8, 'A senha precisa de pelo menos 8 caracteres').max(128),
  confirm: z.string(),
}).refine(v => v.password === v.confirm, { path: ['confirm'], message: 'As senhas não conferem' })

async function onSubmit(event: FormSubmitEvent<z.output<typeof schema>>) {
  errorMessage.value = ''
  const { error } = await api.POST('/auth/password/reset', {
    body: { token: token.value, password: event.data.password },
  })
  if (error) {
    errorMessage.value = apiErrorMessage(error)
    return
  }
  toast.add({ title: 'Senha alterada', description: 'Entre com a nova senha.', color: 'success' })
  await navigateTo('/login')
}
</script>

<template>
  <div v-if="!token" class="space-y-4 text-center">
    <UIcon name="i-lucide-link-2-off" class="size-10 text-warning mx-auto" />
    <h1 class="text-xl font-semibold text-highlighted">
      Link incompleto
    </h1>
    <p class="text-sm text-muted">
      Abra o link exatamente como chegou no e-mail, ou peça um novo.
    </p>
    <UButton label="Pedir novo link" to="/forgot-password" block />
  </div>

  <UAuthForm
    v-else
    title="Criar nova senha"
    description="Depois de salvar, você vai entrar de novo em todos os aparelhos."
    icon="i-lucide-lock-keyhole"
    :fields="fields"
    :schema="schema"
    :submit="{ label: 'Salvar nova senha', block: true }"
    loading-auto
    @submit="onSubmit"
  >
    <template #validation>
      <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage">
        <template v-if="errorMessage.includes('expirou')" #actions>
          <UButton label="Pedir novo link" to="/forgot-password" size="xs" color="error" variant="outline" />
        </template>
      </UAlert>
    </template>
  </UAuthForm>
</template>
