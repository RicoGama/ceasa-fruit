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

// Frota: o mapa e os cadastros que só fazem sentido para quem vê a frota.
const fleetLinks = computed<NavigationMenuItem[]>(() => auth.can('fleet.view')
  ? [
      { label: 'Frota', type: 'label' },
      { label: 'Mapa da frota', icon: 'i-lucide-map', to: '/app/fleet', exact: true },
      { label: 'Veículos', icon: 'i-lucide-car-front', to: '/app/fleet/vehicles' },
      { label: 'Motoristas', icon: 'i-lucide-id-card', to: '/app/fleet/drivers' },
    ]
  : [])

// Cada item só aparece para quem tem a permissão (computed: muda sozinho se
// o papel da pessoa mudar).
const registryLinks = computed<NavigationMenuItem[]>(() => [
  { label: 'Cadastros', type: 'label' },
  ...(auth.can('products.view') ? [{ label: 'Produtos', icon: 'i-lucide-apple', to: '/app/products' }] : []),
  ...(auth.can('suppliers.view') ? [{ label: 'Fornecedores', icon: 'i-lucide-truck', to: '/app/suppliers' }] : []),
  ...(auth.can('customers.view') ? [{ label: 'Clientes', icon: 'i-lucide-users', to: '/app/customers' }] : []),
])

// Certificado digital: selo "vence em N dias" no menu e aviso uma vez por sessão.
const { current: certificate, badge: certificateBadge } = useCertificateStatus()

const adminLinks = computed<NavigationMenuItem[]>(() => {
  const items: NavigationMenuItem[] = []
  if (auth.can('users.manage')) {
    items.push(
      { label: 'Usuários', icon: 'i-lucide-user-cog', to: '/app/settings/users' },
      { label: 'Papéis e permissões', icon: 'i-lucide-shield-check', to: '/app/settings/roles' },
    )
  }
  if (auth.can('audit.view')) {
    items.push(
      { label: 'Acessos', icon: 'i-lucide-log-in', to: '/app/settings/access' },
      { label: 'Alterações', icon: 'i-lucide-file-pen-line', to: '/app/settings/changes' },
    )
  }
  if (auth.can('company.edit')) items.push({ label: 'Dados da empresa', icon: 'i-lucide-building-2', to: '/app/settings/company' })
  if (auth.can('certificate.manage')) {
    items.push({
      label: 'Certificado digital', icon: 'i-lucide-file-key-2', to: '/app/settings/certificate',
      badge: certificateBadge.value
        ? { label: certificateBadge.value, color: certificate.value?.status === 'expired' ? 'error' : 'warning', variant: 'subtle', size: 'sm' }
        : undefined,
    })
  }
  // Painel do River (fora do Nuxt): abre em outra aba.
  if (auth.can('queues.view')) items.push({ label: 'Filas', icon: 'i-lucide-list-checks', to: '/admin/queues', external: true, target: '_blank' })
  return items.length ? [{ label: 'Administração', type: 'label' }, ...items] : []
})

// Aviso de conexão: se o tempo real cair por mais de 4s (Wi-Fi do box,
// servidor reiniciando), mostra uma faixa; some sozinha quando voltar, e as
// telas recarregam o que perderam.
const realtime = useRealtimeStore()
const showOffline = ref(false)
let offlineTimer: ReturnType<typeof setTimeout> | undefined
watch(() => realtime.status, (status) => {
  clearTimeout(offlineTimer)
  if (status === 'live') showOffline.value = false
  else offlineTimer = setTimeout(() => { showOffline.value = realtime.status !== 'live' }, 4000)
})

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
        <!-- Cartão da empresa (cliente) dona deste servidor -->
        <div
          v-if="!collapsed && auth.company"
          class="flex items-center gap-3 rounded-xl border border-default bg-canvas/60 p-3"
        >
          <span class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-brand-800 dark:text-brand-200">
            <UIcon name="i-lucide-store" class="size-5" />
          </span>
          <span class="min-w-0 flex-1">
            <span class="block text-sm font-bold text-highlighted truncate">{{ auth.company.tradeName || auth.company.name }}</span>
            <span class="block text-xs text-muted truncate">Box CEASA-RJ</span>
          </span>
        </div>
      </template>

      <template #default="{ collapsed }">
        <UNavigationMenu :collapsed="collapsed" :items="commercialLinks" orientation="vertical" tooltip popover :ui="menuUi" />
        <UNavigationMenu :collapsed="collapsed" :items="registryLinks" orientation="vertical" tooltip popover :ui="menuUi" />
        <UNavigationMenu v-if="fleetLinks.length" :collapsed="collapsed" :items="fleetLinks" orientation="vertical" tooltip popover :ui="menuUi" />
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

    <!-- Faixa de "sem conexão", flutuando acima do conteúdo -->
    <Transition
      enter-from-class="opacity-0 translate-y-2"
      leave-to-class="opacity-0 translate-y-2"
      enter-active-class="transition"
      leave-active-class="transition"
    >
      <div
        v-if="showOffline"
        role="status"
        class="fixed inset-x-4 bottom-4 z-50 mx-auto flex max-w-md items-center gap-3 rounded-xl bg-warning px-4 py-3 text-sm font-medium text-inverted shadow-lg"
      >
        <UIcon name="i-lucide-wifi-off" class="size-5 shrink-0" />
        <span>Sem conexão com o servidor. Tentando de novo… As telas podem estar desatualizadas.</span>
      </div>
    </Transition>
  </UDashboardGroup>
</template>
