<script setup lang="ts">
// "Quem fornece este produto": aparece no cadastro do produto para quem pode
// ver fornecedores. Preferidos primeiro. Atualiza em tempo real.
const props = defineProps<{ productId: number }>()
const api = useApi()

const { data, status, refresh } = useAsyncData(`product-suppliers-${props.productId}`, async () => {
  const { data } = await api.GET('/products/{id}/suppliers', { params: { path: { id: props.productId } } })
  return data?.items ?? []
})

let timer: ReturnType<typeof setTimeout> | undefined
useRealtime('product_suppliers', (ev) => {
  if (ev.topic !== 'resync' && ev.id !== props.productId) return
  clearTimeout(timer)
  timer = setTimeout(() => refresh(), 300)
})

const whatsappLink = (n: string) => `https://wa.me/55${onlyDigits(n)}`
</script>

<template>
  <UPageCard title="Quem fornece" description="Fornecedores que trazem este produto. Para ligar um fornecedor, use a aba Produtos no cadastro dele." variant="subtle">
    <p v-if="status !== 'pending' && !data?.length" class="text-sm text-muted">
      Nenhum fornecedor ligado a este produto ainda.
    </p>
    <ul v-else class="divide-y divide-default">
      <li v-for="s in data ?? []" :key="s.supplierId" class="flex items-center gap-3 py-3 first:pt-0 last:pb-0">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-1.5">
            <UIcon v-if="s.preferred" name="i-lucide-star" class="size-4 shrink-0 text-warning" aria-label="Preferencial" />
            <NuxtLink :to="`/app/suppliers/${s.supplierId}?aba=produtos`" class="truncate font-medium text-highlighted hover:text-primary">
              {{ s.tradeName || s.legalName }}
            </NuxtLink>
            <UBadge v-if="s.status !== 'active'" :label="supplierStatusLabels[s.status]" :color="supplierStatusColors[s.status]" variant="subtle" size="sm" />
          </div>
          <p class="text-xs text-muted">
            {{ formatMonths(s.months) }} · {{ purchaseModeLabels[s.purchaseMode] }}<template v-if="s.consignmentCommission">
              {{ formatPercent(s.consignmentCommission) }}
            </template>
            <template v-if="s.supplierProductName">
              · “{{ s.supplierProductName }}”
            </template>
          </p>
        </div>
        <UButton
          v-if="s.whatsapp"
          :to="whatsappLink(s.whatsapp)"
          target="_blank"
          icon="i-lucide-message-circle"
          color="neutral"
          variant="ghost"
          :aria-label="`WhatsApp de ${s.tradeName || s.legalName}`"
        />
      </li>
    </ul>
  </UPageCard>
</template>
