<script setup lang="ts">
definePageMeta({ layout: 'dashboard' })
useHead({ title: 'Início · CEASA Fruit' })

const api = useApi()
const { me, currentOrganization } = useAuth()

// Só o total de fornecedores ativos (limit=1 para não trazer a lista toda).
const { data: suppliers } = await useAsyncData('home-suppliers', async () => {
  const { data } = await api.GET('/suppliers', { params: { query: { limit: 1 } } })
  return data
})

const firstName = computed(() => me.value?.user.name.split(' ')[0])

const comingSoon = [
  { title: 'Clientes', description: 'Restaurantes, sacolões e mercados que compram de você.', icon: 'i-lucide-users' },
  { title: 'Estoque', description: 'Lotes com validade e alerta antes de virar perda.', icon: 'i-lucide-package' },
  { title: 'Vendas', description: 'Pedidos por kg, caixa, maço ou unidade.', icon: 'i-lucide-shopping-cart' },
  { title: 'Rotas', description: 'Entregas organizadas por rota e caminhão.', icon: 'i-lucide-map' },
]
</script>

<template>
  <UDashboardPanel id="home">
    <template #header>
      <UDashboardNavbar title="Início">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <div>
        <h2 class="text-2xl font-semibold text-highlighted">
          Olá, {{ firstName }}!
        </h2>
        <p class="text-muted">
          Você está em <span class="font-medium text-default">{{ currentOrganization?.name }}</span>.
        </p>
      </div>

      <UPageGrid class="lg:grid-cols-4 gap-4">
        <UPageCard
          title="Fornecedores ativos"
          icon="i-lucide-truck"
          to="/app/suppliers"
          variant="subtle"
        >
          <p class="text-3xl font-semibold text-highlighted">
            {{ suppliers?.total ?? '—' }}
          </p>
        </UPageCard>
      </UPageGrid>

      <div>
        <h3 class="text-sm font-medium text-muted mb-3">
          Em breve
        </h3>
        <UPageGrid class="lg:grid-cols-4 gap-4">
          <UPageCard
            v-for="item in comingSoon"
            :key="item.title"
            :title="item.title"
            :description="item.description"
            :icon="item.icon"
            variant="outline"
            class="opacity-70"
          />
        </UPageGrid>
      </div>
    </template>
  </UDashboardPanel>
</template>
