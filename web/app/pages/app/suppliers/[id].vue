<script setup lang="ts">
definePageMeta({ layout: 'dashboard' })

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
  <SuppliersSupplierForm v-if="supplier" :key="supplier.id" :supplier="supplier" />

  <UDashboardPanel v-else id="supplier-not-found">
    <template #header>
      <UDashboardNavbar title="Fornecedor">
        <template #leading>
          <UButton icon="i-lucide-arrow-left" color="neutral" variant="ghost" to="/app/suppliers" aria-label="Voltar" />
        </template>
      </UDashboardNavbar>
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
