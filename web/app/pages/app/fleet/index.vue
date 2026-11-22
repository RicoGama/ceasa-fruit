<script setup lang="ts">
import type { TrackPoint, Vehicle } from '~/types'

definePageMeta({ layout: 'dashboard', permission: 'fleet.view' })
useHead({ title: 'Mapa da frota · CEASA Fruit' })

const api = useApi()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

// --- Veículos e última posição ---------------------------------------------------
const { data, error, refresh } = await useAsyncData('fleet-live', async () => {
  const { data, error } = await api.GET('/fleet/live')
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
})
const vehicles = computed(() => data.value?.vehicles ?? [])
const sync = computed(() => data.value?.sync)

// Tempo real: posição nova, veículo alterado ou a sincronização mudou de situação.
useLiveRefresh(['vehicles', 'fleet_sync'], refresh)
// O "há 3 min" e o "sem sinal" mudam com o tempo, mesmo sem posição nova.
const now = ref(new Date())
const ticker = setInterval(() => {
  now.value = new Date()
}, 30_000)
const reloader = setInterval(() => refresh(), 60_000)
onScopeDispose(() => {
  clearInterval(ticker)
  clearInterval(reloader)
})

// --- Seleção (fica na URL: ?veiculo=12) ------------------------------------------------
const selectedId = computed<number | null>({
  get: () => (route.query.veiculo ? Number(route.query.veiculo) : null),
  set: id => router.replace({ query: { ...route.query, veiculo: id ?? undefined, dia: id ? route.query.dia : undefined } }),
})
const selected = computed(() => vehicles.value.find(v => v.id === selectedId.value) ?? null)

// Ordem da lista: andando, parado ligado, parado, sem sinal...
const order = ['moving', 'idle', 'stopped', 'offline', 'waiting', 'no_tracker'] as const
const sorted = computed(() => [...vehicles.value].sort((a, b) =>
  order.indexOf(a.status) - order.indexOf(b.status) || a.plate.localeCompare(b.plate)))
const counts = computed(() => {
  const c: Partial<Record<Vehicle['status'], number>> = {}
  for (const v of vehicles.value) c[v.status] = (c[v.status] ?? 0) + 1
  return c
})

// --- Trajeto do dia ------------------------------------------------------------------------
const todayIso = () => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
const showTrack = computed({
  get: () => !!route.query.dia,
  set: on => router.replace({ query: { ...route.query, dia: on ? todayIso() : undefined } }),
})
const trackDay = computed({
  get: () => (route.query.dia as string) || todayIso(),
  set: d => router.replace({ query: { ...route.query, dia: d || todayIso() } }),
})
const track = ref<{ points: TrackPoint[], distanceKm: number, maxSpeedKmh: number, movingMinutes: number } | null>(null)
const trackLoading = ref(false)
const trackError = ref('')

async function loadTrack() {
  trackError.value = ''
  if (!selectedId.value || !showTrack.value) {
    track.value = null
    return
  }
  // O dia inteiro no horário do Rio: 00:00 até 00:00 do dia seguinte.
  const from = new Date(`${trackDay.value}T00:00:00`)
  const to = new Date(from)
  to.setDate(to.getDate() + 1)
  trackLoading.value = true
  const { data, error } = await api.GET('/vehicles/{id}/track', {
    params: { path: { id: selectedId.value }, query: { from: from.toISOString(), to: to.toISOString() } },
  })
  trackLoading.value = false
  if (error) {
    trackError.value = apiErrorMessage(error)
    track.value = null
    return
  }
  track.value = data
}
watch([selectedId, showTrack, trackDay], loadTrack, { immediate: true })
// Trajeto de hoje cresce sozinho conforme o caminhão anda.
useRealtime('vehicle_positions', (ev) => {
  if (showTrack.value && ev.id === selectedId.value && trackDay.value === todayIso()) loadTrack()
})

// --- Fundo do mapa -------------------------------------------------------------------------
const basemapOk = ref(true)

function googleMapsUrl(v: Vehicle) {
  const p = v.lastPosition!
  return `https://www.google.com/maps/search/?api=1&query=${p.latitude},${p.longitude}`
}

function minutesText(min: number) {
  if (min < 60) return `${min} min`
  return `${Math.floor(min / 60)} h ${String(min % 60).padStart(2, '0')} min`
}
</script>

<template>
  <UDashboardPanel id="fleet" :ui="{ body: 'p-0 sm:p-0 gap-0' }">
    <template #header>
      <AppNavbar>
        <template #right>
          <UButton v-if="auth.can('fleet.edit')" label="Veículos" icon="i-lucide-car-front" color="neutral" variant="outline" to="/app/fleet/vehicles" class="hidden sm:inline-flex" />
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <!-- Situação da sincronização com o servidor de rastreamento -->
      <UAlert
        v-if="sync && !sync.configured"
        color="info"
        variant="subtle"
        icon="i-lucide-satellite-dish"
        title="Rastreamento ainda não configurado"
        description="Os veículos podem ser cadastrados, mas as posições só aparecem depois de ligar o servidor de rastreamento (TRACCAR_URL e TRACCAR_TOKEN na configuração do servidor)."
        class="m-3 mb-0 w-auto"
      />
      <UAlert
        v-else-if="sync?.lastError"
        color="warning"
        variant="subtle"
        icon="i-lucide-wifi-off"
        :title="sync.lastError"
        :description="`Desde ${sync.lastErrorAt ? formatWhen(sync.lastErrorAt) : 'agora'}. Os rastreadores continuam gravando: quando a conexão voltar, os trajetos se completam sozinhos.${sync.lastSuccessAt ? ` Última atualização: ${formatWhen(sync.lastSuccessAt)}.` : ''}`"
        class="m-3 mb-0 w-auto"
      />
      <UAlert
        v-if="error"
        color="error"
        variant="subtle"
        icon="i-lucide-circle-alert"
        title="Não foi possível carregar a frota"
        :description="error.message"
        :actions="[{ label: 'Tentar de novo', color: 'error', variant: 'outline', onClick: () => refresh() }]"
        class="m-3 w-auto"
      />

      <UEmpty
        v-else-if="!vehicles.length"
        icon="i-lucide-truck"
        title="Nenhum veículo cadastrado"
        description="Cadastre os caminhões com o IMEI do rastreador para vê-los no mapa."
        :actions="auth.can('fleet.edit') ? [{ label: 'Cadastrar veículo', icon: 'i-lucide-plus', to: '/app/fleet/vehicles?novo=1' }] : []"
        class="m-6"
      />

      <div v-else class="flex min-h-0 flex-1 flex-col lg:flex-row">
        <!-- Mapa (em cima no celular, à direita no computador) -->
        <div class="relative h-[55vh] shrink-0 lg:order-2 lg:h-auto lg:flex-1">
          <FleetMap
            :vehicles="vehicles"
            :selected-id="selectedId"
            :track="showTrack ? track?.points : null"
            @select="selectedId = $event"
            @basemap="basemapOk = $event"
          />
          <p v-if="!basemapOk" class="absolute left-2 right-14 top-2 w-fit rounded-md bg-default/90 px-2 py-1 text-xs text-muted shadow">
            Mapa de fundo sem internet: os veículos continuam no lugar certo.
          </p>

          <!-- Detalhe do veículo escolhido -->
          <div v-if="selected" class="absolute inset-x-2 bottom-2 z-10 max-h-[70%] overflow-y-auto rounded-xl border border-default bg-default p-4 shadow-lg sm:left-auto sm:right-3 sm:w-96">
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <p class="font-mono text-lg font-bold text-highlighted">
                  {{ formatPlate(selected.plate) }}
                </p>
                <p class="truncate text-sm text-muted">
                  {{ selected.description || vehicleKindLabels[selected.kind] }}
                  <span v-if="selected.driver"> · {{ selected.driver.name }}</span>
                </p>
              </div>
              <UButton icon="i-lucide-x" color="neutral" variant="ghost" size="sm" aria-label="Fechar" @click="selectedId = null" />
            </div>

            <div class="mt-3 flex flex-wrap items-center gap-2">
              <UBadge :label="vehicleStatusLabels[selected.status]" :color="vehicleStatusColors[selected.status]" variant="subtle" />
              <span v-if="selected.lastPosition" class="text-sm text-muted">
                {{ Math.round(selected.lastPosition.speedKmh) }} km/h ·
                {{ formatAgo(selected.lastPosition.fixAt, now) }} ({{ formatWhen(selected.lastPosition.fixAt) }})
              </span>
            </div>
            <p v-if="selected.status === 'waiting'" class="mt-2 text-sm text-muted">
              Rastreador {{ selected.trackerImei }} cadastrado. A primeira posição aparece quando ele for configurado para mandar ao servidor de rastreamento.
            </p>

            <div v-if="selected.lastPosition" class="mt-3 flex flex-wrap gap-2">
              <USwitch v-model="showTrack" label="Trajeto do dia" />
              <UButton :to="googleMapsUrl(selected)" target="_blank" label="Google Maps" icon="i-lucide-external-link" color="neutral" variant="ghost" size="xs" />
            </div>

            <div v-if="showTrack" class="mt-3 space-y-2 border-t border-default pt-3">
              <UInput v-model="trackDay" type="date" :max="todayIso()" size="sm" class="w-40" aria-label="Dia do trajeto" />
              <p v-if="trackLoading" class="text-sm text-muted">
                Carregando o trajeto…
              </p>
              <p v-else-if="trackError" class="text-sm text-error">
                {{ trackError }}
              </p>
              <p v-else-if="track && track.points.length < 2" class="text-sm text-muted">
                Nenhum trajeto neste dia.
              </p>
              <dl v-else-if="track" class="grid grid-cols-3 gap-2 text-center">
                <div class="rounded-lg bg-elevated p-2">
                  <dt class="text-xs text-muted">Distância</dt>
                  <dd class="font-semibold text-highlighted">{{ track.distanceKm.toLocaleString('pt-BR') }} km</dd>
                </div>
                <div class="rounded-lg bg-elevated p-2">
                  <dt class="text-xs text-muted">Andando</dt>
                  <dd class="font-semibold text-highlighted">{{ minutesText(track.movingMinutes) }}</dd>
                </div>
                <div class="rounded-lg bg-elevated p-2">
                  <dt class="text-xs text-muted">Máxima</dt>
                  <dd class="font-semibold text-highlighted">{{ Math.round(track.maxSpeedKmh) }} km/h</dd>
                </div>
              </dl>
            </div>
          </div>
        </div>

        <!-- Lista de veículos -->
        <aside class="min-h-0 border-default lg:order-1 lg:w-80 lg:shrink-0 lg:overflow-y-auto lg:border-r">
          <div class="flex flex-wrap gap-1.5 border-b border-default p-3">
            <template v-for="s in order" :key="s">
              <UBadge v-if="counts[s]" :label="`${counts[s]} ${vehicleStatusLabels[s].toLowerCase()}`" :color="vehicleStatusColors[s]" variant="subtle" size="sm" />
            </template>
          </div>
          <ul class="divide-y divide-default">
            <li v-for="v in sorted" :key="v.id">
              <button
                type="button"
                class="flex w-full items-center gap-3 px-3 py-2.5 text-left hover:bg-elevated/60"
                :class="v.id === selectedId ? 'bg-elevated' : ''"
                @click="selectedId = v.id"
              >
                <span class="size-2.5 shrink-0 rounded-full" :style="{ background: vehicleStatusHex[v.status] }" />
                <span class="min-w-0 flex-1">
                  <span class="block font-mono font-semibold text-highlighted">{{ formatPlate(v.plate) }}</span>
                  <span class="block truncate text-xs text-muted">{{ v.driver?.name || v.description || vehicleKindLabels[v.kind] }}</span>
                </span>
                <span class="shrink-0 text-right text-xs">
                  <span class="block font-medium" :class="v.status === 'moving' ? 'text-success' : 'text-muted'">
                    {{ v.status === 'moving' ? `${Math.round(v.lastPosition!.speedKmh)} km/h` : vehicleStatusLabels[v.status] }}
                  </span>
                  <span v-if="v.lastPosition" class="block text-dimmed">{{ formatAgo(v.lastPosition.fixAt, now) }}</span>
                </span>
              </button>
            </li>
          </ul>
        </aside>
      </div>
    </template>
  </UDashboardPanel>
</template>
