<script setup lang="ts">
definePageMeta({ layout: 'dashboard', permission: 'users.manage' })
useHead({ title: 'Novo papel · CEASA Fruit' })
</script>

<template>
  <RolesRoleForm :role="null" />
</template>
