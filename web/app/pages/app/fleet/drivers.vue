<script setup lang="ts">
import { z } from 'zod'
import type { DropdownMenuItem, FormSubmitEvent, TableColumn } from '@nuxt/ui'
import type { Driver } from '~/types'

definePageMeta({ layout: 'dashboard', permission: 'fleet.view' })
useHead({ title: 'Motoristas · CEASA Fruit' })

const api = useApi()
const toast = useToast()
const auth = useAuthStore()
const canEdit = computed(() => auth.can('fleet.edit'))

const { data, status, error, refresh } = await useAsyncData('drivers', async () => {
  const { data, error } = await api.GET('/drivers')
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data.items
})
useLiveRefresh(['drivers', 'vehicles'], refresh)

const showInactive = ref(false)
const drivers = computed(() => (data.value ?? []).filter(d => showInactive.value || d.active))

// --- CNH: vencida ou vencendo em 30 dias ----------------------------------------------
function licenseStatus(d: Driver): { label: string, color: 'error' | 'warning' | 'neutral' } | null {
  if (!d.licenseExpiresOn) return null
  const expires = new Date(`${d.licenseExpiresOn}T23:59:59`)
  const days = Math.ceil((expires.getTime() - Date.now()) / 86_400_000)
  if (days < 0) return { label: 'CNH vencida', color: 'error' }
  if (days <= 30) return { label: days === 0 ? 'CNH vence hoje' : `CNH vence em ${days} dia${days > 1 ? 's' : ''}`, color: 'warning' }
  return null
}
const formatDay = (iso: string) => new Date(`${iso}T12:00:00`).toLocaleDateString('pt-BR')

const desktopOnly = { class: { th: 'hidden md:table-cell', td: 'hidden md:table-cell' } }
const columns: TableColumn<Driver>[] = [
  { accessorKey: 'name', header: 'Motorista' },
  { id: 'license', header: 'CNH' },
  { accessorKey: 'vehicleCount', header: 'Veículos', meta: desktopOnly },
  { id: 'actions' },
]

function rowActions(d: Driver): DropdownMenuItem[] {
  if (!canEdit.value) return []
  return [
    { label: 'Editar', icon: 'i-lucide-pencil', onSelect: () => openEdit(d) },
    ...(d.active ? [{ label: 'Inativar', icon: 'i-lucide-archive', color: 'error' as const, onSelect: () => deactivate(d) }] : []),
  ]
}

// --- Formulário --------------------------------------------------------------------------
const schema = z.object({
  name: z.string().trim().min(2, 'Informe o nome').max(100),
  phone: textField(20).refine(v => !v || (onlyDigits(v).length >= 8 && onlyDigits(v).length <= 13), 'Telefone inválido'),
  document: z.string().refine(v => !v || (onlyDigits(v).length === 11 && isValidDocument(v)), 'CPF inválido'),
  licenseNumber: textField(20),
  licenseCategory: z.enum(licenseCategories).optional(),
  licenseExpiresOn: z.string(),
  active: z.boolean(),
  notes: textField(1000),
})
type Schema = z.output<typeof schema>

const empty = (): Schema => ({ name: '', phone: '', document: '', licenseNumber: '', licenseCategory: undefined, licenseExpiresOn: '', active: true, notes: '' })
const modalOpen = ref(false)
const editing = ref<Driver | null>(null)
const form = reactive<Schema>(empty())
const formRef = useTemplateRef('formRef')
const saving = ref(false)

function openCreate() {
  editing.value = null
  Object.assign(form, empty())
  modalOpen.value = true
}
function openEdit(d: Driver) {
  editing.value = d
  Object.assign(form, {
    name: d.name, phone: formatPhone(d.phone), document: d.document ? formatDocument(d.document) : '',
    licenseNumber: d.licenseNumber ?? '', licenseCategory: (d.licenseCategory ?? undefined) as Schema['licenseCategory'],
    licenseExpiresOn: d.licenseExpiresOn ?? '', active: d.active, notes: d.notes ?? '',
  })
  modalOpen.value = true
}

async function onSubmit(event: FormSubmitEvent<Schema>) {
  const v = event.data
  const body = {
    name: v.name.trim(),
    phone: blankToUndefined(v.phone),
    document: blankToUndefined(v.document),
    licenseNumber: blankToUndefined(v.licenseNumber),
    licenseCategory: v.licenseCategory || undefined,
    licenseExpiresOn: blankToUndefined(v.licenseExpiresOn),
    active: v.active,
    notes: blankToUndefined(v.notes),
  }
  saving.value = true
  const { error } = editing.value
    ? await api.PUT('/drivers/{id}', { params: { path: { id: editing.value.id } }, body: { ...body, version: editing.value.version } })
    : await api.POST('/drivers', { body })
  saving.value = false
  if (error) {
    if (isStaleError(error)) {
      modalOpen.value = false
      toast.add({ title: apiErrorMessage(error), description: 'A lista foi atualizada; abra o motorista de novo.', color: 'warning' })
      await refresh()
      return
    }
    const fieldErrors = apiFieldErrors(error)
    if (fieldErrors.length) formRef.value?.setErrors(fieldErrors)
    else toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: editing.value ? 'Motorista atualizado' : 'Motorista cadastrado', color: 'success' })
  modalOpen.value = false
  await refresh()
}

const confirm = useConfirm()
async function deactivate(d: Driver) {
  const yes = await confirm({
    title: `Inativar ${d.name}?`,
    description: 'Ele sai das listas de escolha. Os veículos que têm ele como motorista habitual continuam como estão.',
    confirmLabel: 'Inativar',
    color: 'error',
    icon: 'i-lucide-archive',
  })
  if (!yes) return
  const { error } = await api.DELETE('/drivers/{id}', { params: { path: { id: d.id } } })
  if (error) toast.add({ title: apiErrorMessage(error), color: 'error' })
  else toast.add({ title: 'Motorista inativado', color: 'success' })
}

const categoryOptions = licenseCategories.map(c => ({ label: c, value: c }))
</script>

<template>
  <UDashboardPanel id="drivers">
    <template #header>
      <AppNavbar>
        <template #right>
          <UButton v-if="canEdit" icon="i-lucide-plus" aria-label="Novo motorista" @click="openCreate">
            <span class="hidden sm:inline">Novo motorista</span>
          </UButton>
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <div class="flex items-center justify-between gap-3">
        <p class="text-sm text-muted">
          Quem dirige os veículos. A validade da CNH avisa 30 dias antes de vencer.
        </p>
        <USwitch v-model="showInactive" label="Mostrar inativos" />
      </div>

      <UAlert
        v-if="error"
        color="error"
        variant="subtle"
        icon="i-lucide-circle-alert"
        title="Não foi possível carregar os motoristas"
        :description="error.message"
        :actions="[{ label: 'Tentar de novo', color: 'error', variant: 'outline', onClick: () => refresh() }]"
      />

      <UTable v-else :data="drivers" :columns="columns" :loading="status === 'pending'" class="shrink-0">
        <template #name-cell="{ row }">
          <button type="button" class="block text-left group" :disabled="!canEdit" @click="openEdit(row.original)">
            <p class="font-medium text-highlighted group-hover:text-primary">
              {{ row.original.name }}
              <UBadge v-if="!row.original.active" label="Inativo" color="neutral" variant="subtle" size="sm" class="ml-1" />
            </p>
            <p v-if="row.original.phone" class="text-xs text-muted">
              {{ formatPhone(row.original.phone) }}
            </p>
          </button>
        </template>
        <template #license-cell="{ row }">
          <div class="flex flex-col items-start gap-0.5">
            <span v-if="row.original.licenseCategory || row.original.licenseExpiresOn" class="text-sm">
              <span v-if="row.original.licenseCategory">Categoria {{ row.original.licenseCategory }}</span>
              <span v-if="row.original.licenseExpiresOn" class="text-muted"> · até {{ formatDay(row.original.licenseExpiresOn) }}</span>
            </span>
            <span v-else class="text-dimmed">—</span>
            <UBadge v-if="licenseStatus(row.original)" :label="licenseStatus(row.original)!.label" :color="licenseStatus(row.original)!.color" variant="subtle" size="sm" />
          </div>
        </template>
        <template #vehicleCount-cell="{ row }">
          {{ row.original.vehicleCount || '—' }}
        </template>
        <template #actions-cell="{ row }">
          <div v-if="canEdit" class="text-right">
            <UDropdownMenu :items="rowActions(row.original)" :content="{ align: 'end' }">
              <UButton icon="i-lucide-ellipsis-vertical" color="neutral" variant="ghost" aria-label="Ações" />
            </UDropdownMenu>
          </div>
        </template>
        <template #empty>
          <UEmpty
            icon="i-lucide-id-card"
            title="Nenhum motorista"
            :actions="canEdit ? [{ label: 'Cadastrar motorista', icon: 'i-lucide-plus', onClick: openCreate }] : []"
          />
        </template>
      </UTable>
    </template>
  </UDashboardPanel>

  <UModal v-model:open="modalOpen" :title="editing ? `Editar ${editing.name}` : 'Novo motorista'">
    <template #body>
      <UForm id="driver-form" ref="formRef" :schema="schema" :state="form" class="grid grid-cols-1 gap-4 sm:grid-cols-2" @submit="onSubmit">
        <UFormField label="Nome" name="name" required class="sm:col-span-2">
          <UInput v-model="form.name" class="w-full" />
        </UFormField>
        <UFormField label="Telefone / WhatsApp" name="phone">
          <UInput v-model="form.phone" type="tel" placeholder="(21) 99999-0000" class="w-full" @blur="form.phone = formatPhone(form.phone)" />
        </UFormField>
        <UFormField label="CPF" name="document">
          <UInput v-model="form.document" placeholder="000.000.000-00" class="w-full" @blur="form.document = formatDocument(form.document)" />
        </UFormField>
        <UFormField label="Número da CNH" name="licenseNumber">
          <UInput v-model="form.licenseNumber" class="w-full" />
        </UFormField>
        <UFormField label="Categoria" name="licenseCategory">
          <USelect v-model="form.licenseCategory" :items="categoryOptions" placeholder="Selecione" class="w-full" />
        </UFormField>
        <UFormField label="Validade da CNH" name="licenseExpiresOn">
          <UInput v-model="form.licenseExpiresOn" type="date" class="w-full" />
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
        <UButton type="submit" form="driver-form" label="Salvar" icon="i-lucide-check" :loading="saving" />
      </div>
    </template>
  </UModal>
</template>
