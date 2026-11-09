import type { Me } from '~/types'

// Store da sessão: quem está logado, de quais organizações participa e qual
// está ativa. Qualquer componente que use useAuthStore() enxerga o mesmo
// estado e se atualiza sozinho quando ele muda.
//
// É um "setup store": parece um <script setup>. ref() vira estado,
// computed() vira getter e function vira action.
//
// Tempo real (futuro): quando o servidor avisar que a organização foi
// suspensa, ou que o usuário ganhou acesso a outra, basta chamar fetchMe()
// (ou alterar `me`) e todas as telas refletem a mudança.
export const useAuthStore = defineStore('auth', () => {
  // --- Estado ------------------------------------------------------------------
  const me = ref<Me | null>(null)
  // true depois da primeira consulta ao /auth/me (logado ou não).
  const loaded = ref(false)

  // --- Getters (valores derivados do estado) ----------------------------------
  const user = computed(() => me.value?.user ?? null)
  const isLoggedIn = computed(() => me.value !== null)
  const isPlatformAdmin = computed(() => me.value?.user.isPlatformAdmin ?? false)
  const organizations = computed(() => me.value?.organizations ?? [])
  const currentOrganization = computed(() =>
    organizations.value.find(o => o.id === me.value?.currentOrganizationId) ?? null,
  )
  // Organizações em que dá para entrar (as suspensas ficam de fora).
  const activeOrganizations = computed(() => organizations.value.filter(o => o.status === 'active'))

  // --- Actions -----------------------------------------------------------------
  // O cliente da API é pego na hora de usar (e não na criação do store), para
  // o store não depender da ordem em que os plugins são carregados.
  const api = () => useNuxtApp().$api

  async function fetchMe() {
    const { data } = await api().GET('/auth/me')
    me.value = data ?? null
    loaded.value = true
    return me.value
  }

  async function login(body: { email: string, password: string }) {
    const { data, error } = await api().POST('/auth/login', { body })
    if (error) throw error
    me.value = data
    loaded.value = true
    return data
  }

  async function signup(body: { name: string, email: string, password: string, organizationName: string }) {
    const { data, error } = await api().POST('/auth/signup', { body })
    if (error) throw error
    me.value = data
    loaded.value = true
    return data
  }

  async function logout() {
    await api().POST('/auth/logout')
    reset()
    clearNuxtData() // descarta dados em cache da sessão anterior
    await navigateTo('/login')
  }

  async function switchOrganization(organizationId: number) {
    const { data, error } = await api().PUT('/auth/organization', { body: { organizationId } })
    if (error) throw error
    me.value = data
    clearNuxtData() // os dados em tela eram da organização anterior
    await navigateTo('/app')
  }

  // A sessão acabou (ex.: a API respondeu 401): volta ao estado "deslogado".
  function reset() {
    me.value = null
    loaded.value = true
  }

  // Algo mudou no servidor (ex.: organização suspensa): a próxima navegação
  // consulta o /auth/me de novo.
  function invalidate() {
    loaded.value = false
  }

  return {
    me, loaded,
    user, isLoggedIn, isPlatformAdmin, organizations, currentOrganization, activeOrganizations,
    fetchMe, login, signup, logout, switchOrganization, reset, invalidate,
  }
})
