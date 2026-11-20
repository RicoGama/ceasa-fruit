<script setup lang="ts">
definePageMeta({ layout: 'dashboard', permission: 'suppliers.view' })

// [id] no nome do arquivo vira um parâmetro da rota: /app/suppliers/42 -> id = "42".
const route = useRoute()
const id = Number(route.params.id)
const api = useApi()

const { data: supplier, error } = await useAsyncData(`supplier-${id}`, async () => {
  const { data, error } = await api.GET('/suppliers/{id}', { params: { path: { id } } })
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
})

useHead({ title: () => `${supplier.value ? displayName(supplier.value) : 'Fornecedor'} · CEASA Fruit` })
</script>

<template>
  <!-- A chave inclui a versão: ao recarregar depois de um conflito, o formulário é recriado com os dados novos. -->
  <SuppliersSupplierForm v-if="supplier" :key="`${supplier.id}-${supplier.version}`" :supplier="supplier" />

  <UDashboardPanel v-else id="supplier-not-found">
    <template #header>
      <AppNavbar />
    </template>
    <template #body>
      <UEmpty
        icon="i-lucide-search-x"
        :title="error?.statusCode === 404 ? 'Fornecedor não encontrado' : 'Não foi possível carregar'"
        :description="error?.message"
        :actions="[{ label: 'Voltar para a lista', to: '/app/suppliers' }]"
      />
    </template>
  </UDashboardPanel>
</template>
