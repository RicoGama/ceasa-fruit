<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui'

defineProps<{ collapsed?: boolean }>()

const auth = useAuthStore()
const colorMode = useColorMode()
const theme = useBrandTheme()

const initials = computed(() =>
  (auth.user?.name ?? '?').split(' ').filter(Boolean).slice(0, 2).map(p => p[0]!.toUpperCase()).join(''),
)

const items = computed<DropdownMenuItem[][]>(() => [
  [{ type: 'label', label: auth.user?.name, description: auth.user?.email }],
  [
    {
      label: 'Cor do sistema',
      icon: 'i-lucide-palette',
      children: brandThemes.map(t => ({
        label: t.label,
        type: 'checkbox' as const,
        checked: theme.value === t.value,
        onSelect: () => { theme.value = t.value },
      })),
    },
    {
      label: 'Aparência',
      icon: 'i-lucide-sun-moon',
      children: [
        { label: 'Claro', icon: 'i-lucide-sun', type: 'checkbox', checked: colorMode.preference === 'light', onSelect: () => { colorMode.preference = 'light' } },
        { label: 'Escuro', icon: 'i-lucide-moon', type: 'checkbox', checked: colorMode.preference === 'dark', onSelect: () => { colorMode.preference = 'dark' } },
        { label: 'Automático', icon: 'i-lucide-monitor', type: 'checkbox', checked: colorMode.preference === 'system', onSelect: () => { colorMode.preference = 'system' } },
      ],
    },
  ],
  [{ label: 'Sair', icon: 'i-lucide-log-out', onSelect: () => auth.logout() }],
])
</script>

<template>
  <UDropdownMenu
    :items="items"
    :content="{ align: 'start', side: 'top', collisionPadding: 12 }"
    :ui="{ content: collapsed ? 'w-56' : 'w-(--reka-dropdown-menu-trigger-width)' }"
  >
    <button
      type="button"
      class="w-full flex items-center gap-3 rounded-xl hover:bg-elevated transition-colors text-left"
      :class="collapsed ? 'justify-center p-1' : 'p-2'"
      :aria-label="auth.user?.name ?? 'Usuário'"
    >
      <span class="flex size-9 shrink-0 items-center justify-center rounded-full bg-brand-900 text-xs font-bold text-white dark:bg-brand-700">
        {{ initials }}
      </span>
      <span v-if="!collapsed" class="min-w-0 flex-1">
        <span class="block text-sm font-bold text-highlighted truncate">{{ auth.user?.name }}</span>
        <span class="block text-xs text-muted truncate">{{ auth.user ? roleLabels[auth.user.role] : '' }}</span>
      </span>
      <UIcon v-if="!collapsed" name="i-lucide-chevrons-up-down" class="size-4 text-dimmed shrink-0" />
    </button>
  </UDropdownMenu>
</template>
