<script setup lang="ts">
// Layout das telas de acesso (login e senha), conforme o layout do designer.
// Esquerda: foto com o slogan (some em telas menores). Direita: logo,
// formulário e rodapé, no fundo creme. Cores e fonte: .auth-skin (main.css).
const brand = useRuntimeConfig().public.brand
const year = new Date().getFullYear()
</script>

<template>
  <div class="auth-skin grid min-h-dvh lg:grid-cols-[52fr_48fr]">
    <!-- Foto + slogan -->
    <aside class="relative hidden overflow-hidden bg-[#183426] lg:block">
      <img
        src="/brand/login-hero.webp"
        alt=""
        class="absolute inset-0 size-full object-cover object-[center_bottom]"
      >
      <div class="relative px-[9.6%] pt-[11vh]">
        <h1 class="text-[clamp(2.75rem,4.55vw,5.25rem)] font-bold leading-[0.97] tracking-[-0.025em] text-[#f6f1e9]">
          Cuidado em<br>cada etapa.
        </h1>
        <span class="mt-[clamp(1rem,1.6vw,1.75rem)] block h-1 w-20 rounded-full bg-(--auth-orange)" />
        <p class="mt-[clamp(0.5rem,0.6vw,0.75rem)] text-[clamp(1.25rem,1.8vw,2rem)] font-medium text-[#f6f1e9]">
          Da compra à entrega.
        </p>
      </div>
    </aside>

    <!-- Logo, formulário e rodapé -->
    <main class="flex min-w-0 flex-col px-5 sm:px-10">
      <div class="flex flex-1 flex-col items-center justify-center py-10 sm:pt-[3.4rem]">
        <div class="w-full max-w-[32.4rem]">
          <img :src="brand.logo" :alt="brand.name" class="mx-auto h-24 w-auto sm:h-[8.5rem] sm:translate-x-3.5">
          <div class="mt-5">
            <slot />
          </div>
        </div>
      </div>
      <p class="pb-6 text-center text-sm text-muted sm:pb-10 sm:text-base">
        © {{ year }} {{ brand.name }}
      </p>
    </main>
  </div>
</template>
