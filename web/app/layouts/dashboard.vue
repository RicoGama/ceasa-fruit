<script setup lang="ts">
import type { NavigationMenuItem } from '@nuxt/ui'

const auth = useAuthStore()
const brand = useRuntimeConfig().public.brand

// open controla a barra lateral no celular (vira um menu deslizante).
const open = ref(false)

// Menu agrupado em seções, como no layout do designer. Itens ainda não
// construídos aparecem desabilitados com "em breve".
const soon = { disabled: true, badge: { label: 'em breve', color: 'neutral' as const, variant: 'outline' as const, size: 'sm' as const } }

const commercialLinks: NavigationMenuItem[] = [
  { label: 'Gestão comercial', type: 'label' },
  { label: 'Visão geral', icon: 'i-lucide-layout-grid', to: '/app', exact: true },
  { label: 'Cotações e pedidos', icon: 'i-lucide-clipboard-list', ...soon },
  { label: 'Estoque por lote', icon: 'i-lucide-package', ...soon },
  { label: 'Compras e vendas', icon: 'i-lucide-chart-column', ...soon },
  { label: 'Diagnóstico operacional', icon: 'i-lucide-clipboard-check', ...soon },
]

const registryLinks: NavigationMenuItem[] = [
  { label: 'Cadastros', type: 'label' },
  { label: 'Fornecedores', icon: 'i-lucide-truck', to: '/app/suppliers' },
  { label: 'Clientes', icon: 'i-lucide-users', ...soon },
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

// Estilo dos itens do menu: o item ativo ganha fundo suave na cor do tema.
const menuUi = {
  label: 'text-[11px] font-bold uppercase tracking-[0.12em] text-dimmed px-2.5 pt-4',
  link: 'px-2.5 py-2.5 text-sm font-semibold rounded-xl',
  linkLeadingIcon: 'size-5',
}
</script>

<template>
  <UDashboardGroup unit="rem" class="bg-canvas">
    <UDashboardSidebar
      id="default"
      v-model:open="open"
      collapsible
      resizable
      :default-size="17"
      class="bg-default"
      :ui="{ header: 'h-auto flex-col items-stretch gap-4 py-5', footer: 'lg:border-t lg:border-default py-3' }"
    >
      <template #header="{ collapsed }">
        <!-- No modo escuro o logo (vermelho) ganha uma plaquinha branca para não sumir no fundo. -->
        <NuxtLink to="/app" class="flex justify-center rounded-xl px-1 dark:bg-white dark:py-1.5">
          <img v-if="!collapsed" :src="brand.logo" :alt="brand.name" class="h-14 w-auto object-contain">
          <img v-else src="/brand/icon-512.png" :alt="brand.name" class="size-9 object-contain">
        </NuxtLink>
        <OrganizationSwitcher :collapsed="collapsed" />
      </template>

      <template #default="{ collapsed }">
        <UNavigationMenu :collapsed="collapsed" :items="commercialLinks" orientation="vertical" tooltip popover :ui="menuUi" />
        <UNavigationMenu :collapsed="collapsed" :items="registryLinks" orientation="vertical" tooltip popover :ui="menuUi" />
        <UNavigationMenu
          v-if="adminLinks.length"
          :collapsed="collapsed"
          :items="adminLinks"
          orientation="vertical"
          tooltip
          :ui="menuUi"
        />
      </template>

      <template #footer="{ collapsed }">
        <UserMenu :collapsed="collapsed" />
      </template>
    </UDashboardSidebar>

    <slot />
  </UDashboardGroup>
</template>
