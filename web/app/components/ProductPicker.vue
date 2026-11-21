<script setup lang="ts">
// Campo de produto com busca na API (nome, variedade, classificação ou código).
// v-model é { id, displayName } ou null.
export interface PickedProduct { id: number, displayName: string }

const model = defineModel<PickedProduct | null>({ default: null })
const props = withDefaults(defineProps<{ placeholder?: string, exclude?: number[] }>(), {
  placeholder: 'Digite para buscar o produto',
  exclude: () => [],
})

const api = useApi()
const searchTerm = ref('')
const results = ref<PickedProduct[]>([])
const loading = ref(false)

const items = computed(() => {
  const list = results.value.filter(p => !props.exclude.includes(p.id) || p.id === model.value?.id)
  if (model.value && !list.some(p => p.id === model.value!.id)) list.unshift(model.value)
  return list
})

let timer: ReturnType<typeof setTimeout> | undefined
async function search(term: string) {
  loading.value = true
  const { data } = await api.GET('/products', { params: { query: { search: term || undefined, limit: 20 } } })
  results.value = (data?.items ?? []).map(p => ({ id: p.id, displayName: p.displayName }))
  loading.value = false
}
watch(searchTerm, (term) => {
  clearTimeout(timer)
  timer = setTimeout(() => search(term.trim()), 250)
})
onMounted(() => search('')) // já mostra os primeiros ao abrir

const selectedId = computed({
  get: () => model.value?.id,
  set: (id) => { model.value = items.value.find(p => p.id === id) ?? null },
})
</script>

<template>
  <USelectMenu
    v-model="selectedId"
    v-model:search-term="searchTerm"
    :items="items"
    value-key="id"
    label-key="displayName"
    ignore-filter
    :loading="loading"
    :placeholder="placeholder"
    :search-input="{ placeholder: 'Ex.: tomate italiano' }"
    class="w-full"
  >
    <template #empty>
      Nenhum produto encontrado
    </template>
  </USelectMenu>
</template>
