<script setup lang="ts">
import * as maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
// O MapLibre desenha linhas e áreas num "worker" (outro processo do
// navegador) e procura o arquivo dele ao lado do próprio código. Com o Vite
// juntando tudo num pacote, esse arquivo fica para trás: indicamos o endereço.
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url'
import type { TrackPoint, Vehicle } from '~/types'

// Mapa da frota (MapLibre). Só roda no navegador (.client.vue).
//
// O fundo do mapa (ruas) vem da internet (OpenFreeMap). Se não carregar, o
// mapa troca para um fundo neutro: os caminhões e o trajeto continuam
// aparecendo no lugar certo, só sem as ruas por baixo.
maplibregl.setWorkerUrl(workerUrl)

const props = defineProps<{
  vehicles: Vehicle[]
  selectedId?: number | null
  track?: TrackPoint[] | null
}>()
const emit = defineEmits<{ select: [id: number], basemap: [ok: boolean] }>()

const el = useTemplateRef('el')
const styleUrl = useRuntimeConfig().public.mapStyleUrl as string
let map: maplibregl.Map | null = null
// Mapa pronto para receber camadas (o evento "load" já aconteceu).
let ready = false
const markers = new Map<number, { marker: maplibregl.Marker, el: HTMLElement }>()
let fitted = false
let trackMarkers: maplibregl.Marker[] = []

// CEASA-RJ (Irajá): onde o mapa abre antes de haver posições.
const CEASA: [number, number] = [-43.3445, -22.8277]

const blankStyle: maplibregl.StyleSpecification = {
  version: 8,
  sources: {},
  layers: [{ id: 'fundo', type: 'background', paint: { 'background-color': '#e6ebf0' } }],
}

// Confere se o estilo do mapa responde (sem internet, usa o fundo neutro).
async function pickStyle(): Promise<string | maplibregl.StyleSpecification> {
  try {
    const res = await fetch(styleUrl, { signal: AbortSignal.timeout(5000) })
    if (res.ok) {
      emit('basemap', true)
      return styleUrl
    }
  }
  catch { /* sem internet */ }
  emit('basemap', false)
  return blankStyle
}

let unmounted = false
onMounted(async () => {
  // Primeiro descobre se o fundo do mapa está acessível (sem internet, usa o
  // neutro). A tela pode ter saído enquanto isso: confere antes de desenhar.
  const style = await pickStyle()
  if (unmounted || !el.value) return
  map = new maplibregl.Map({
    container: el.value,
    style,
    center: CEASA,
    zoom: 11,
    attributionControl: { compact: true },
  })
  map.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'top-right')
  // Em desenvolvimento, o mapa fica acessível no console (window.__fleetMap).
  if (import.meta.dev) (window as unknown as { __fleetMap: maplibregl.Map }).__fleetMap = map
  map.on('load', () => {
    ready = true
    syncMarkers()
    drawTrack()
  })
})

onBeforeUnmount(() => {
  unmounted = true
  ready = false
  map?.remove()
  map = null
})

// --- Marcadores dos veículos ----------------------------------------------------
function markerHtml(v: Vehicle) {
  const color = vehicleStatusHex[v.status]
  const moving = v.status === 'moving'
  const course = v.lastPosition?.course ?? 0
  // Andando: seta apontando para onde vai. Parado: bolinha.
  const icon = moving
    ? `<svg viewBox="0 0 24 24" width="22" height="22" style="transform: rotate(${course}deg)"><path d="M12 2 20 21 12 16 4 21Z" fill="${color}" stroke="white" stroke-width="2" stroke-linejoin="round"/></svg>`
    : `<svg viewBox="0 0 24 24" width="18" height="18"><circle cx="12" cy="12" r="8" fill="${color}" stroke="white" stroke-width="3"/></svg>`
  return `${icon}<span class="fleet-plate">${formatPlate(v.plate)}</span>`
}

function syncMarkers() {
  if (!map) return
  const seen = new Set<number>()
  for (const v of props.vehicles) {
    const p = v.lastPosition
    if (!p) continue
    seen.add(v.id)
    let m = markers.get(v.id)
    if (!m) {
      const div = document.createElement('button')
      div.type = 'button'
      div.className = 'fleet-marker'
      div.setAttribute('aria-label', `Veículo ${formatPlate(v.plate)}`)
      div.addEventListener('click', (e) => {
        e.stopPropagation()
        emit('select', v.id)
      })
      m = { marker: new maplibregl.Marker({ element: div }).setLngLat([p.longitude, p.latitude]).addTo(map), el: div }
      markers.set(v.id, m)
    }
    m.marker.setLngLat([p.longitude, p.latitude])
    m.el.innerHTML = markerHtml(v)
    m.el.classList.toggle('is-selected', v.id === props.selectedId)
    m.el.classList.toggle('is-stale', v.status === 'offline')
  }
  for (const [id, m] of markers) {
    if (!seen.has(id)) {
      m.marker.remove()
      markers.delete(id)
    }
  }
  // Na primeira vez, enquadra todos os veículos.
  if (!fitted && seen.size) {
    fitted = true
    const b = new maplibregl.LngLatBounds()
    for (const v of props.vehicles) if (v.lastPosition) b.extend([v.lastPosition.longitude, v.lastPosition.latitude])
    map.fitBounds(b, { padding: 80, maxZoom: 14, duration: 0 })
  }
}

watch(() => props.vehicles, syncMarkers, { deep: true })

// Selecionou um veículo na lista: centraliza nele (se não estiver vendo o trajeto).
watch(() => props.selectedId, (id) => {
  syncMarkers()
  const p = props.vehicles.find(v => v.id === id)?.lastPosition
  if (map && p && !props.track?.length) map.flyTo({ center: [p.longitude, p.latitude], zoom: Math.max(map.getZoom(), 14) })
})

// --- Trajeto -------------------------------------------------------------------------
function drawTrack() {
  if (!map || !ready) return
  for (const m of trackMarkers) m.remove()
  trackMarkers = []
  const pts = props.track ?? []
  const data: maplibregl.GeoJSONSourceSpecification['data'] = {
    type: 'Feature',
    properties: {},
    geometry: { type: 'LineString', coordinates: pts.map(p => [p.lng, p.lat]) },
  }
  const source = map.getSource('trajeto') as maplibregl.GeoJSONSource | undefined
  if (source) source.setData(data)
  else {
    map.addSource('trajeto', { type: 'geojson', data })
    map.addLayer({
      id: 'trajeto',
      type: 'line',
      source: 'trajeto',
      layout: { 'line-join': 'round', 'line-cap': 'round' },
      paint: { 'line-color': '#1d4ed8', 'line-width': 4, 'line-opacity': 0.85 },
    })
  }
  if (pts.length < 2) return
  const dot = (color: string, label: string) => {
    const d = document.createElement('div')
    d.className = 'fleet-track-dot'
    d.title = label
    d.style.background = color
    return d
  }
  const first = pts[0]!
  const last = pts[pts.length - 1]!
  trackMarkers.push(
    new maplibregl.Marker({ element: dot('#16a34a', 'Início') }).setLngLat([first.lng, first.lat]).addTo(map),
    new maplibregl.Marker({ element: dot('#dc2626', 'Fim') }).setLngLat([last.lng, last.lat]).addTo(map),
  )
  const b = new maplibregl.LngLatBounds()
  for (const p of pts) b.extend([p.lng, p.lat])
  map.fitBounds(b, { padding: 60, maxZoom: 15 })
}

watch(() => props.track, drawTrack)
</script>

<template>
  <div ref="el" class="size-full" />
</template>

<style>
/* Veículos na frente dos pontos de início/fim do trajeto. */
.maplibregl-marker.fleet-marker {
  z-index: 2;
}
.maplibregl-marker.fleet-track-dot {
  z-index: 1;
}
.fleet-marker {
  display: flex;
  align-items: center;
  gap: 4px;
  background: none;
  border: 0;
  padding: 0;
  cursor: pointer;
}
.fleet-marker .fleet-plate {
  font: 600 11px/1 ui-sans-serif, system-ui, sans-serif;
  background: white;
  color: #111827;
  border-radius: 4px;
  padding: 3px 5px;
  box-shadow: 0 1px 3px rgb(0 0 0 / 0.3);
  white-space: nowrap;
}
.fleet-marker.is-selected .fleet-plate {
  background: #1d4ed8;
  color: white;
}
.fleet-marker.is-stale {
  opacity: 0.6;
}
.fleet-track-dot {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 3px solid white;
  box-shadow: 0 1px 3px rgb(0 0 0 / 0.4);
}
</style>
