import type { Me } from '~/types'

// Estado de autenticação compartilhado pelo app inteiro.
// useState é o "estado global" do Nuxt: todos os componentes que chamam
// useAuth() enxergam o mesmo `me`.
export function useAuth() {
  const api = useApi()
  const me = useState<Me | null>('me', () => null)
  const loaded = useState('me-loaded', () => false)

  const currentOrganization = computed(() =>
    me.value?.organizations.find(o => o.id === me.value?.currentOrganizationId),
  )

  async function fetchMe() {
    const { data } = await api.GET('/auth/me')
    me.value = data ?? null
    loaded.value = true
    return me.value
  }

  async function login(body: { email: string, password: string }) {
    const { data, error } = await api.POST('/auth/login', { body })
    if (error) throw error
    me.value = data
    loaded.value = true
    return data
  }

  async function signup(body: { name: string, email: string, password: string, organizationName: string }) {
    const { data, error } = await api.POST('/auth/signup', { body })
    if (error) throw error
    me.value = data
    loaded.value = true
    return data
  }

  async function logout() {
    await api.POST('/auth/logout')
    me.value = null
    clearNuxtData() // descarta dados em cache da sessão anterior
    await navigateTo('/login')
  }

  async function switchOrganization(organizationId: number) {
    const { data, error } = await api.PUT('/auth/organization', { body: { organizationId } })
    if (error) throw error
    me.value = data
    clearNuxtData() // os dados em tela eram da organização anterior
    await navigateTo('/app')
  }

  return { me, loaded, currentOrganization, fetchMe, login, signup, logout, switchOrganization }
}
