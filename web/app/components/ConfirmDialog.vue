<script setup lang="ts">
// Janela de confirmação do sistema, no lugar do window.confirm do navegador
// (que não segue o visual do sistema e mostra o endereço do site no título).
// Não use direto: chame useConfirm(), que abre esta janela e devolve a resposta.
const props = withDefaults(defineProps<{
  title: string
  description?: string
  confirmLabel?: string
  cancelLabel?: string
  color?: 'primary' | 'error' | 'warning'
  icon?: string
}>(), {
  confirmLabel: 'Confirmar',
  cancelLabel: 'Cancelar',
  color: 'primary',
})

const emit = defineEmits<{ close: [confirmed: boolean] }>()

const iconBox = computed(() => ({
  primary: 'bg-primary/10 text-primary',
  warning: 'bg-warning/10 text-warning',
  error: 'bg-error/10 text-error',
})[props.color])
</script>

<template>
  <UModal
    :title="title"
    :description="description"
    :close="false"
    :ui="{ footer: 'justify-end gap-2', content: 'sm:max-w-md' }"
  >
    <!-- O título fica dentro do #title (e não num #header próprio) para o leitor de tela
         continuar anunciando título e descrição da janela. -->
    <template #title>
      <span class="flex items-center gap-3">
        <span v-if="icon" class="flex size-9 shrink-0 items-center justify-center rounded-full" :class="iconBox">
          <UIcon :name="icon" class="size-5" />
        </span>
        {{ title }}
      </span>
    </template>

    <template #footer>
      <UButton :label="cancelLabel" color="neutral" variant="ghost" @click="emit('close', false)" />
      <UButton :label="confirmLabel" :color="color" autofocus @click="emit('close', true)" />
    </template>
  </UModal>
</template>
