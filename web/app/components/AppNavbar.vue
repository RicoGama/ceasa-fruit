<script setup lang="ts">
// Cabeçalho padrão das telas do painel, com a trilha de navegação (breadcrumb).
//
//   <AppNavbar />                       -> trilha da rota atual (utils/breadcrumbs.ts)
//   <AppNavbar :title="fornecedor" />   -> troca o último nível pelo nome do registro
//   <template #right> ... </template>   -> botões da tela (Salvar, Novo...)
//
// Celular (< 640px): a trilha inteira não cabe. Mostra um botão "voltar" para o
// nível de cima e, empilhados, o nome desse nível (pequeno) e o da tela atual.
// Telas de primeiro nível (listas) mostram só o título; o menu abre pelo
// botão que o próprio UDashboardNavbar coloca no celular.
// Tablet e computador: a trilha completa, com o último nível em destaque.
const props = defineProps<{ title?: string }>()

const route = useRoute()
const trail = computed(() => breadcrumbTrail(route.path, props.title))
const current = computed(() => trail.value[trail.value.length - 1]!)
const parent = computed(() => breadcrumbParent(trail.value))
// Na lista (ex.: Fornecedores) o "pai" é o Início: não vale um botão voltar.
const showBack = computed(() => !!parent.value && parent.value.to !== '/app')

const items = computed(() => {
  const last = trail.value.length - 1
  return trail.value.map((c, i) => ({
    label: c.label,
    to: c.to,
    icon: i === 0 ? c.icon : undefined,
    // Nível só agrupador (sem link): aparece em cinza, sem cara de link.
    class: c.to || i === last ? undefined : 'text-dimmed',
    // Os níveis de cima ficam sempre inteiros; só o último (o nome do
    // registro, que pode ser bem longo) é cortado com "…" se faltar espaço.
    ui: i === last
      ? { item: 'min-w-0 shrink', link: 'min-w-0', linkLabel: 'truncate' }
      : { item: 'shrink-0' },
  }))
})

</script>

<template>
  <UDashboardNavbar :ui="{ left: 'min-w-0 flex-1', title: 'min-w-0' }">
    <template #leading>
      <!-- Recolher o menu lateral (só aparece em telas grandes) -->
      <UDashboardSidebarCollapse />
      <!-- Celular: voltar para o nível de cima -->
      <UButton
        v-if="showBack"
        :to="parent!.to"
        icon="i-lucide-arrow-left"
        color="neutral"
        variant="ghost"
        class="sm:hidden -ms-1.5"
        :aria-label="`Voltar para ${parent!.label}`"
      />
    </template>

    <template #title>
      <!-- Celular: nível de cima pequeno + tela atual -->
      <div class="sm:hidden min-w-0 leading-tight">
        <NuxtLink v-if="showBack" :to="parent!.to" class="block truncate text-xs font-medium text-muted">
          {{ parent!.label }}
        </NuxtLink>
        <h1 class="truncate text-base font-semibold text-highlighted">
          {{ current.label }}
        </h1>
      </div>

      <!-- Tablet e computador: trilha completa -->
      <UBreadcrumb
        :items="items"
        class="hidden sm:flex min-w-0"
        :ui="{ root: 'min-w-0', list: 'min-w-0 flex-nowrap', link: 'text-sm' }"
      />
    </template>

    <template #right>
      <slot name="right" />
    </template>
  </UDashboardNavbar>
</template>
