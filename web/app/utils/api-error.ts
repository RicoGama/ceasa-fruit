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
  'not allowed for your role': 'Seu perfil não tem permissão para isso',
  'you cannot deactivate your own user': 'Você não pode desativar o seu próprio usuário',
  'the system must keep at least one active admin': 'O sistema precisa de pelo menos um administrador ativo',
  'user is inactive': 'Este usuário está desativado',
  'user not found': 'Usuário não encontrado',
  'company not configured': 'Os dados da empresa ainda não foram cadastrados',
  'invalid CNPJ': 'CNPJ inválido',
  'validation failed': 'Verifique os campos destacados',
  'authentication required': 'Sua sessão expirou. Entre de novo.',
  'invalid CPF': 'CPF inválido',
  'invalid document': 'Documento inválido',
  'invalid length': 'Tamanho inválido',
  'commission over 100': 'A comissão não pode passar de 100%',
}

// Mensagens de validação automáticas da API (Huma), que trazem números ou
// nomes no meio. Cada regra vira um texto em português.
const patterns: [RegExp, (...m: string[]) => string][] = [
  [/^expected required property \S+ to be present$/, () => 'Campo obrigatório'],
  [/^expected length >= (\d+)$/, n => `Use pelo menos ${n} caracteres`],
  [/^expected length <= (\d+)$/, n => `Use no máximo ${n} caracteres`],
  [/^expected number >= (\S+)$/, n => `O valor mínimo é ${n}`],
  [/^expected number <= (\S+)$/, n => `O valor máximo é ${n}`],
  [/^expected array length >= (\d+)$/, n => `Adicione pelo menos ${n} item(ns)`],
  [/^expected value to be one of/, () => 'Escolha uma das opções'],
  [/email/i, () => 'E-mail inválido'],
  [/^expected (string|number|integer|boolean|object|array)/, () => 'Valor inválido'],
  [/^unexpected property/, () => 'Campo não reconhecido'],
]

// Nada da API chega em inglês na tela: o que não estiver traduzido vira um
// texto genérico (e o original vai para o console, para a gente traduzir).
const fallback = 'Não foi possível concluir a operação. Tente novamente.'

export function translateApiMessage(message?: string | null): string {
  if (!message) return fallback
  const known = translations[message]
  if (known) return known
  for (const [re, text] of patterns) {
    const m = message.match(re)
    if (m) return text(...m.slice(1))
  }
  console.warn('[api] mensagem sem tradução:', message)
  return fallback
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
