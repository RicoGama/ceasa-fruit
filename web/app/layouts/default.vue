<!-- Layout do site público (página inicial). -->
<template>
  <div>
    <UHeader title="CEASA Fruit" to="/">
      <template #right>
        <UColorModeButton />
        <UButton label="Entrar" color="neutral" variant="ghost" to="/login" />
        <UButton label="Criar conta" to="/signup" class="hidden sm:inline-flex" />
      </template>
    </UHeader>

    <UMain>
      <slot />
    </UMain>

    <UFooter>
      <template #left>
        <p class="text-sm text-muted">
          © {{ new Date().getFullYear() }} CEASA Fruit
        </p>
      </template>
    </UFooter>
  </div>
</template>
