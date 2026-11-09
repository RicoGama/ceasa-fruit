import type { FormError } from '@nuxt/ui'
import type { ApiErrorModel } from '~/types'

// A API responde mensagens em inglês (código em inglês); aqui traduzimos as
// conhecidas para mostrar ao usuário. Mensagem desconhecida aparece como veio.
const translations: Record<string, string> = {
  'invalid CPF or CNPJ': 'CPF ou CNPJ inválido',
  'a supplier with this document already exists': 'Já existe um fornecedor com este documento',
  'supplier not found': 'Fornecedor não encontrado',
  'email already registered': 'Este e-mail já está cadastrado',
  'invalid email or password': 'E-mail ou senha inválidos',
  'not a member of this organization': 'Você não participa desta organização',
  'organization is suspended': 'Esta organização está suspensa',
  'no organization selected': 'Selecione uma organização',
  'organization not found': 'Organização não encontrada',
  'platform admin only': 'Acesso restrito ao administrador da plataforma',
  'validation failed': 'Verifique os campos destacados',
}

export function translateApiMessage(message?: string | null): string {
  if (!message) return 'Erro inesperado. Tente novamente.'
  return translations[message] ?? message
}

// Mensagem principal de um erro da API (formato RFC 9457 do Huma).
export function apiErrorMessage(error: unknown): string {
  const e = error as Partial<ApiErrorModel> | undefined
  return translateApiMessage(e?.detail)
}

// Converte os erros de validação da API ("body.document") em erros de
// campo do UForm ("document"), para destacar o campo certo na tela.
export function apiFieldErrors(error: unknown): FormError[] {
  const e = error as Partial<ApiErrorModel> | undefined
  return (e?.errors ?? [])
    .filter(err => err.location?.startsWith('body.'))
    .map(err => ({
      name: err.location!.slice('body.'.length),
      message: translateApiMessage(err.message),
    }))
}
