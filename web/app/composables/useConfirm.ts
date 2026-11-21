import ConfirmDialog from '~/components/ConfirmDialog.vue'

export interface ConfirmOptions {
  title: string
  description?: string
  confirmLabel?: string
  cancelLabel?: string
  /** primary (padrão), warning (cuidado) ou error (ação destrutiva) */
  color?: 'primary' | 'error' | 'warning'
  icon?: string
}

/**
 * Pergunta e espera a resposta, como o window.confirm, mas com a janela do sistema:
 *
 *   const confirm = useConfirm()
 *   if (!await confirm({ title: 'Excluir?', color: 'error' })) return
 *
 * Fechar pelo Esc ou clicando fora conta como "não".
 * Chame useConfirm() no setup do componente (não dentro de um clique).
 */
export function useConfirm() {
  const overlay = useOverlay()
  const dialog = overlay.create(ConfirmDialog)
  return async (options: ConfirmOptions): Promise<boolean> => {
    const answer = await dialog.open(options)
    return answer === true
  }
}

/**
 * Proteção contra perder um formulário não salvo:
 *  - ao sair da tela pelo sistema (menu, breadcrumb, voltar), pergunta com a janela do sistema;
 *  - ao fechar a aba ou recarregar, o navegador mostra o aviso dele. Esse aviso o
 *    navegador não deixa trocar nem mudar o texto (proteção contra sites abusivos).
 */
export function useUnsavedChangesGuard(dirty: () => boolean) {
  const confirm = useConfirm()

  onBeforeRouteLeave(async () => {
    if (!dirty()) return true
    return confirm({
      title: 'Descartar as alterações?',
      description: 'Você mudou dados nesta tela e ainda não salvou. Se sair agora, essas mudanças se perdem.',
      confirmLabel: 'Descartar e sair',
      cancelLabel: 'Continuar editando',
      color: 'warning',
      icon: 'i-lucide-triangle-alert',
    })
  })

  function beforeUnload(e: BeforeUnloadEvent) {
    if (dirty()) e.preventDefault()
  }
  onMounted(() => window.addEventListener('beforeunload', beforeUnload))
  onUnmounted(() => window.removeEventListener('beforeunload', beforeUnload))
}
