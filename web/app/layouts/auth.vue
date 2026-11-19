<script setup lang="ts">
// Layout das telas de acesso (login, cadastro, senha, escolha de organização).
// Esquerda: painel da marca (some no celular). Direita: logo + formulário.
const brand = useRuntimeConfig().public.brand

const highlights = [
  { icon: 'i-lucide-package-check', title: 'Estoque por lote', text: 'Cor do dia e alerta antes de virar perda.' },
  { icon: 'i-lucide-handshake', title: 'Fornecedores e consignação', text: 'Compra direta ou consignada, com acerto organizado.' },
  { icon: 'i-lucide-truck', title: 'Vendas e entregas', text: 'Do pedido ao boleto, com rotas de entrega.' },
]
</script>

<template>
  <div class="grid min-h-dvh bg-canvas lg:grid-cols-[1.05fr_1fr]">
    <!-- Painel da marca -->
    <aside class="relative hidden overflow-hidden bg-brand-900 text-white lg:flex lg:flex-col dark:bg-brand-950">
      <div class="relative z-10 flex flex-1 flex-col p-12 xl:p-16">
        <div class="flex items-center gap-3">
          <span class="flex size-12 items-center justify-center rounded-2xl bg-white shadow-sm">
            <img src="/brand/icon-512.png" alt="" class="size-9 object-contain">
          </span>
          <div>
            <p class="font-bold leading-tight">
              {{ brand.name }}
            </p>
            <p class="text-sm text-white/70">
              CEASA-RJ
            </p>
          </div>
        </div>

        <div class="my-auto max-w-lg py-12">
          <p class="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-white/70">
            <UIcon name="i-lucide-leaf" class="size-4" />
            Gestão de hortifrúti
          </p>
          <h1 class="mt-4 text-5xl font-extrabold leading-[1.05] tracking-tight">
            Da cotação à expedição, num lugar só.
          </h1>
          <p class="mt-5 text-lg text-white/75">
            Fornecedores, estoque, vendas e entregas do box, em tempo real.
          </p>

          <ul class="mt-10 space-y-5">
            <li v-for="h in highlights" :key="h.title" class="flex gap-4">
              <span class="flex size-11 shrink-0 items-center justify-center rounded-xl bg-white/10 ring ring-white/15">
                <UIcon :name="h.icon" class="size-5" />
              </span>
              <span>
                <span class="block font-bold">{{ h.title }}</span>
                <span class="block text-sm text-white/70">{{ h.text }}</span>
              </span>
            </li>
          </ul>
        </div>

        <p class="text-xs text-white/50">
          © {{ new Date().getFullYear() }} {{ brand.name }}
        </p>
      </div>

      <!-- Ilustração numa faixa própria no rodapé, com o topo esmaecendo no fundo -->
      <div class="relative h-64 shrink-0">
        <HeroIllustration />
        <div class="pointer-events-none absolute inset-x-0 top-0 h-10 bg-gradient-to-b from-brand-900 to-transparent dark:from-brand-950" />
      </div>
    </aside>

    <!-- Formulário -->
    <main class="flex flex-col items-center justify-center px-5 py-10 sm:px-10">
      <div class="w-full max-w-md">
        <div class="mx-auto mb-8 w-fit rounded-2xl dark:bg-white dark:px-5 dark:py-3">
          <img :src="brand.logo" :alt="brand.name" class="h-24 w-auto object-contain sm:h-28">
        </div>
        <div class="rounded-3xl bg-default p-6 shadow-sm ring ring-default sm:p-8">
          <slot />
        </div>
        <p class="mt-6 text-center text-xs text-muted">
          Ambiente seguro · seus dados ficam no servidor da empresa
        </p>
      </div>
    </main>
  </div>
</template>
