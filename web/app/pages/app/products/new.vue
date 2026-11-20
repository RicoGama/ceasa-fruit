<script setup lang="ts">
definePageMeta({ layout: 'dashboard' })
useHead({ title: 'Novo produto · CEASA Fruit' })
</script>

<template>
  <ProductsProductForm :product="null" />
</template>
