<script setup lang="ts">
// Campo de senha com o "olho" para mostrar o que foi digitado.
defineProps<{ placeholder?: string, autocomplete?: string }>()
const model = defineModel<string>({ default: '' })
const visible = ref(false)
</script>

<template>
  <UInput
    v-model="model"
    :type="visible ? 'text' : 'password'"
    :placeholder="placeholder"
    :autocomplete="autocomplete"
    size="xl"
    :ui="authInputUi"
  >
    <template #trailing>
      <UButton
        color="neutral"
        variant="link"
        size="lg"
        :icon="visible ? 'i-lucide-eye-off' : 'i-lucide-eye'"
        :aria-label="visible ? 'Esconder a senha' : 'Mostrar a senha'"
        :aria-pressed="visible"
        class="text-muted hover:text-highlighted"
        :ui="{ leadingIcon: 'size-6' }"
        @click="visible = !visible"
      />
    </template>
  </UInput>
</template>
