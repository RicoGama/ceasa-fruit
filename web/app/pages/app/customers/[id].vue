<script setup lang="ts">
definePageMeta({ layout: 'dashboard', permission: 'customers.view' })

const route = useRoute()
const id = Number(route.params.id)
const api = useApi()

const { data: customer, error } = await useAsyncData(`customer-${id}`, async () => {
  const { data, error } = await api.GET('/customers/{id}', { params: { path: { id } } })
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
})

useHead({ title: () => `${customer.value ? displayName(customer.value) : 'Cliente'} · CEASA Fruit` })
</script>

<template>
  <!-- A chave inclui a versão: ao recarregar depois de um conflito, o formulário é recriado com os dados novos. -->
  <CustomersCustomerForm v-if="customer" :key="`${customer.id}-${customer.version}`" :customer="customer" />

  <UDashboardPanel v-else id="customer-not-found">
    <template #header>
      <AppNavbar />
    </template>
    <template #body>
      <UEmpty
        icon="i-lucide-search-x"
        :title="error?.statusCode === 404 ? 'Cliente não encontrado' : 'Não foi possível carregar'"
        :description="error?.message"
        :actions="[{ label: 'Voltar para a lista', to: '/app/customers' }]"
      />
    </template>
  </UDashboardPanel>
</template>
