<script setup lang="ts">
// Aviso no topo de um cadastro aberto quando outra pessoa o alterou (ou
// excluiu) enquanto esta tela tinha alterações não salvas.
defineProps<{ kind: 'updated' | 'deleted' | null, backTo?: string }>()
defineEmits<{ reload: [] }>()
</script>

<template>
  <UAlert
    v-if="kind === 'updated'"
    icon="i-lucide-refresh-cw"
    color="warning"
    variant="subtle"
    title="Outra pessoa alterou este cadastro enquanto você editava"
    description="Recarregue para ver a versão atual. O que você mudou nesta tela será descartado."
    :actions="[{ label: 'Recarregar', color: 'warning', variant: 'outline', onClick: () => $emit('reload') }]"
  />
  <UAlert
    v-else-if="kind === 'deleted'"
    icon="i-lucide-trash-2"
    color="error"
    variant="subtle"
    title="Este cadastro foi excluído por outra pessoa"
    :actions="backTo ? [{ label: 'Voltar para a lista', color: 'error', variant: 'outline', to: backTo }] : []"
  />
</template>
