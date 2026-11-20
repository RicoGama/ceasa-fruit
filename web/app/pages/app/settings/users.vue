<script setup lang="ts">
import { z } from 'zod'
import type { DropdownMenuItem, FormSubmitEvent, TableColumn } from '@nuxt/ui'
import type { Role, User } from '~/types'

definePageMeta({ layout: 'dashboard' })
useHead({ title: 'Usuários · CEASA Fruit' })

const api = useApi()
const toast = useToast()
const auth = useAuthStore()

// --- Dados ----------------------------------------------------------------------
const { data, status, error, refresh } = await useAsyncData('users', async () => {
  const { data, error } = await api.GET('/users')
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data.items
})

const showInactive = ref(false)
const users = computed(() => (data.value ?? []).filter(u => showInactive.value || u.active))

const columns: TableColumn<User>[] = [
  { accessorKey: 'name', header: 'Usuário' },
  { accessorKey: 'role', header: 'Papel' },
  { accessorKey: 'createdAt', header: 'Desde', meta: { class: { th: 'hidden md:table-cell', td: 'hidden md:table-cell' } } },
  { accessorKey: 'active', header: 'Situação' },
  { id: 'actions' },
]

function rowActions(u: User): DropdownMenuItem[] {
  return [
    { label: 'Editar', icon: 'i-lucide-pencil', onSelect: () => openEdit(u) },
    ...(u.active
      ? [{ label: 'Reenviar convite', icon: 'i-lucide-mail', onSelect: () => resendInvite(u) }]
      : []),
  ]
}

// --- Modal de criar/editar ----------------------------------------------------
const roleItems = (Object.keys(roleLabels) as Role[]).map(value => ({
  value,
  label: roleLabels[value],
  description: roleDescriptions[value],
}))

const schema = z.object({
  name: z.string().trim().min(2, 'Informe o nome'),
  email: z.email('E-mail inválido'),
  role: z.enum(Object.keys(roleLabels) as [Role, ...Role[]]),
  active: z.boolean(),
})
type Schema = z.output<typeof schema>

const modalOpen = ref(false)
const editing = ref<User | null>(null) // null = criando
const form = reactive<Schema>({ name: '', email: '', role: 'seller', active: true })
const formRef = useTemplateRef('formRef')
const saving = ref(false)

function openCreate() {
  editing.value = null
  Object.assign(form, { name: '', email: '', role: 'seller', active: true })
  modalOpen.value = true
}

function openEdit(u: User) {
  editing.value = u
  Object.assign(form, { name: u.name, email: u.email, role: u.role, active: u.active })
  modalOpen.value = true
}

async function onSubmit(event: FormSubmitEvent<Schema>) {
  saving.value = true
  const { name, email, role, active } = event.data
  const { error } = editing.value
    ? await api.PUT('/users/{id}', { params: { path: { id: editing.value.id } }, body: { name, role, active, version: editing.value.version } })
    : await api.POST('/users', { body: { name, email, role } })
  saving.value = false

  if (error) {
    if (isStaleError(error)) {
      // Outra pessoa alterou este usuário: fecha o modal e mostra a lista atual.
      modalOpen.value = false
      toast.add({ title: apiErrorMessage(error), description: 'A lista foi atualizada; abra o usuário de novo.', color: 'warning' })
      await refresh()
      return
    }
    const fieldErrors = apiFieldErrors(error)
    if (fieldErrors.length) formRef.value?.setErrors(fieldErrors)
    else toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add(editing.value
    ? { title: 'Usuário atualizado', color: 'success' }
    : { title: 'Usuário criado', description: `Enviamos um convite para ${email} criar a senha.`, color: 'success' })
  modalOpen.value = false
  await refresh()
}

async function resendInvite(u: User) {
  const { error } = await api.POST('/users/{id}/invite', { params: { path: { id: u.id } } })
  if (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: 'Convite reenviado', description: `Link enviado para ${u.email} (vale 3 dias).`, color: 'success' })
}
</script>

<template>
  <UDashboardPanel id="users">
    <template #header>
      <AppNavbar>
        <template #right>
          <UButton icon="i-lucide-plus" aria-label="Novo usuário" @click="openCreate">
            <span class="hidden sm:inline">Novo usuário</span>
          </UButton>
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <div class="flex items-center justify-between gap-3">
        <p class="text-sm text-muted">
          Quem pode entrar no sistema e o que cada um faz.
        </p>
        <USwitch v-model="showInactive" label="Mostrar desativados" />
      </div>

      <UAlert
        v-if="error"
        color="error"
        variant="subtle"
        icon="i-lucide-circle-alert"
        title="Não foi possível carregar os usuários"
        :description="error.message"
        :actions="[{ label: 'Tentar de novo', color: 'error', variant: 'outline', onClick: () => refresh() }]"
      />

      <UTable v-else :data="users" :columns="columns" :loading="status === 'pending'" class="shrink-0">
        <template #name-cell="{ row }">
          <button type="button" class="block text-left group" @click="openEdit(row.original)">
            <p class="font-medium text-highlighted group-hover:text-primary">
              {{ row.original.name }}
              <span v-if="row.original.id === auth.user?.id" class="text-xs font-normal text-muted">(você)</span>
            </p>
            <p class="text-xs text-muted">
              {{ row.original.email }}
            </p>
          </button>
        </template>
        <template #role-cell="{ row }">
          {{ roleLabels[row.original.role] }}
        </template>
        <template #createdAt-cell="{ row }">
          {{ formatDate(row.original.createdAt) }}
        </template>
        <template #active-cell="{ row }">
          <UBadge
            :label="row.original.active ? 'Ativo' : 'Desativado'"
            :color="row.original.active ? 'success' : 'neutral'"
            variant="subtle"
          />
        </template>
        <template #actions-cell="{ row }">
          <div class="text-right">
            <UDropdownMenu :items="rowActions(row.original)" :content="{ align: 'end' }">
              <UButton icon="i-lucide-ellipsis-vertical" color="neutral" variant="ghost" aria-label="Ações" />
            </UDropdownMenu>
          </div>
        </template>
      </UTable>
    </template>
  </UDashboardPanel>

  <UModal
    v-model:open="modalOpen"
    :title="editing ? 'Editar usuário' : 'Novo usuário'"
    :description="editing ? undefined : 'A pessoa recebe um e-mail para criar a própria senha.'"
  >
    <template #body>
      <UForm id="user-form" ref="formRef" :schema="schema" :state="form" class="space-y-4" @submit="onSubmit">
        <UFormField label="Nome" name="name" required>
          <UInput v-model="form.name" class="w-full" autofocus />
        </UFormField>
        <UFormField label="E-mail" name="email" required :help="editing ? 'O e-mail não pode ser alterado.' : undefined">
          <UInput v-model="form.email" type="email" class="w-full" :disabled="!!editing" />
        </UFormField>
        <UFormField label="Papel" name="role" required>
          <USelect v-model="form.role" :items="roleItems" class="w-full" />
          <template #help>
            {{ roleDescriptions[form.role] }}
          </template>
        </UFormField>
        <UFormField v-if="editing" name="active">
          <USwitch
            v-model="form.active"
            label="Usuário ativo"
            description="Desativado não consegue entrar; as sessões abertas são encerradas."
          />
        </UFormField>
      </UForm>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancelar" color="neutral" variant="ghost" @click="modalOpen = false" />
        <UButton type="submit" form="user-form" :label="editing ? 'Salvar' : 'Criar e enviar convite'" :loading="saving" />
      </div>
    </template>
  </UModal>
</template>
