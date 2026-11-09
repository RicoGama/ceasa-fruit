<script setup lang="ts">
definePageMeta({ layout: 'dashboard' })
useHead({ title: 'Novo fornecedor · CEASA Fruit' })
</script>

<template>
  <SuppliersSupplierForm :supplier="null" />
</template>
