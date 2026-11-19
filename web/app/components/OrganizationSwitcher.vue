<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui'

defineProps<{ collapsed?: boolean }>()

const auth = useAuthStore()
const toast = useToast()

async function select(id: number) {
  try {
    await auth.switchOrganization(id)
  }
  catch (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
  }
}

const roleLabels: Record<string, string> = { owner: 'Painel do gestor', admin: 'Administração', member: 'Equipe' }

// Uma opção por organização do usuário; a atual aparece marcada.
const items = computed<DropdownMenuItem[][]>(() => [
  auth.organizations.map(org => ({
    label: org.name,
    icon: 'i-lucide-store',
    type: 'checkbox' as const,
    checked: org.id === auth.currentOrganization?.id,
    disabled: org.status !== 'active',
    onSelect: () => select(org.id),
  })),
])
</script>

<template>
  <!-- Cartão da organização atual (como no layout); clicando, troca de organização. -->
  <UDropdownMenu
    :items="items"
    :content="{ align: 'start', collisionPadding: 12 }"
    :ui="{ content: collapsed ? 'w-56' : 'w-(--reka-dropdown-menu-trigger-width)' }"
  >
    <button
      type="button"
      class="w-full flex items-center gap-3 rounded-xl border border-default bg-canvas/60 hover:bg-elevated transition-colors text-left"
      :class="collapsed ? 'justify-center p-1.5' : 'p-3'"
      :aria-label="`Organização: ${auth.currentOrganization?.name ?? 'nenhuma'}`"
    >
      <span class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-brand-800 dark:text-brand-200">
        <UIcon name="i-lucide-store" class="size-5" />
      </span>
      <span v-if="!collapsed" class="min-w-0 flex-1">
        <span class="block text-sm font-bold text-highlighted truncate">
          {{ roleLabels[auth.currentOrganization?.role ?? ''] ?? 'Selecionar organização' }}
        </span>
        <span class="block text-xs text-muted truncate">{{ auth.currentOrganization?.name }}</span>
      </span>
      <UIcon v-if="!collapsed && auth.organizations.length > 1" name="i-lucide-chevrons-up-down" class="size-4 text-dimmed shrink-0" />
    </button>
  </UDropdownMenu>
</template>
