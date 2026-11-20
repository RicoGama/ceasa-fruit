<script setup lang="ts">
definePageMeta({ layout: 'dashboard', permission: 'users.manage' })

const route = useRoute()
const id = Number(route.params.id)
const api = useApi()

const { data: role, error } = await useAsyncData(`role-${id}`, async () => {
  const { data, error } = await api.GET('/roles/{id}', { params: { path: { id } } })
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
})

useHead({ title: () => `${role.value?.name ?? 'Papel'} · CEASA Fruit` })
</script>

<template>
  <RolesRoleForm v-if="role" :key="`${role.id}-${role.version}`" :role="role" />

  <UDashboardPanel v-else id="role-not-found">
    <template #header>
      <AppNavbar />
    </template>
    <template #body>
      <UEmpty
        icon="i-lucide-search-x"
        :title="error?.statusCode === 404 ? 'Papel não encontrado' : 'Não foi possível carregar'"
        :description="error?.message"
        :actions="[{ label: 'Voltar para os papéis', to: '/app/settings/roles' }]"
      />
    </template>
  </UDashboardPanel>
</template>
