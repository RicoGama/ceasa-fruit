<script setup lang="ts">
import type { NavigationMenuItem } from '@nuxt/ui'

const auth = useAuthStore()

// open controla a barra lateral no celular (vira um menu deslizante).
const open = ref(false)

const mainLinks: NavigationMenuItem[] = [
  { label: 'Início', icon: 'i-lucide-house', to: '/app', exact: true },
  { label: 'Fornecedores', icon: 'i-lucide-truck', to: '/app/suppliers' },
  { label: 'Clientes', icon: 'i-lucide-users', disabled: true, badge: 'em breve' },
  { label: 'Estoque', icon: 'i-lucide-package', disabled: true, badge: 'em breve' },
  { label: 'Vendas', icon: 'i-lucide-shopping-cart', disabled: true, badge: 'em breve' },
  { label: 'Rotas', icon: 'i-lucide-map', disabled: true, badge: 'em breve' },
]

// computed: recalcula sozinho quando o store muda (ex.: depois do login).
const adminLinks = computed<NavigationMenuItem[]>(() =>
  auth.isPlatformAdmin
    ? [
        { label: 'Plataforma', type: 'label' },
        { label: 'Organizações', icon: 'i-lucide-building-2', to: '/app/admin/organizations' },
        // Painel do River (fora do Nuxt): abre em outra aba.
        { label: 'Filas', icon: 'i-lucide-list-checks', to: '/admin/queues', external: true, target: '_blank' },
      ]
    : [],
)
</script>

<template>
  <UDashboardGroup unit="rem">
    <UDashboardSidebar
      id="default"
      v-model:open="open"
      collapsible
      resizable
      class="bg-elevated/25"
      :ui="{ footer: 'lg:border-t lg:border-default' }"
    >
      <template #header="{ collapsed }">
        <OrganizationSwitcher :collapsed="collapsed" />
      </template>

      <template #default="{ collapsed }">
        <UNavigationMenu :collapsed="collapsed" :items="mainLinks" orientation="vertical" tooltip popover />
        <UNavigationMenu
          v-if="adminLinks.length"
          :collapsed="collapsed"
          :items="adminLinks"
          orientation="vertical"
          tooltip
          class="mt-auto"
        />
      </template>

      <template #footer="{ collapsed }">
        <UserMenu :collapsed="collapsed" />
      </template>
    </UDashboardSidebar>

    <slot />
  </UDashboardGroup>
</template>
