import type { FormError } from '@nuxt/ui'
import type { ApiErrorModel } from '~/types'

// A API responde mensagens em inglês (código em inglês); aqui traduzimos as
// conhecidas para mostrar ao usuário. Mensagem desconhecida aparece como veio.
// Concorrência: a API recusa salvar por cima do que outra pessoa gravou
// (409, versão diferente) ou quando o registro está travado há tempo demais (503).
const STALE = 'this record was changed by someone else'
const BUSY = 'the record is busy, try again'

export function isStaleError(error: unknown): boolean {
  return (error as Partial<ApiErrorModel> | undefined)?.detail === STALE
}

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
  'customer not found': 'Cliente não encontrado',
  'person not found': 'Cadastro não encontrado',
  'no registration with this document': 'Nenhum cadastro com este documento',
  'this person is already a customer': 'Este cadastro já é cliente',
  'this person is already a supplier': 'Este cadastro já é fornecedor',
  'version is required when reusing a person': 'Falta a versão do cadastro aproveitado',
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
  [STALE]: 'Outra pessoa salvou este registro enquanto você editava',
  [BUSY]: 'Outra pessoa está gravando este registro agora. Tente de novo em instantes.',
  'authentication required': 'Sua sessão expirou. Entre de novo.',
  'invalid CPF': 'CPF inválido',
  'product not found': 'Produto não encontrado',
  'role not found': 'Papel não encontrado',
  'the administrator role cannot be changed': 'O papel Administrador é do sistema e não pode ser alterado',
  'role has users; move them to another role first': 'Este papel tem usuários. Mude-os para outro papel antes de excluir.',
  'a role with this name already exists': 'Já existe um papel com este nome',
  'a product with this code already exists': 'Já existe um produto com este código',
  'a product with this name, variety and grade already exists': 'Já existe um produto com este nome, variedade e classificação',
  'NCM must have 8 digits': 'O NCM tem 8 dígitos',
  'weight must be greater than zero': 'O peso precisa ser maior que zero',
  'average weight is required to sell by weight': 'Informe o peso médio para vender a quilo',
  'packaging name is required': 'Informe o nome da embalagem',
  'duplicate packaging name': 'Já existe uma embalagem com este nome',
  'quantity must be greater than zero': 'A quantidade precisa ser maior que zero',
  'packaging does not belong to this product': 'Embalagem inválida para este produto',
  'invalid document': 'Documento inválido',
  'invalid length': 'Tamanho inválido',
  'commission over 100': 'A comissão não pode passar de 100%',
  'commission must be between 0 and 100': 'A comissão deve ficar entre 0% e 100%',
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
  [/^expected string to match pattern/, () => 'Número inválido'],
  [/decimal|decimal places|out of range/, () => 'Número inválido'],
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
  // Login bloqueado por excesso de tentativas: a API diz em quantos minutos.
  if (e?.detail === 'too many login attempts') {
    const minutes = Number(e.errors?.[0]?.value)
    const wait = minutes > 1 ? `${minutes} minutos` : '1 minuto'
    return `Muitas tentativas sem sucesso. Por segurança, o acesso foi bloqueado por alguns minutos: tente de novo em ${wait}.`
  }
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
