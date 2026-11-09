<script setup lang="ts">
import type { TableColumn } from '@nuxt/ui'
import type { AdminOrganization } from '~/types'

definePageMeta({ layout: 'dashboard' })
useHead({ title: 'Organizações · CEASA Fruit' })

const api = useApi()
const toast = useToast()

const limit = 20
const page = ref(1)

const { data, status, error, refresh } = await useAsyncData('admin-organizations', async () => {
  const { data, error } = await api.GET('/admin/organizations', {
    params: { query: { limit, offset: (page.value - 1) * limit } },
  })
  if (error) throw createError({ statusCode: error.status, message: apiErrorMessage(error) })
  return data
}, { watch: [page] })

const columns: TableColumn<AdminOrganization>[] = [
  { accessorKey: 'name', header: 'Organização' },
  { accessorKey: 'memberCount', header: 'Usuários' },
  { accessorKey: 'createdAt', header: 'Criada em' },
  { accessorKey: 'status', header: 'Status' },
  { id: 'actions' },
]

const updating = ref<number | null>(null)

async function setStatus(org: AdminOrganization, newStatus: 'active' | 'suspended') {
  updating.value = org.id
  const { error } = await api.PUT('/admin/organizations/{id}/status', {
    params: { path: { id: org.id } },
    body: { status: newStatus },
  })
  updating.value = null
  if (error) {
    toast.add({ title: apiErrorMessage(error), color: 'error' })
    return
  }
  toast.add({ title: newStatus === 'active' ? 'Organização reativada' : 'Organização suspensa', color: 'success' })
  await refresh()
}
</script>

<template>
  <UDashboardPanel id="admin-organizations">
    <template #header>
      <UDashboardNavbar title="Organizações">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <UAlert
        v-if="error"
        color="error"
        variant="subtle"
        icon="i-lucide-circle-alert"
        title="Não foi possível carregar os dados"
        :description="error.message"
        :actions="[{ label: 'Tentar de novo', color: 'error', variant: 'outline', onClick: () => refresh() }]"
      />
      <UTable v-else :data="data?.items ?? []" :columns="columns" :loading="status === 'pending'">
        <template #name-cell="{ row }">
          <span class="font-medium text-highlighted">{{ row.original.name }}</span>
        </template>
        <template #createdAt-cell="{ row }">
          {{ formatDate(row.original.createdAt) }}
        </template>
        <template #status-cell="{ row }">
          <UBadge
            :label="row.original.status === 'active' ? 'Ativa' : 'Suspensa'"
            :color="row.original.status === 'active' ? 'success' : 'warning'"
            variant="subtle"
          />
        </template>
        <template #actions-cell="{ row }">
          <div class="text-right">
            <UButton
              v-if="row.original.status === 'active'"
              label="Suspender"
              color="warning"
              variant="ghost"
              size="sm"
              :loading="updating === row.original.id"
              @click="setStatus(row.original, 'suspended')"
            />
            <UButton
              v-else
              label="Reativar"
              color="success"
              variant="ghost"
              size="sm"
              :loading="updating === row.original.id"
              @click="setStatus(row.original, 'active')"
            />
          </div>
        </template>
      </UTable>

      <div v-if="(data?.total ?? 0) > limit" class="flex justify-end border-t border-default pt-4">
        <UPagination v-model:page="page" :total="data?.total ?? 0" :items-per-page="limit" />
      </div>
    </template>
  </UDashboardPanel>
</template>
