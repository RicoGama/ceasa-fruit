<script setup lang="ts">
definePageMeta({ layout: 'dashboard' })
useHead({ title: 'Visão geral · CEASA Fruit' })

const auth = useAuthStore()
const { kpis, chart, attention } = useDashboardDemo()

// Período: os últimos 6 meses ("Novembro de 2026"...). Por enquanto só exibe,
// os dados do painel ainda são ilustrativos.
const monthFormatter = new Intl.DateTimeFormat('pt-BR', { month: 'long', year: 'numeric' })
const periods = Array.from({ length: 6 }, (_, i) => {
  const d = new Date()
  d.setDate(1)
  d.setMonth(d.getMonth() - i)
  const label = monthFormatter.format(d)
  return { label: label.charAt(0).toUpperCase() + label.slice(1), value: `${d.getFullYear()}-${d.getMonth() + 1}` }
})
const period = ref(periods[0]!.value)

// Gráfico: compras ou vendas.
const chartMode = ref<'purchases' | 'sales'>('purchases')
const series = computed(() => chart[chartMode.value])
const maxValue = 8000 // topo do eixo (8 mil kg)
const ticks = [8000, 6000, 4000, 2000, 0]
const thousand = (n: number) => (n === 0 ? '0' : `${n / 1000} mil`)
const monthShort = (offset: number) => {
  const d = new Date()
  d.setDate(1)
  d.setMonth(d.getMonth() - offset)
  const s = new Intl.DateTimeFormat('pt-BR', { month: 'short' }).format(d).replace('.', '')
  return s.charAt(0).toUpperCase() + s.slice(1)
}

const toneClasses = {
  brand: 'bg-accent-soft text-brand-800 dark:text-brand-200',
  warning: 'bg-warning/12 text-warning',
} as const
</script>

<template>
  <UDashboardPanel id="home" :ui="{ body: 'bg-canvas gap-6 lg:p-8' }">
    <template #header>
      <UDashboardNavbar>
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
        <template #title>
          <nav class="flex items-center gap-2 text-sm" aria-label="Você está em">
            <span class="text-muted">Comercial</span>
            <UIcon name="i-lucide-chevron-right" class="size-4 text-dimmed" />
            <span class="font-bold text-highlighted">Visão geral</span>
          </nav>
        </template>
        <template #right>
          <UPopover>
            <UButton icon="i-lucide-bell" color="neutral" variant="outline" aria-label="Notificações" class="rounded-xl" />
            <template #content>
              <p class="p-4 text-sm text-muted">
                Nenhuma notificação nova.
              </p>
            </template>
          </UPopover>
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <!-- Cabeçalho da página -->
      <div class="flex flex-wrap items-end justify-between gap-4">
        <div>
          <div class="flex items-center gap-2">
            <p class="text-xs font-bold uppercase tracking-[0.14em] text-muted">
              Gestão comercial
            </p>
            <UBadge label="Dados ilustrativos" color="warning" variant="subtle" size="sm" />
          </div>
          <h1 class="mt-1 text-4xl font-extrabold tracking-tight text-highlighted">
            Visão geral
          </h1>
          <p class="mt-1 text-muted">
            Acompanhe as cotações, os pedidos e o volume das principais frutas.
          </p>
        </div>
        <div class="flex flex-col gap-1">
          <span class="text-xs font-semibold text-muted">Período</span>
          <USelect v-model="period" :items="periods" class="w-52" :ui="{ base: 'rounded-xl bg-default' }" />
        </div>
      </div>

      <!-- Banner -->
      <!-- shrink-0: sem ele, o banner (overflow-hidden) encolhe até sumir na coluna com rolagem -->
      <section class="relative shrink-0 overflow-hidden rounded-3xl bg-brand-900 text-white shadow-sm dark:bg-brand-950">
        <div class="grid lg:grid-cols-[1.1fr_1fr]">
          <div class="relative z-10 p-8 lg:p-10">
            <p class="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-white/80">
              <UIcon name="i-lucide-leaf" class="size-4" />
              {{ auth.company?.tradeName || auth.company?.name }}
            </p>
            <h2 class="mt-3 text-4xl font-extrabold tracking-tight lg:text-5xl">
              Da cotação à expedição
            </h2>
            <p class="mt-3 max-w-md text-white/80">
              Veja o que está em negociação e o que a operação já pode separar para entrega.
            </p>
            <UButton
              label="Ver fornecedores"
              trailing-icon="i-lucide-arrow-right"
              to="/app/suppliers"
              size="lg"
              class="mt-6 rounded-xl bg-accent-soft font-bold text-brand-900 hover:bg-white dark:bg-brand-200 dark:text-brand-950"
            />
          </div>
          <div class="h-48 lg:h-auto lg:min-h-64">
            <HeroIllustration />
          </div>
        </div>
      </section>

      <!-- Indicadores -->
      <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <div v-for="kpi in kpis" :key="kpi.label" class="rounded-2xl bg-default p-5 ring ring-default">
          <div class="flex items-start justify-between">
            <span class="flex size-10 items-center justify-center rounded-xl" :class="toneClasses[kpi.tone]">
              <UIcon :name="kpi.icon" class="size-5" />
            </span>
            <UIcon name="i-lucide-arrow-up-right" class="size-4 text-dimmed" />
          </div>
          <p class="mt-5 text-sm font-semibold text-muted">
            {{ kpi.label }}
          </p>
          <p class="mt-1 text-3xl font-extrabold tracking-tight text-highlighted">
            {{ kpi.value }}
          </p>
          <p class="mt-1 text-xs text-muted">
            {{ kpi.caption }}
          </p>
        </div>
      </div>

      <div class="grid gap-4 xl:grid-cols-[1.8fr_1fr]">
        <!-- Gráfico -->
        <section class="rounded-2xl bg-default p-6 ring ring-default">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 class="text-lg font-bold text-highlighted">
                {{ chartMode === 'purchases' ? 'Compras' : 'Vendas' }} das principais frutas
              </h3>
              <p class="text-sm text-muted">
                {{ chartMode === 'purchases' ? 'Recebimentos de fornecedores' : 'Saídas para clientes' }} · volume em kg
              </p>
            </div>
            <div class="flex items-center gap-4">
              <div class="flex rounded-xl bg-elevated p-1 text-sm font-semibold">
                <button
                  v-for="opt in [{ v: 'purchases', l: 'Compras' }, { v: 'sales', l: 'Vendas' }] as const"
                  :key="opt.v"
                  type="button"
                  class="rounded-lg px-3 py-1.5 transition-colors"
                  :class="chartMode === opt.v ? 'bg-default text-highlighted shadow-sm' : 'text-muted hover:text-highlighted'"
                  @click="chartMode = opt.v"
                >
                  {{ opt.l }}
                </button>
              </div>
              <div class="hidden sm:flex items-center gap-3 text-xs text-muted">
                <span class="flex items-center gap-1.5"><span class="size-2.5 rounded-sm bg-brand-700 dark:bg-brand-400" />{{ monthShort(0) }}</span>
                <span class="flex items-center gap-1.5"><span class="size-2.5 rounded-sm bg-accent-strong" />{{ monthShort(1) }}</span>
              </div>
            </div>
          </div>

          <!-- Barras: cada fruta com mês atual e anterior lado a lado -->
          <div class="mt-6 grid grid-cols-[3rem_1fr] gap-2">
            <div class="flex h-64 flex-col justify-between text-right text-[11px] text-dimmed">
              <span v-for="t in ticks" :key="t" class="-translate-y-1/2 first:translate-y-0 last:translate-y-0">{{ thousand(t) }}</span>
            </div>
            <div class="relative h-64">
              <div class="absolute inset-0 flex flex-col justify-between">
                <div v-for="t in ticks" :key="t" class="border-t border-dashed border-default" />
              </div>
              <div class="relative flex h-full items-end justify-around">
                <div v-for="item in series" :key="item.label" class="flex h-full items-end gap-1.5">
                  <UTooltip :text="`${item.label} · ${monthShort(0)}: ${item.current.toLocaleString('pt-BR')} kg`">
                    <div class="w-6 rounded-t-md bg-brand-700 sm:w-8 dark:bg-brand-400" :style="{ height: `${(item.current / maxValue) * 100}%` }" />
                  </UTooltip>
                  <UTooltip :text="`${item.label} · ${monthShort(1)}: ${item.previous.toLocaleString('pt-BR')} kg`">
                    <div class="w-6 rounded-t-md bg-accent-strong sm:w-8" :style="{ height: `${(item.previous / maxValue) * 100}%` }" />
                  </UTooltip>
                </div>
              </div>
            </div>
            <div />
            <div class="flex justify-around text-xs font-semibold text-highlighted">
              <span v-for="item in series" :key="item.label">{{ item.label }}</span>
            </div>
          </div>
          <p class="mt-4 text-xs text-muted">
            Em breve: clique numa fruta para ver o estoque por lote.
          </p>
        </section>

        <!-- Atenção do comercial -->
        <section class="rounded-2xl bg-default p-6 ring ring-default">
          <div class="flex items-start justify-between">
            <div>
              <h3 class="text-lg font-bold text-highlighted">
                Atenção do comercial
              </h3>
              <p class="text-sm text-muted">
                Solicitações que pedem uma resposta.
              </p>
            </div>
            <span class="flex size-10 items-center justify-center rounded-xl bg-accent-soft text-brand-800 dark:text-brand-200">
              <UIcon name="i-lucide-bell" class="size-5" />
            </span>
          </div>
          <ul class="mt-4 divide-y divide-default">
            <li v-for="item in attention" :key="item.code">
              <button type="button" class="flex w-full items-center gap-3 py-3.5 text-left hover:bg-elevated/50 rounded-lg px-1 -mx-1 transition-colors">
                <span class="flex size-10 shrink-0 items-center justify-center rounded-xl" :class="toneClasses[item.tone]">
                  <UIcon name="i-lucide-clipboard-list" class="size-5" />
                </span>
                <span class="min-w-0 flex-1">
                  <span class="block text-sm font-bold text-highlighted truncate">{{ item.customer }}</span>
                  <span class="block text-xs text-muted">{{ item.code }} · {{ item.status }}</span>
                </span>
                <UIcon name="i-lucide-chevron-right" class="size-4 text-dimmed" />
              </button>
            </li>
          </ul>
        </section>
      </div>
    </template>
  </UDashboardPanel>
</template>
