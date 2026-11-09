<!-- Layout das telas de login/cadastro: um cartão centralizado. -->
<template>
  <div class="min-h-dvh flex flex-col items-center justify-center gap-6 px-4 py-12 bg-muted/40">
    <NuxtLink to="/" class="flex items-center gap-2 font-semibold text-highlighted">
      <UIcon name="i-lucide-apple" class="size-6 text-primary" />
      CEASA Fruit
    </NuxtLink>
    <UPageCard class="w-full max-w-md">
      <slot />
    </UPageCard>
  </div>
</template>
