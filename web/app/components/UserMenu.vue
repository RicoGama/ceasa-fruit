<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui'

defineProps<{ collapsed?: boolean }>()

const auth = useAuthStore()
const colorMode = useColorMode()

const items = computed<DropdownMenuItem[][]>(() => [
  [{ type: 'label', label: auth.user?.name, description: auth.user?.email }],
  [
    {
      label: 'Tema',
      icon: 'i-lucide-sun-moon',
      children: [
        {
          label: 'Claro',
          icon: 'i-lucide-sun',
          type: 'checkbox',
          checked: colorMode.preference === 'light',
          onSelect: () => { colorMode.preference = 'light' },
        },
        {
          label: 'Escuro',
          icon: 'i-lucide-moon',
          type: 'checkbox',
          checked: colorMode.preference === 'dark',
          onSelect: () => { colorMode.preference = 'dark' },
        },
        {
          label: 'Automático',
          icon: 'i-lucide-monitor',
          type: 'checkbox',
          checked: colorMode.preference === 'system',
          onSelect: () => { colorMode.preference = 'system' },
        },
      ],
    },
  ],
  [{ label: 'Sair', icon: 'i-lucide-log-out', onSelect: () => auth.logout() }],
])
</script>

<template>
  <UDropdownMenu
    :items="items"
    :content="{ align: 'center', collisionPadding: 12 }"
    :ui="{ content: collapsed ? 'w-48' : 'w-(--reka-dropdown-menu-trigger-width)' }"
  >
    <UButton
      :label="collapsed ? undefined : auth.user?.name"
      :avatar="{ alt: auth.user?.name }"
      :trailing-icon="collapsed ? undefined : 'i-lucide-chevrons-up-down'"
      color="neutral"
      variant="ghost"
      block
      :square="collapsed"
      class="data-[state=open]:bg-elevated"
      :ui="{ trailingIcon: 'text-dimmed' }"
    />
  </UDropdownMenu>
</template>
