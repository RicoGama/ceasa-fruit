import type { FormError } from '@nuxt/ui'
import type { ApiErrorModel } from '~/types'

// A API responde mensagens em inglês (código em inglês); aqui traduzimos as
// conhecidas para mostrar ao usuário. Mensagem desconhecida aparece como veio.
const translations: Record<string, string> = {
  'invalid CPF or CNPJ': 'CPF ou CNPJ inválido',
  'a supplier with this document already exists': 'Já existe um fornecedor com este documento',
  'a registration with this document already exists': 'Já existe um cadastro com este documento',
  'an individual must have a CPF': 'Pessoa física precisa de CPF',
  'a company must have a CNPJ': 'Pessoa jurídica precisa de CNPJ',
  'invalid postal code': 'CEP inválido',
  'invalid phone': 'Telefone inválido',
  'pix key type is required': 'Informe o tipo da chave PIX',
  'pix key is required': 'Informe a chave PIX',
  'invalid pix key': 'Chave PIX inválida',
  'commission is required for consignment': 'Informe a comissão da consignação',
  'reason is required when blocked': 'Informe o motivo do bloqueio',
  'city not found': 'Município não encontrado',
  'invalid or expired token': 'Este link é inválido ou expirou. Peça um novo.',
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
      // "body.contacts[1].name" -> "contacts.1.name" (o formato de caminho do UForm)
      name: err.location!.slice('body.'.length).replace(/\[(\d+)\]/g, '.$1'),
      message: translateApiMessage(err.message),
    }))
}
