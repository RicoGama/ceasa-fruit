<script setup lang="ts">
import type { City } from '~/types'

// Campo de município com busca na API (lista oficial do IBGE).
// v-model é o objeto { id, name, state } ou null.
const model = defineModel<City | null>({ default: null })

const api = useApi()
const searchTerm = ref('')
const results = ref<City[]>([])
const loading = ref(false)

// Os itens são os resultados da busca + o município já escolhido, para o
// componente conseguir mostrar o nome dele mesmo antes de qualquer busca.
const items = computed(() => {
  const list = [...results.value]
  if (model.value && !list.some(c => c.id === model.value!.id)) list.unshift(model.value)
  return list.map(c => ({ ...c, label: `${c.name} - ${c.state}` }))
})

let timer: ReturnType<typeof setTimeout> | undefined
watch(searchTerm, (term) => {
  clearTimeout(timer)
  if (term.trim().length < 2) {
    results.value = []
    return
  }
  timer = setTimeout(async () => {
    loading.value = true
    const { data } = await api.GET('/cities', { params: { query: { search: term.trim(), limit: 20 } } })
    results.value = data ?? []
    loading.value = false
  }, 250)
})

// O USelectMenu trabalha com o id (valueKey); convertemos de/para o objeto.
const selectedId = computed({
  get: () => model.value?.id,
  set: (id) => { model.value = items.value.find(c => c.id === id) ?? null },
})
</script>

<template>
  <USelectMenu
    v-model="selectedId"
    v-model:search-term="searchTerm"
    :items="items"
    value-key="id"
    label-key="label"
    ignore-filter
    :loading="loading"
    placeholder="Digite para buscar o município"
    :search-input="{ placeholder: 'Ex.: Teresópolis' }"
    class="w-full"
  >
    <template #empty>
      {{ searchTerm.trim().length < 2 ? 'Digite pelo menos 2 letras' : 'Nenhum município encontrado' }}
    </template>
  </USelectMenu>
</template>
