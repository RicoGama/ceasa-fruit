<script setup lang="ts">
import type { CNPJData } from '~/types'

// Consulta o CNPJ na Receita (pela API do box) e oferece preencher o
// cadastro. Nunca preenche sozinho: mostra o que achou e a pessoa decide.
//
// auto: consulta assim que o CNPJ fica completo e válido (cadastro novo).
const props = defineProps<{ cnpj: string, auto?: boolean }>()
const emit = defineEmits<{ fill: [data: CNPJData] }>()

const api = useApi()
const doc = computed(() => normalizeDocument(props.cnpj))
const ready = computed(() => doc.value.length === 14 && isValidDocument(doc.value))

const result = ref<CNPJData | null>(null)
const problem = ref<{ text: string, color: 'neutral' | 'warning' | 'error' } | null>(null)
const loading = ref(false)
let asked = ''

async function consult() {
  if (!ready.value) return
  asked = doc.value
  loading.value = true
  problem.value = null
  result.value = null
  const { data, error, response } = await api.GET('/cnpj/{cnpj}', { params: { path: { cnpj: doc.value } } })
  loading.value = false
  if (asked !== doc.value) return // o CNPJ mudou enquanto consultava
  if (data) {
    result.value = data
    return
  }
  if (response.status === 404) problem.value = { text: 'CNPJ não encontrado na Receita. Confira os números.', color: 'neutral' }
  else if (response.status === 503) problem.value = { text: 'Não deu para consultar a Receita agora (o servidor está sem internet?). Preencha à mão.', color: 'warning' }
  else problem.value = { text: apiErrorMessage(error), color: 'error' }
}

// Trocou o CNPJ: some o resultado antigo; no cadastro novo, consulta sozinho.
watch(doc, (d) => {
  if (d === asked) return
  result.value = null
  problem.value = null
  if (props.auto && ready.value) consult()
}, { immediate: true })

function fill() {
  if (!result.value) return
  emit('fill', result.value)
  result.value = null
}

const details = computed(() => {
  const r = result.value
  if (!r) return ''
  const parts = [`Situação: ${r.status || '?'}`]
  if (r.openedOn) parts.push(`aberta em ${new Date(`${r.openedOn}T12:00:00`).toLocaleDateString('pt-BR')}`)
  if (r.simples !== null && r.simples !== undefined) parts.push(`Simples Nacional: ${r.simples ? 'sim' : 'não'}`)
  if (r.address.city) parts.push(`${r.address.city.name}/${r.address.city.state}`)
  if (r.mainActivity) parts.push(r.mainActivity)
  return parts.join(' · ')
})

defineExpose({ consult })
</script>

<template>
  <div v-if="ready" class="space-y-2">
    <UButton
      v-if="!result && !auto"
      label="Consultar na Receita"
      icon="i-lucide-search-check"
      color="neutral"
      variant="outline"
      size="xs"
      :loading="loading"
      @click="consult"
    />
    <p v-else-if="loading" class="flex items-center gap-2 text-sm text-muted">
      <UIcon name="i-lucide-loader-circle" class="size-4 animate-spin" /> Consultando a Receita…
    </p>
    <UAlert
      v-if="result"
      :color="result.active ? 'success' : 'warning'"
      variant="subtle"
      :icon="result.active ? 'i-lucide-badge-check' : 'i-lucide-triangle-alert'"
      :title="result.active ? `Na Receita: ${result.legalName}` : `Atenção: CNPJ ${result.status} na Receita — ${result.legalName}`"
      :description="details"
      :actions="[
        { label: 'Preencher com estes dados', color: result.active ? 'success' : 'warning', icon: 'i-lucide-download', onClick: fill },
        { label: 'Agora não', color: 'neutral', variant: 'ghost', onClick: () => { result = null } },
      ]"
    />
    <UAlert v-if="problem" :color="problem.color" variant="subtle" icon="i-lucide-info" :description="problem.text" />
  </div>
</template>
