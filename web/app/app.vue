<script setup lang="ts">
import { pt_br } from '@nuxt/ui/locale'
</script>

<template>
  <!-- UApp habilita toasts, tooltips e modais; o locale traduz os textos internos dos componentes. -->
  <UApp :locale="pt_br">
    <NuxtLayout>
      <NuxtPage />
    </NuxtLayout>
  </UApp>
</template>
