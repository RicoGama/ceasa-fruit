<script setup lang="ts">
import type { NuxtError } from '#app'

// Página de erro do app inteiro (ex.: endereço que não existe), em português.
const props = defineProps<{ error: NuxtError }>()

const title = computed(() => {
  if (props.error.statusCode === 404) return 'Página não encontrada'
  if (props.error.statusCode === 403) return 'Acesso não permitido'
  return 'Algo deu errado'
})
const description = computed(() => {
  if (props.error.statusCode === 404) return 'O endereço que você abriu não existe ou foi movido.'
  if (props.error.statusCode === 403) return 'Seu perfil não tem permissão para ver esta página.'
  return 'Tente de novo em alguns instantes. Se continuar, avise o administrador do sistema.'
})

useHead({ title: `${title.value} · CEASA Fruit` })
</script>

<template>
  <UApp>
    <div class="flex min-h-dvh flex-col items-center justify-center gap-4 bg-canvas p-6 text-center">
      <p class="text-6xl font-extrabold text-primary">
        {{ error.statusCode }}
      </p>
      <h1 class="text-2xl font-bold text-highlighted">
        {{ title }}
      </h1>
      <p class="max-w-md text-muted">
        {{ description }}
      </p>
      <UButton label="Voltar ao painel" icon="i-lucide-arrow-left" size="lg" @click="clearError({ redirect: '/app' })" />
    </div>
  </UApp>
</template>
