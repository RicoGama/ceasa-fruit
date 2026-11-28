import type { Permission } from '~/types'

// Como o histórico de alterações aparece na tela: nomes dos cadastros, das
// partes de cada cadastro e dos campos em português, e cada valor formatado
// ("Sim", "(21) 99999-0000", "12,5%", "Tomate Italiano"...).
//
// A API manda os nomes das colunas do banco (phone, consignment_commission).
// Coluna nova sem texto aqui aparece com o nome técnico: acrescente abaixo.

export type AuditEntityType = 'party' | 'product' | 'user' | 'role' | 'company' | 'vehicle' | 'driver' | 'certificate'

export const auditEntityLabels: Record<AuditEntityType, string> = {
  party: 'Fornecedor / cliente',
  product: 'Produto',
  user: 'Usuário',
  role: 'Papel',
  company: 'Dados da empresa',
  vehicle: 'Veículo',
  driver: 'Motorista',
  certificate: 'Certificado digital',
}

// Endereço da tela de cada cadastro (para o link no histórico geral).
export function auditEntityLink(type: AuditEntityType, id: number): string | undefined {
  switch (type) {
    case 'product': return `/app/products/${id}`
    case 'role': return `/app/settings/roles/${id}`
    case 'company': return '/app/settings/company'
    case 'user': return '/app/settings/users'
    case 'vehicle': return '/app/fleet/vehicles'
    case 'driver': return '/app/fleet/drivers'
    case 'certificate': return '/app/settings/certificate'
    default: return undefined // pessoa: pode ser fornecedor e/ou cliente
  }
}

// Parte do cadastro que mudou (tabela do banco).
export const auditTableLabels: Record<string, string> = {
  parties: 'Cadastro',
  party_contacts: 'Contato',
  suppliers: 'Dados de fornecedor',
  customers: 'Dados de cliente',
  supplier_products: 'Produto do fornecedor',
  products: 'Produto',
  product_packagings: 'Embalagem',
  users: 'Usuário',
  roles: 'Papel',
  role_permissions: 'Permissão',
  company: 'Empresa',
  drivers: 'Motorista',
  vehicles: 'Veículo',
  digital_certificates: 'Certificado digital',
}

export const auditActionLabels = { created: 'incluído', updated: 'alterado', deleted: 'removido' } as const

const fieldLabels: Record<string, string> = {
  // pessoa
  person_type: 'Tipo de pessoa',
  document: 'CPF/CNPJ',
  legal_name: 'Razão social / nome',
  trade_name: 'Nome fantasia / apelido',
  state_registration: 'Inscrição estadual',
  taxpayer_type: 'Contribuinte do ICMS',
  email: 'E-mail',
  phone: 'Telefone',
  whatsapp: 'WhatsApp',
  postal_code: 'CEP',
  street: 'Logradouro',
  number: 'Número',
  complement: 'Complemento',
  district: 'Bairro',
  city_id: 'Município',
  pix_key_type: 'Tipo de chave PIX',
  pix_key: 'Chave PIX',
  bank: 'Banco',
  bank_branch: 'Agência',
  bank_account: 'Conta',
  bank_account_type: 'Tipo de conta',
  bank_account_holder: 'Titular da conta',
  bank_account_holder_document: 'CPF/CNPJ do titular',
  notes: 'Observações',
  // contato
  name: 'Nome',
  role: 'Função',
  is_whatsapp: 'É WhatsApp',
  // fornecedor / cliente
  supplier_type: 'Tipo de fornecedor',
  customer_type: 'Tipo de cliente',
  default_purchase_mode: 'Modalidade padrão',
  consignment_commission: 'Comissão da consignação',
  payment_term_days: 'Prazo de pagamento (dias)',
  ceasa_pavilion: 'Pavilhão na CEASA',
  ceasa_box: 'Box',
  status: 'Situação',
  blocked_reason: 'Motivo do bloqueio',
  // fornecedor × produto
  product_id: 'Produto',
  supplier_product_name: 'Nome na nota do fornecedor',
  months: 'Safra',
  purchase_mode: 'Modalidade',
  preferred: 'Preferencial',
  // produto
  code: 'Código',
  variety: 'Variedade',
  grade: 'Classificação',
  category: 'Categoria',
  stock_unit: 'Unidade de estoque',
  nominal_weight_kg: 'Peso médio (kg)',
  sells_by_weight: 'Vende por peso',
  ncm: 'NCM',
  origin: 'Origem',
  active: 'Ativo',
  quantity: 'Quantidade',
  // usuários e papéis
  role_id: 'Papel',
  description: 'Descrição',
  permission: 'Permissão',
  key: 'Chave do sistema',
  // empresa
  tax_regime: 'Regime tributário',
  // frota
  license_number: 'Número da CNH',
  license_category: 'Categoria da CNH',
  license_expires_on: 'Validade da CNH',
  plate: 'Placa',
  kind: 'Tipo',
  capacity_kg: 'Capacidade (kg)',
  driver_id: 'Motorista habitual',
  tracker_imei: 'IMEI do rastreador',
  // certificado digital
  holder_name: 'Titular',
  issuer: 'Autoridade certificadora',
  serial_number: 'Número de série',
  fingerprint: 'Impressão digital (SHA-256)',
  not_before: 'Válido a partir de',
  not_after: 'Vencimento',
  uploaded_by: 'Enviado por',
  replaced_at: 'Substituído em',
}

// Campos da própria ligação que não interessam na tela.
const hiddenFields = new Set(['supplier_id', 'party_id', 'role_id@role_permissions', 'product_id@product_packagings'])

export function auditFieldVisible(table: string, field: string) {
  return !hiddenFields.has(field) && !hiddenFields.has(`${field}@${table}`)
}

export function auditFieldLabel(table: string, field: string) {
  if (table === 'vehicles' && field === 'description') return 'Descrição'
  if (table === 'roles' && field === 'name') return 'Nome do papel'
  return fieldLabels[field] ?? field
}

// Campos que apontam para outro cadastro: o nome vem em refs ("product:12").
const refKinds: Record<string, string> = {
  product_id: 'product', role_id: 'role', driver_id: 'driver', city_id: 'city', uploaded_by: 'user',
}

const enumLabels: Record<string, Record<string, string>> = {
  person_type: personTypeLabels,
  taxpayer_type: taxpayerTypeLabels,
  pix_key_type: pixKeyTypeLabels,
  bank_account_type: bankAccountTypeLabels,
  supplier_type: supplierTypeLabels,
  customer_type: customerTypeLabels,
  default_purchase_mode: purchaseModeLabels,
  purchase_mode: purchaseModeLabels,
  status: supplierStatusLabels,
  category: productCategoryLabels,
  stock_unit: stockUnitLabels,
  tax_regime: taxRegimeLabels,
  kind: vehicleKindLabels,
}

// Texto de cada permissão ("Fornecedores: Ver").
function permissionText(code: string) {
  for (const r of permissionResources) {
    const a = r.actions.find(x => x.permission === code)
    if (a) return r.actions.length > 1 ? `${r.label}: ${a.label}` : r.label
  }
  return code
}

// Formata um valor do histórico para a tela. undefined = campo vazio.
export function auditValue(field: string, value: unknown, refs: Record<string, string>): string {
  if (value === undefined || value === null || value === '') return 'vazio'
  if (typeof value === 'boolean') return value ? 'Sim' : 'Não'
  const s = String(value)
  if (refKinds[field]) return refs[`${refKinds[field]}:${s}`] ?? `#${s}`
  if (enumLabels[field]?.[s]) return enumLabels[field][s]!
  switch (field) {
    case 'document':
    case 'bank_account_holder_document':
      return formatDocument(s)
    case 'phone':
    case 'whatsapp':
      return formatPhone(s)
    case 'postal_code':
      return formatPostalCode(s)
    case 'ncm':
      return formatNcm(s)
    case 'plate':
      return formatPlate(s)
    case 'origin':
      return productOriginLabels[Number(s)] ?? s
    case 'consignment_commission':
      return formatPercent(s)
    case 'months':
      return Array.isArray(value) && value.length ? formatMonths(value as number[]) : 'o ano todo'
    case 'permission':
      return permissionText(s)
    case 'license_expires_on':
      return new Date(`${s}T12:00:00`).toLocaleDateString('pt-BR')
    case 'not_before':
    case 'not_after':
    case 'replaced_at':
      return new Date(s).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' })
  }
  if (typeof value === 'number') return value.toLocaleString('pt-BR')
  if (Array.isArray(value)) return value.join(', ')
  return s
}

// Nome curto da linha que entrou ou saiu ("Contato Seu Jorge", "Permissão Fornecedores: Ver").
export function auditRowTitle(table: string, fields: { name: string, old?: unknown, new?: unknown }[], refs: Record<string, string>) {
  const get = (n: string) => {
    const f = fields.find(x => x.name === n)
    return f ? (f.new ?? f.old) : undefined
  }
  const label = auditTableLabels[table] ?? table
  if (table === 'role_permissions') return `${label}: ${permissionText(String(get('permission')))}`
  if (table === 'supplier_products') return `${label}: ${auditValue('product_id', get('product_id'), refs)}`
  const name = get('name') ?? get('plate') ?? get('legal_name') ?? get('holder_name')
  return name ? `${label}: ${String(name)}` : label
}

// Quem pode ver o histórico de cada tipo de cadastro (igual à API).
export function canSeeHistory(can: (p: Permission) => boolean, type: AuditEntityType) {
  if (can('audit.view')) return true
  switch (type) {
    case 'party': return can('suppliers.view') || can('customers.view')
    case 'product': return can('products.view')
    case 'user':
    case 'role': return can('users.manage')
    case 'company': return can('company.edit')
    default: return can('fleet.view')
  }
}
