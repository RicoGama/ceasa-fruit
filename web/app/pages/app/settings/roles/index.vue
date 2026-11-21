<script setup lang="ts">
import type { Role } from '~/types'

definePageMeta({ layout: 'dashboard', permission: 'users.manage' })
useHead({ title: 'Papéis e permissões · CEASA Fruit' })

const api = useApi()

const { data, error, refresh } = await useAsyncData('roles', async () => {
  const { data, error } = await api.GET('/roles')
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data.items
})

// Tempo real: alguém cadastrou ou alterou -> a lista recarrega sozinha.
useLiveRefresh('roles', refresh)

// Total de permissões do catálogo (para "5 de 7").
const catalogSize = permissionResources.reduce((n, r) => n + r.actions.length, 0)

// Nomes das áreas que o papel acessa, para o resumo do cartão.
function areas(role: Role) {
  if (role.isAdmin) return ['Tudo']
  const have = new Set(role.permissions)
  return permissionResources
    .filter(r => r.actions.some(a => have.has(a.permission)))
    .map(r => r.label)
}
</script>

<template>
  <UDashboardPanel id="roles">
    <template #header>
      <AppNavbar>
        <template #right>
          <UButton icon="i-lucide-plus" to="/app/settings/roles/new" aria-label="Novo papel">
            <span class="hidden sm:inline">Novo papel</span>
          </UButton>
        </template>
      </AppNavbar>
    </template>

    <template #body>
      <p class="text-sm text-muted">
        Cada usuário tem um papel, e o papel define o que ele pode fazer. Mudanças valem na hora para todos os usuários do papel.
      </p>

      <UAlert
        v-if="error"
        color="error"
        variant="subtle"
        icon="i-lucide-circle-alert"
        title="Não foi possível carregar os papéis"
        :description="error.message"
        :actions="[{ label: 'Tentar de novo', color: 'error', variant: 'outline', onClick: () => refresh() }]"
      />

      <div v-else class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <NuxtLink
          v-for="role in data ?? []"
          :key="role.id"
          :to="`/app/settings/roles/${role.id}`"
          class="group flex flex-col gap-3 rounded-2xl border border-default bg-default p-5 transition-colors hover:border-primary/50"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <p class="truncate font-semibold text-highlighted group-hover:text-primary">
                {{ role.name }}
              </p>
              <p v-if="role.description" class="mt-0.5 line-clamp-2 text-sm text-muted">
                {{ role.description }}
              </p>
            </div>
            <UIcon :name="role.isAdmin ? 'i-lucide-shield-check' : 'i-lucide-shield'" class="size-5 shrink-0 text-dimmed" />
          </div>

          <div class="flex flex-wrap gap-1.5">
            <UBadge v-for="a in areas(role)" :key="a" :label="a" color="neutral" variant="subtle" size="sm" />
            <UBadge v-if="!areas(role).length" label="Sem acesso a nada" color="warning" variant="subtle" size="sm" />
          </div>

          <div class="mt-auto flex items-center justify-between text-xs text-muted">
            <span>{{ role.activeUsers }} {{ role.activeUsers === 1 ? 'usuário ativo' : 'usuários ativos' }}</span>
            <span>{{ role.isAdmin ? 'Papel do sistema' : `${role.permissions.length} de ${catalogSize} permissões` }}</span>
          </div>
        </NuxtLink>
      </div>
    </template>
  </UDashboardPanel>
</template>
