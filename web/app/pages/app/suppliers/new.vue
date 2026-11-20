<script setup lang="ts">
definePageMeta({ layout: 'dashboard', permission: 'suppliers.edit' })
useHead({ title: 'Novo fornecedor · CEASA Fruit' })
</script>

<template>
  <SuppliersSupplierForm :supplier="null" />
</template>
