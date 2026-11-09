<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui'

defineProps<{ collapsed?: boolean }>()

const { me, currentOrganization, switchOrganization } = useAuth()
const toast = useToast()

async function select(id: number) {
  try {
    await switchOrganization(id)
  }
  catch (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
  }
}

// Uma opção por organização do usuário; a atual aparece marcada.
const items = computed<DropdownMenuItem[][]>(() => [
  (me.value?.organizations ?? []).map(org => ({
    label: org.name,
    avatar: { alt: org.name },
    type: 'checkbox' as const,
    checked: org.id === me.value?.currentOrganizationId,
    disabled: org.status !== 'active',
    onSelect: () => select(org.id),
  })),
])
</script>

<template>
  <UDropdownMenu
    :items="items"
    :content="{ align: 'center', collisionPadding: 12 }"
    :ui="{ content: collapsed ? 'w-48' : 'w-(--reka-dropdown-menu-trigger-width)' }"
  >
    <UButton
      :label="collapsed ? undefined : (currentOrganization?.name ?? 'Selecionar organização')"
      :avatar="{ alt: currentOrganization?.name ?? '?' }"
      :trailing-icon="collapsed ? undefined : 'i-lucide-chevrons-up-down'"
      color="neutral"
      variant="ghost"
      block
      :square="collapsed"
      class="data-[state=open]:bg-elevated"
      :class="[!collapsed && 'py-2']"
      :ui="{ trailingIcon: 'text-dimmed' }"
    />
  </UDropdownMenu>
</template>
