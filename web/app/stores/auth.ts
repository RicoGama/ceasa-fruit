import type { Me, Permission } from '~/types'

// Store da sessão: quem está logado, com qual papel e permissões, e qual é a empresa.
// Qualquer componente que use useAuthStore() enxerga o mesmo estado e se
// atualiza sozinho quando ele muda.
//
// É um "setup store": parece um <script setup>. ref() vira estado,
// computed() vira getter e function vira action.
//
// Tempo real (futuro): quando o servidor avisar que o papel do usuário mudou,
// basta chamar fetchMe() (ou alterar `me`) e todas as telas refletem a mudança.
export const useAuthStore = defineStore('auth', () => {
  // --- Estado ------------------------------------------------------------------
  const me = ref<Me | null>(null)
  // true depois da primeira consulta ao /auth/me (logado ou não).
  const loaded = ref(false)

  // --- Getters (valores derivados do estado) ----------------------------------
  const user = computed(() => me.value?.user ?? null)
  const company = computed(() => me.value?.company ?? null)
  const isLoggedIn = computed(() => me.value !== null)
  const isAdmin = computed(() => me.value?.user.role.isAdmin ?? false)
  const permissions = computed(() => new Set(me.value?.user.permissions ?? []))

  // can('products.edit'): mostra ou esconde botões, menus e telas.
  // Quem barra de verdade é a API; aqui é só para não mostrar o que a pessoa
  // não pode usar. (No administrador a API já manda o catálogo inteiro.)
  function can(...perms: Permission[]) {
    return perms.some(p => permissions.value.has(p))
  }

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

  async function logout() {
    await api().POST('/auth/logout')
    reset()
    clearNuxtData() // descarta dados em cache da sessão anterior
    await navigateTo('/login')
  }

  // A sessão acabou (ex.: a API respondeu 401): volta ao estado "deslogado".
  function reset() {
    me.value = null
    loaded.value = true
  }

  return {
    me, loaded,
    user, company, isLoggedIn, isAdmin, can,
    fetchMe, login, logout, reset,
  }
})
