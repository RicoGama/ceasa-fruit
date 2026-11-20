<script setup lang="ts">
definePageMeta({ layout: 'dashboard' })

const route = useRoute()
const id = Number(route.params.id)
const api = useApi()

const { data: product, error } = await useAsyncData(`product-${id}`, async () => {
  const { data, error } = await api.GET('/products/{id}', { params: { path: { id } } })
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
})

useHead({ title: () => `${product.value?.displayName ?? 'Produto'} · CEASA Fruit` })
</script>

<template>
  <!-- A chave inclui a versão: ao recarregar depois de um conflito, o formulário é recriado com os dados novos. -->
  <ProductsProductForm v-if="product" :key="`${product.id}-${product.version}`" :product="product" />

  <UDashboardPanel v-else id="product-not-found">
    <template #header>
      <UDashboardNavbar title="Produto">
        <template #leading>
          <UButton icon="i-lucide-arrow-left" color="neutral" variant="ghost" to="/app/products" aria-label="Voltar" />
        </template>
      </UDashboardNavbar>
    </template>
    <template #body>
      <UEmpty
        icon="i-lucide-search-x"
        :title="error?.statusCode === 404 ? 'Produto não encontrado' : 'Não foi possível carregar'"
        :description="error?.message"
        :actions="[{ label: 'Voltar para a lista', to: '/app/products' }]"
      />
    </template>
  </UDashboardPanel>
</template>
