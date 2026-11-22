<script setup lang="ts">
// Painel lateral com o histórico de um cadastro (abre a partir de um botão
// ou de uma ação da lista).
defineProps<{
  entityType: AuditEntityType
  entityId: number
  title?: string
}>()
const open = defineModel<boolean>('open', { default: false })
</script>

<template>
  <USlideover v-model:open="open" :title="`Histórico${title ? ` · ${title}` : ''}`" description="Quem mudou o quê, quando e de quanto para quanto.">
    <template #body>
      <AuditHistory v-if="open" :entity-type="entityType" :entity-id="entityId" />
    </template>
  </USlideover>
</template>
