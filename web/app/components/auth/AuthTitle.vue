<script setup lang="ts">
// Título das telas de acesso (login e senha).
defineProps<{ title: string, description?: string }>()
</script>

<template>
  <header class="text-center">
    <h1 class="text-[2rem] font-bold leading-tight tracking-[-0.01em] text-highlighted sm:text-[2.75rem]">
      {{ title }}
    </h1>
    <p v-if="description" class="mt-1.5 text-base text-muted sm:text-[1.3rem]">
      {{ description }}
    </p>
  </header>
</template>
