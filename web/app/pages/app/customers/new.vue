<script setup lang="ts">
definePageMeta({ layout: 'dashboard', permission: 'customers.edit' })
useHead({ title: 'Novo cliente · CEASA Fruit' })
</script>

<template>
  <CustomersCustomerForm :customer="null" />
</template>
