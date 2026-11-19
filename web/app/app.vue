<script setup lang="ts">
import { pt_br } from '@nuxt/ui/locale'

// Aplica o tema de cor (azul/verde) no <html>; o CSS em main.css faz o resto.
const theme = useBrandTheme()
useHead({ htmlAttrs: { 'data-theme': () => theme.value } })
</script>

<template>
  <!-- UApp habilita toasts, tooltips e modais; o locale traduz os textos internos dos componentes. -->
  <UApp :locale="pt_br">
    <NuxtLayout>
      <NuxtPage />
    </NuxtLayout>
  </UApp>
</template>
