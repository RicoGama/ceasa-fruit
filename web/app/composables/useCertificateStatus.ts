import type { CertificateInfo } from '~/types'

// Situação do certificado digital para o menu e os avisos do painel: quem pode
// gerenciar o certificado vê "vence em N dias" no menu e recebe um aviso uma
// vez por sessão quando ele está para vencer ou venceu. Atualiza sozinho
// (tempo real) quando alguém troca o certificado.
export function useCertificateStatus() {
  const auth = useAuthStore()
  const api = useApi()
  const current = useState<CertificateInfo | null>('certificate-status', () => null)
  const warned = useState('certificate-warned', () => false)
  const toast = useToast()
  const route = useRoute()

  async function load() {
    if (!auth.can('certificate.manage')) {
      current.value = null
      return
    }
    const { data } = await api.GET('/company/certificate')
    current.value = (data?.current ?? null) as CertificateInfo | null
  }

  // Aviso uma vez por sessão (não a cada tela).
  watch(current, (c) => {
    // Na própria tela do certificado o aviso já está na página.
    if (!c || c.status === 'valid' || warned.value || route.path === '/app/settings/certificate') return
    warned.value = true
    toast.add({
      title: c.status === 'expired' ? 'O certificado digital venceu' : `O certificado digital vence em ${daysText(c.daysLeft)}`,
      description: c.status === 'expired'
        ? 'Sem ele não dá para emitir notas fiscais. Envie o certificado renovado.'
        : 'Providencie a renovação com a certificadora e envie o novo arquivo.',
      color: c.status === 'expired' ? 'error' : 'warning',
      icon: 'i-lucide-file-key-2',
      duration: 0,
      actions: [{ label: 'Ver certificado', to: '/app/settings/certificate', color: 'neutral', variant: 'outline' }],
    })
  })

  useLiveRefresh('digital_certificates', load)
  watch(() => auth.can('certificate.manage'), load)
  onMounted(load)

  // Selo do item de menu: só quando precisa de atenção.
  const badge = computed(() => {
    const c = current.value
    if (!c || c.status === 'valid') return undefined
    return c.status === 'expired' ? 'vencido' : daysText(c.daysLeft)
  })
  return { current, badge }
}

// "hoje", "1 dia", "12 dias"
export function daysText(days: number) {
  if (days <= 0) return 'hoje'
  return days === 1 ? '1 dia' : `${days} dias`
}
