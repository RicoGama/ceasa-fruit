<script setup lang="ts">
import type { components } from '~/types/api'

type AuditEvent = components['schemas']['AuditEvent']
type AuditChange = components['schemas']['AuditChange']

// Lista de alterações (do histórico de um cadastro ou do histórico geral).
const props = defineProps<{
  events: AuditEvent[]
  refs: Record<string, string>
  // No histórico geral, cada evento mostra de qual cadastro é.
  showEntity?: boolean
}>()

// Tabelas "principais": criar/remover a linha é criar/remover o cadastro.
const mainTables = new Set(['parties', 'products', 'users', 'roles', 'company', 'drivers', 'vehicles'])

function visibleFields(c: AuditChange) {
  return (c.fields ?? []).filter(f => auditFieldVisible(c.table, f.name))
}

function changeTitle(c: AuditChange) {
  if (c.table === 'suppliers' && c.action === 'created') return 'Passou a ser fornecedor'
  if (c.table === 'customers' && c.action === 'created') return 'Passou a ser cliente'
  if (mainTables.has(c.table) && c.action === 'created') return 'Cadastro criado'
  if (mainTables.has(c.table) && c.action === 'deleted') return 'Cadastro apagado'
  if (c.action === 'updated') return `${auditTableLabels[c.table] ?? c.table} alterado`
  return auditRowTitle(c.table, c.fields ?? [], props.refs)
}

const value = (field: string, v: unknown) => auditValue(field, v, props.refs)
</script>

<template>
  <ol class="space-y-4">
    <li v-for="ev in events" :key="ev.id" class="relative border-l-2 border-default pl-4">
      <span class="absolute -left-[7px] top-1.5 size-3 rounded-full border-2 border-default bg-default" />
      <div class="flex flex-wrap items-baseline gap-x-2 text-sm">
        <span class="font-medium text-highlighted">{{ ev.actor?.name ?? 'Sistema' }}</span>
        <span class="text-muted">{{ formatWhen(ev.occurredAt) }}</span>
      </div>
      <p v-if="showEntity" class="text-sm">
        <span class="mr-1 text-muted">{{ auditEntityLabels[ev.entityType] }}:</span>
        <NuxtLink v-if="auditEntityLink(ev.entityType, ev.entityId)" :to="auditEntityLink(ev.entityType, ev.entityId)" class="font-medium text-primary hover:underline">
          {{ ev.entityName }}
        </NuxtLink>
        <span v-else class="font-medium">{{ ev.entityName }}</span>
      </p>

      <ul class="mt-1.5 space-y-2">
        <li v-for="(c, i) in ev.changes" :key="i" class="text-sm">
          <p class="flex items-center gap-1.5">
            <UIcon
              :name="c.action === 'created' ? 'i-lucide-plus' : c.action === 'deleted' ? 'i-lucide-minus' : 'i-lucide-pencil'"
              class="size-3.5 shrink-0"
              :class="c.action === 'created' ? 'text-success' : c.action === 'deleted' ? 'text-error' : 'text-muted'"
            />
            <span :class="c.action === 'deleted' ? 'text-muted' : ''">{{ changeTitle(c) }}</span>
          </p>
          <!-- Alteração: campo, de quanto, para quanto -->
          <dl v-if="c.action === 'updated'" class="ml-5 mt-1 space-y-1">
            <div v-for="f in visibleFields(c)" :key="f.name" class="grid grid-cols-1 gap-x-2 sm:grid-cols-[10rem_1fr]">
              <dt class="text-muted">
                {{ auditFieldLabel(c.table, f.name) }}
              </dt>
              <dd class="min-w-0 break-words">
                <span class="text-muted line-through decoration-error/50">{{ value(f.name, f.old) }}</span>
                <UIcon name="i-lucide-arrow-right" class="mx-1 size-3 align-middle text-dimmed" />
                <span class="font-medium text-highlighted">{{ value(f.name, f.new) }}</span>
              </dd>
            </div>
          </dl>
        </li>
      </ul>
    </li>
  </ol>
</template>
