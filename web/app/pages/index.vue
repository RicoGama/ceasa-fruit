<script setup lang="ts">
// Página pública (gerada como HTML estático no build, boa para SEO).
useSeoMeta({
  title: 'CEASA Fruit · Gestão de vendas para hortifrúti',
  description: 'Fornecedores, estoque com alerta de perdas, vendas e rotas de entrega para distribuidoras de legumes, frutas e verduras.',
})

const features = [
  { title: 'Fornecedores', description: 'Produtores e atacadistas com CPF ou CNPJ, busca rápida e histórico preservado.', icon: 'i-lucide-truck' },
  { title: 'Estoque com alerta de perdas', description: 'Controle por lote e validade: saiba o que vence antes de virar prejuízo.', icon: 'i-lucide-package-check' },
  { title: 'Vendas', description: 'Pedidos por quilo, caixa, maço ou unidade, do jeito que o hortifrúti funciona.', icon: 'i-lucide-shopping-cart' },
  { title: 'Rotas de entrega', description: 'Organize as entregas por rota e, em breve, acompanhe os caminhões no mapa.', icon: 'i-lucide-map' },
]
</script>

<template>
  <div>
    <UPageHero
      title="Gestão de vendas para quem vive de hortifrúti"
      description="Do fornecedor à entrega: controle de estoque com alerta de perdas, vendas e rotas para distribuidoras e atacadistas da CEASA."
      :links="[
        { label: 'Criar conta grátis', to: '/signup', trailingIcon: 'i-lucide-arrow-right', size: 'xl' },
        { label: 'Já tenho conta', to: '/login', color: 'neutral', variant: 'subtle', size: 'xl' },
      ]"
    />

    <UPageSection title="Tudo em um só lugar" :features="features" />
  </div>
</template>
