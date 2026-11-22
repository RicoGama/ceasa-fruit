<script setup lang="ts">
// Botão "Histórico" dos cadastros. Só aparece para quem pode ver o histórico
// daquele tipo de cadastro (a API confere de novo).
const props = defineProps<{
  entityType: AuditEntityType
  entityId: number
  title?: string
}>()

const auth = useAuthStore()
const visible = computed(() => canSeeHistory(auth.can, props.entityType))
// Aberto/fechado guardado por cadastro (useState): quando outra pessoa salva,
// o formulário se recarrega com os dados novos e este botão é recriado; o
// painel continua aberto, mostrando a alteração que acabou de chegar.
const open = useState(`audit-open-${props.entityType}-${props.entityId}`, () => false)
// Saiu da tela: fecha (para não abrir sozinho na próxima visita).
onBeforeRouteLeave(() => {
  open.value = false
})
</script>

<template>
  <template v-if="visible">
    <UButton icon="i-lucide-history" color="neutral" variant="ghost" aria-label="Histórico de alterações" @click="open = true">
      <span class="hidden sm:inline">Histórico</span>
    </UButton>
    <AuditHistorySlideover v-model:open="open" :entity-type="entityType" :entity-id="entityId" :title="title" />
  </template>
</template>
