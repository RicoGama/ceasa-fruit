<script setup lang="ts">
import { z } from 'zod'
import type { DropdownMenuItem, FormSubmitEvent, TableColumn } from '@nuxt/ui'
import type { Vehicle } from '~/types'

definePageMeta({ layout: 'dashboard', permission: 'fleet.view' })
useHead({ title: 'Veículos · CEASA Fruit' })

const api = useApi()
const toast = useToast()
const auth = useAuthStore()
const route = useRoute()
const canEdit = computed(() => auth.can('fleet.edit'))

// --- Dados ----------------------------------------------------------------------
const { data, status, error, refresh } = await useAsyncData('vehicles', async () => {
  const { data, error } = await api.GET('/vehicles')
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data.items
})
const { data: drivers, refresh: refreshDrivers } = await useAsyncData('drivers-for-vehicles', async () => {
  const { data } = await api.GET('/drivers', { params: { query: { status: 'active' } } })
  return data?.items ?? []
})
useLiveRefresh('vehicles', refresh)
useLiveRefresh('drivers', refreshDrivers)

const showInactive = ref(false)
const search = ref('')
const vehicles = computed(() => {
  const q = search.value.trim().toLowerCase().replace('-', '')
  return (data.value ?? []).filter(v => (showInactive.value || v.active)
    && (!q || v.plate.toLowerCase().includes(q) || (v.description ?? '').toLowerCase().includes(q)
      || (v.driver?.name ?? '').toLowerCase().includes(q)))
})

const desktopOnly = { class: { th: 'hidden md:table-cell', td: 'hidden md:table-cell' } }
const columns: TableColumn<Vehicle>[] = [
  { accessorKey: 'plate', header: 'Veículo' },
  { id: 'driver', header: 'Motorista', meta: desktopOnly },
  { id: 'tracker', header: 'Rastreador', meta: desktopOnly },
  { accessorKey: 'status', header: 'Situação' },
  { id: 'actions' },
]

function rowActions(v: Vehicle): DropdownMenuItem[] {
  return [
    ...(v.trackerImei ? [{ label: 'Ver no mapa', icon: 'i-lucide-map', to: `/app/fleet?veiculo=${v.id}` }] : []),
    ...(canEdit.value ? [{ label: 'Editar', icon: 'i-lucide-pencil', onSelect: () => openEdit(v) }] : []),
    ...(canEdit.value && v.active
      ? [{ label: 'Inativar', icon: 'i-lucide-archive', color: 'error' as const, onSelect: () => deactivate(v) }]
      : []),
  ]
}

// --- Formulário -------------------------------------------------------------------
const plateRe = /^[A-Z]{3}[0-9][A-Z0-9][0-9]{2}$/
const normPlate = (s: string) => s.toUpperCase().replace(/[-\s]/g, '')
const schema = z.object({
  plate: z.string().refine(v => plateRe.test(normPlate(v)), 'Placa inválida (ex.: ABC-1234 ou ABC1D23)'),
  kind: z.enum(['truck', 'van', 'pickup', 'car', 'motorcycle', 'other']),
  description: textField(150),
  capacityKg: z.number().int().min(1, 'Mínimo 1 kg').max(100000, 'Máximo 100.000 kg').nullable(),
  driverId: z.number().nullable(),
  trackerImei: z.string().refine(v => !v || /^[0-9]{10,20}$/.test(onlyDigits(v)), 'IMEI inválido: são 15 números, na etiqueta do rastreador'),
  active: z.boolean(),
  notes: textField(1000),
})
type Schema = z.output<typeof schema>

const empty = (): Schema => ({ plate: '', kind: 'truck', description: '', capacityKg: null, driverId: null, trackerImei: '', active: true, notes: '' })
const modalOpen = ref(false)
const editing = ref<Vehicle | null>(null)
const form = reactive<Schema>(empty())
const formRef = useTemplateRef('formRef')
const saving = ref(false)

function openCreate() {
  editing.value = null
  Object.assign(form, empty())
  modalOpen.value = true
}
function openEdit(v: Vehicle) {
  editing.value = v
  Object.assign(form, {
    plate: formatPlate(v.plate), kind: v.kind, description: v.description ?? '', capacityKg: v.capacityKg ?? null,
    driverId: v.driver?.id ?? null, trackerImei: v.trackerImei ?? '', active: v.active, notes: v.notes ?? '',
  })
  modalOpen.value = true
}
// Link "Cadastrar veículo" do mapa abre direto o formulário.
onMounted(() => {
  if (route.query.novo && canEdit.value) openCreate()
})

async function onSubmit(event: FormSubmitEvent<Schema>) {
  const v = event.data
  const body = {
    plate: normPlate(v.plate),
    kind: v.kind,
    description: blankToUndefined(v.description),
    capacityKg: v.capacityKg ?? undefined,
    driverId: v.driverId ?? undefined,
    trackerImei: blankToUndefined(onlyDigits(v.trackerImei)),
    active: v.active,
    notes: blankToUndefined(v.notes),
  }
  saving.value = true
  const { error } = editing.value
    ? await api.PUT('/vehicles/{id}', { params: { path: { id: editing.value.id } }, body: { ...body, version: editing.value.version } })
    : await api.POST('/vehicles', { body })
  saving.value = false
  if (error) {
    if (isStaleError(error)) {
      modalOpen.value = false
      toast.add({ title: apiErrorMessage(error), description: 'A lista foi atualizada; abra o veículo de novo.', color: 'warning' })
      await refresh()
      return
    }
    const fieldErrors = apiFieldErrors(error)
    if (fieldErrors.length) formRef.value?.setErrors(fieldErrors)
    else toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: editing.value ? 'Veículo atualizado' : 'Veículo cadastrado', color: 'success' })
  modalOpen.value = false
  await refresh()
}

const confirm = useConfirm()
async function deactivate(v: Vehicle) {
  const yes = await confirm({
    title: `Inativar ${formatPlate(v.plate)}?`,
    description: 'Ele sai do mapa e das listas, mas o histórico de trajetos fica guardado. Dá para reativar editando o veículo.',
    confirmLabel: 'Inativar',
    color: 'error',
    icon: 'i-lucide-archive',
  })
  if (!yes) return
  const { error } = await api.DELETE('/vehicles/{id}', { params: { path: { id: v.id } } })
  if (error) toast.add({ title: apiErrorMessage(error), color: 'error' })
  else toast.add({ title: 'Veículo inativado', color: 'success' })
}

const kindOptions = toOptions(vehicleKindLabels)
const driverOptions = computed(() => [
  { label: 'Nenhum', value: null },
  ...(drivers.value ?? []).map(d => ({ label: d.name, value: d.id })),
])
</script>

<template>
  <UDashboardPanel id="vehicles">
    <template #header>
      <AppNavbar>
        <template #right>
          <UButton v-if="canEdit" icon="i-lucide-plus" aria-label="Novo veículo" @click="openCreate">
            <span class="hidden sm:inline">Novo veículo</span>
          </UButton>
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <UInput v-model="search" icon="i-lucide-search" placeholder="Placa, descrição ou motorista" class="w-full sm:max-w-xs" />
        <USwitch v-model="showInactive" label="Mostrar inativos" />
      </div>

      <UAlert
        v-if="error"
        color="error"
        variant="subtle"
        icon="i-lucide-circle-alert"
        title="Não foi possível carregar os veículos"
        :description="error.message"
        :actions="[{ label: 'Tentar de novo', color: 'error', variant: 'outline', onClick: () => refresh() }]"
      />

      <UTable v-else :data="vehicles" :columns="columns" :loading="status === 'pending'" class="shrink-0">
        <template #plate-cell="{ row }">
          <button type="button" class="block text-left group" :disabled="!canEdit" @click="openEdit(row.original)">
            <p class="font-mono font-semibold text-highlighted group-hover:text-primary">
              {{ formatPlate(row.original.plate) }}
              <UBadge v-if="!row.original.active" label="Inativo" color="neutral" variant="subtle" size="sm" class="ml-1 font-sans" />
            </p>
            <p class="text-xs text-muted">
              {{ row.original.description || vehicleKindLabels[row.original.kind] }}
              <span v-if="row.original.capacityKg"> · {{ row.original.capacityKg.toLocaleString('pt-BR') }} kg</span>
            </p>
            <p v-if="row.original.driver" class="text-xs text-muted md:hidden">
              {{ row.original.driver.name }}
            </p>
          </button>
        </template>
        <template #driver-cell="{ row }">
          {{ row.original.driver?.name ?? '—' }}
        </template>
        <template #tracker-cell="{ row }">
          <span v-if="row.original.trackerImei" class="font-mono text-sm">{{ row.original.trackerImei }}</span>
          <span v-else class="text-dimmed">Sem rastreador</span>
        </template>
        <template #status-cell="{ row }">
          <div class="flex flex-col items-start gap-0.5">
            <UBadge :label="vehicleStatusLabels[row.original.status]" :color="vehicleStatusColors[row.original.status]" variant="subtle" />
            <span v-if="row.original.lastPosition" class="text-xs text-muted">{{ formatWhen(row.original.lastPosition.fixAt) }}</span>
          </div>
        </template>
        <template #actions-cell="{ row }">
          <div class="text-right">
            <UDropdownMenu :items="rowActions(row.original)" :content="{ align: 'end' }">
              <UButton icon="i-lucide-ellipsis-vertical" color="neutral" variant="ghost" aria-label="Ações" />
            </UDropdownMenu>
          </div>
        </template>
        <template #empty>
          <UEmpty
            icon="i-lucide-car-front"
            title="Nenhum veículo"
            description="Cadastre os caminhões da entrega. Com o IMEI do rastreador, eles aparecem no mapa."
            :actions="canEdit ? [{ label: 'Cadastrar veículo', icon: 'i-lucide-plus', onClick: openCreate }] : []"
          />
        </template>
      </UTable>
    </template>
  </UDashboardPanel>

  <UModal v-model:open="modalOpen" :title="editing ? `Editar ${formatPlate(editing.plate)}` : 'Novo veículo'">
    <template #body>
      <UForm id="vehicle-form" ref="formRef" :schema="schema" :state="form" class="grid grid-cols-1 gap-4 sm:grid-cols-2" @submit="onSubmit">
        <UFormField label="Placa" name="plate" required>
          <UInput v-model="form.plate" placeholder="ABC1D23" class="w-full font-mono uppercase" @blur="form.plate = formatPlate(normPlate(form.plate))" />
        </UFormField>
        <UFormField label="Tipo" name="kind">
          <USelect v-model="form.kind" :items="kindOptions" class="w-full" />
        </UFormField>
        <UFormField label="Descrição" name="description" class="sm:col-span-2" help="Marca, modelo, carroceria">
          <UInput v-model="form.description" placeholder="Ex.: Volvo VM 270 baú refrigerado" class="w-full" />
        </UFormField>
        <UFormField label="Capacidade (kg)" name="capacityKg">
          <UInputNumber v-model="form.capacityKg" :min="1" :max="100000" :step="100" class="w-full" />
        </UFormField>
        <UFormField label="Motorista habitual" name="driverId">
          <USelect v-model="form.driverId" :items="driverOptions" class="w-full" />
        </UFormField>
        <UFormField label="IMEI do rastreador" name="trackerImei" class="sm:col-span-2" help="Os 15 números da etiqueta do rastreador. Vazio = veículo sem rastreador.">
          <UInput v-model="form.trackerImei" inputmode="numeric" placeholder="864275079000001" class="w-full font-mono" />
        </UFormField>
        <UFormField label="Observações" name="notes" class="sm:col-span-2">
          <UTextarea v-model="form.notes" :rows="2" class="w-full" />
        </UFormField>
        <USwitch v-if="editing" v-model="form.active" label="Ativo" class="sm:col-span-2" />
      </UForm>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancelar" color="neutral" variant="ghost" @click="modalOpen = false" />
        <UButton type="submit" form="vehicle-form" label="Salvar" icon="i-lucide-check" :loading="saving" />
      </div>
    </template>
  </UModal>
</template>
