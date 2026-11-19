<script setup lang="ts">
// Não há site público: o sistema roda no servidor do box. A raiz só
// encaminha para o painel (o middleware manda para o login se preciso).
definePageMeta({ middleware: () => navigateTo('/app', { replace: true }) })
</script>

<template>
  <div />
</template>
