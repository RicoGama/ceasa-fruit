<script setup lang="ts">
definePageMeta({ layout: 'dashboard', permission: 'products.edit' })
useHead({ title: 'Novo produto · CEASA Fruit' })
</script>

<template>
  <ProductsProductForm :product="null" />
</template>
