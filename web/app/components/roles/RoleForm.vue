<script setup lang="ts">
import { z } from 'zod'
import type { Permission, Role } from '~/types'

// Cadastro/edição de papel: nome, descrição e a matriz de permissões.
// role = null -> novo. O papel Administrador aparece só para consulta.
const props = defineProps<{ role: Role | null }>()

const api = useApi()
const toast = useToast()
const auth = useAuthStore()
const form = useTemplateRef('form')
const saving = ref(false)

const isSystem = computed(() => props.role?.isAdmin ?? false)

const state = reactive({
  name: props.role?.name ?? '',
  description: props.role?.description ?? '',
  permissions: [...(props.role?.permissions ?? [])] as Permission[],
})
const version = ref(props.role?.version ?? 0)
const savedSnapshot = ref(JSON.stringify(state))
const dirty = computed(() => JSON.stringify(state) !== savedSnapshot.value)

const schema = z.object({
  name: z.string().trim().min(2, 'Informe o nome do papel').max(60),
  description: z.string().max(200),
})

// --- Matriz de permissões ---------------------------------------------------------
const has = (p: Permission) => state.permissions.includes(p)
function toggle(p: Permission, on: boolean) {
  state.permissions = togglePermission(state.permissions, p, on)
}
// Marca ou desmarca todas as ações de um grupo de uma vez.
function groupState(g: PermissionGroup): boolean | 'indeterminate' {
  const all = g.resources.flatMap(r => r.actions.map(a => a.permission))
  const n = all.filter(has).length
  return n === 0 ? false : n === all.length ? true : 'indeterminate'
}
function toggleGroup(g: PermissionGroup, on: boolean) {
  for (const r of g.resources) for (const a of r.actions) toggle(a.permission, on)
}

// --- Salvar ----------------------------------------------------------------------
async function onSubmit() {
  // Tirar de si mesmo o acesso a esta tela: confirma antes.
  const ownRole = props.role && auth.user?.role.id === props.role.id
  if (ownRole && !has('users.manage')
    && !window.confirm('Você está tirando do seu próprio papel o acesso a usuários e papéis. Depois de salvar, não vai conseguir voltar a esta tela. Continuar?')) {
    return
  }

  saving.value = true
  const body = {
    name: state.name.trim(),
    description: state.description.trim() || undefined,
    permissions: state.permissions,
  }
  const result = props.role
    ? await api.PUT('/roles/{id}', { params: { path: { id: props.role.id } }, body: { ...body, version: version.value } })
    : await api.POST('/roles', { body })
  saving.value = false

  if (result.error) {
    if (isStaleError(result.error)) {
      toast.add({
        title: apiErrorMessage(result.error),
        description: 'Recarregue para ver a versão atual. O que você mudou nesta tela será descartado.',
        color: 'warning',
        duration: 0,
        actions: [{ label: 'Recarregar', color: 'warning', variant: 'outline', onClick: reload }],
      })
      return
    }
    form.value?.setErrors(apiFieldErrors(result.error))
    toast.add({ title: apiErrorMessage(result.error), color: 'error' })
    return
  }

  const saved = result.data
  version.value = saved.version
  Object.assign(state, { name: saved.name, description: saved.description ?? '', permissions: [...saved.permissions] })
  savedSnapshot.value = JSON.stringify(state)
  toast.add({ title: props.role ? 'Papel atualizado' : 'Papel criado', description: 'Vale na hora para os usuários deste papel.', color: 'success' })
  if (ownRole) await auth.fetchMe() // o menu muda conforme as novas permissões
  if (!props.role) await navigateTo(`/app/settings/roles/${saved.id}`)
}

async function reload() {
  if (!props.role) return
  savedSnapshot.value = JSON.stringify(state)
  await refreshNuxtData(`role-${props.role.id}`)
}

// --- Excluir ---------------------------------------------------------------------
const confirmDelete = ref(false)
const deleting = ref(false)
async function remove() {
  if (!props.role) return
  deleting.value = true
  const { error } = await api.DELETE('/roles/{id}', { params: { path: { id: props.role.id } } })
  deleting.value = false
  confirmDelete.value = false
  if (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  savedSnapshot.value = JSON.stringify(state)
  toast.add({ title: 'Papel excluído', color: 'success' })
  await navigateTo('/app/settings/roles')
}

onBeforeRouteLeave(() => {
  if (dirty.value && !saving.value && !window.confirm('Descartar as alterações não salvas?')) return false
})
</script>

<template>
  <UDashboardPanel id="role-form">
    <template #header>
      <AppNavbar :title="props.role?.name">
        <template #right>
          <template v-if="!isSystem">
            <UBadge v-if="dirty" label="Não salvo" color="warning" variant="subtle" class="hidden sm:inline-flex" />
            <UButton type="submit" form="role-form" label="Salvar" icon="i-lucide-check" :loading="saving" />
          </template>
          <UBadge v-else label="Papel do sistema" color="neutral" variant="subtle" />
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <UForm
        id="role-form"
        ref="form"
        :schema="schema"
        :state="state"
        :disabled="isSystem"
        class="mx-auto w-full max-w-4xl space-y-6"
        @submit="onSubmit"
      >
        <UAlert
          v-if="isSystem"
          icon="i-lucide-shield-check"
          color="neutral"
          variant="subtle"
          title="O Administrador tem todas as permissões"
          description="É um papel do sistema: não pode ser alterado nem excluído. Para dar acesso parcial, crie outro papel."
        />

        <UPageCard title="Papel" variant="subtle">
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <UFormField label="Nome" name="name" required>
              <UInput v-model="state.name" placeholder="Ex.: Vendedor externo" class="w-full" />
            </UFormField>
            <UFormField label="Descrição" name="description" help="Para quem vai escolher o papel do usuário">
              <UInput v-model="state.description" placeholder="Ex.: Visita clientes fora do box" class="w-full" />
            </UFormField>
          </div>
          <p v-if="props.role" class="text-sm text-muted">
            {{ props.role.activeUsers }} {{ props.role.activeUsers === 1 ? 'usuário ativo tem' : 'usuários ativos têm' }} este papel.
          </p>
        </UPageCard>

        <UPageCard
          v-for="group in permissionGroups"
          :key="group.label"
          variant="subtle"
          :ui="{ container: 'gap-y-0 sm:p-6 p-4' }"
        >
          <div class="flex items-center justify-between gap-3 border-b border-default pb-3">
            <h2 class="font-semibold text-highlighted">
              {{ group.label }}
            </h2>
            <UCheckbox
              v-if="!isSystem"
              :model-value="groupState(group)"
              label="Marcar tudo"
              :ui="{ label: 'text-xs text-muted' }"
              @update:model-value="toggleGroup(group, $event === true)"
            />
          </div>

          <!-- Uma linha por recurso; as ações quebram para baixo no celular -->
          <div
            v-for="resource in group.resources"
            :key="resource.label"
            class="grid grid-cols-1 gap-2 border-b border-default py-3 last:border-0 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:gap-6"
          >
            <div class="min-w-0">
              <p class="text-sm font-medium text-highlighted">
                {{ resource.label }}
              </p>
              <p v-if="'description' in resource" class="text-xs text-muted">
                {{ resource.description }}
              </p>
            </div>
            <div class="flex flex-wrap gap-x-6 gap-y-2">
              <UCheckbox
                v-for="action in resource.actions"
                :key="action.permission"
                :model-value="isSystem || has(action.permission)"
                :label="action.label"
                :disabled="isSystem"
                @update:model-value="toggle(action.permission, $event === true)"
              />
            </div>
          </div>
        </UPageCard>

        <div v-if="props.role && !isSystem" class="flex flex-col gap-3 rounded-2xl border border-error/30 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p class="text-sm font-medium text-highlighted">
              Excluir papel
            </p>
            <p class="text-xs text-muted">
              Só é possível quando nenhum usuário (nem desativado) tem este papel.
            </p>
          </div>
          <UButton label="Excluir" color="error" variant="outline" icon="i-lucide-trash-2" @click="confirmDelete = true" />
        </div>
      </UForm>
    </template>
  </UDashboardPanel>

  <UModal
    v-model:open="confirmDelete"
    title="Excluir este papel?"
    :description="`O papel ${props.role?.name ?? ''} será apagado. Esta ação não pode ser desfeita.`"
    :ui="{ footer: 'justify-end' }"
  >
    <template #footer>
      <UButton label="Cancelar" color="neutral" variant="ghost" @click="confirmDelete = false" />
      <UButton label="Excluir" color="error" :loading="deleting" @click="remove" />
    </template>
  </UModal>
</template>
