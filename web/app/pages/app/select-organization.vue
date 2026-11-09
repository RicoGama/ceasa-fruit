<script setup lang="ts">
definePageMeta({ layout: 'auth' })
useHead({ title: 'Escolher organização · CEASA Fruit' })

const { me, currentOrganization, switchOrganization, logout } = useAuth()

// A organização atual pode ter sido suspensa pelo administrador da plataforma.
const suspended = computed(() => currentOrganization.value?.status === 'suspended')
const activeOrganizations = computed(() => me.value?.organizations.filter(o => o.status === 'active') ?? [])
const toast = useToast()
const selecting = ref<number | null>(null)

async function select(id: number) {
  selecting.value = id
  try {
    await switchOrganization(id)
  }
  catch (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
  }
  finally {
    selecting.value = null
  }
}
</script>

<template>
  <div class="space-y-6">
    <div class="text-center space-y-1">
      <h1 class="text-xl font-semibold text-highlighted">
        Escolha a organização
      </h1>
      <p v-if="!suspended" class="text-sm text-muted">
        Você participa de mais de uma empresa. Dá para trocar depois pelo menu lateral.
      </p>
    </div>

    <UAlert
      v-if="suspended"
      color="warning"
      variant="subtle"
      icon="i-lucide-octagon-pause"
      :title="`${currentOrganization?.name} está suspensa`"
      :description="activeOrganizations.length
        ? 'Fale com o suporte para reativar, ou escolha outra organização abaixo.'
        : 'Fale com o suporte para reativar o acesso.'"
    />

    <div v-if="me?.organizations.length" class="space-y-2">
      <UButton
        v-for="org in me.organizations"
        :key="org.id"
        :label="org.name"
        :avatar="{ alt: org.name }"
        :trailing-icon="org.status === 'active' ? 'i-lucide-chevron-right' : undefined"
        :disabled="org.status !== 'active'"
        :loading="selecting === org.id"
        color="neutral"
        variant="outline"
        size="lg"
        block
        class="justify-start"
        @click="select(org.id)"
      >
        <template v-if="org.status !== 'active'" #trailing>
          <UBadge label="Suspensa" color="warning" variant="subtle" class="ms-auto" />
        </template>
      </UButton>
    </div>

    <UEmpty
      v-else
      icon="i-lucide-building-2"
      title="Nenhuma organização"
      description="Você ainda não participa de nenhuma empresa. Peça um convite ao responsável."
    />

    <div class="text-center">
      <UButton label="Sair" color="neutral" variant="link" @click="logout" />
    </div>
  </div>
</template>
