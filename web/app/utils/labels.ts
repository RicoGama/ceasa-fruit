// Textos em português para os valores que a API usa em inglês (enums).
// Um lugar só para todas as telas usarem os mesmos nomes.

// Papéis de usuário. O admin pode tudo; os outros vão ganhando permissões
// conforme os módulos forem construídos.
export const roleLabels = {
  admin: 'Administrador',
  manager: 'Gerente',
  seller: 'Vendedor',
  checker: 'Conferente',
  finance: 'Financeiro',
  driver: 'Motorista',
} as const

export const roleDescriptions = {
  admin: 'Acesso total, inclusive usuários e dados da empresa',
  manager: 'Acompanha a operação, preços e relatórios',
  seller: 'Atende clientes e lança pedidos',
  checker: 'Confere entrada e saída de mercadoria',
  finance: 'Boletos, cheques, contas a pagar e receber',
  driver: 'Vê as entregas da sua rota',
} as const

export const taxRegimeLabels = {
  simples: 'Simples Nacional',
  presumido: 'Lucro Presumido',
  real: 'Lucro Real',
} as const

export const personTypeLabels = {
  company: 'Pessoa jurídica',
  individual: 'Pessoa física',
} as const

export const supplierTypeLabels = {
  producer: 'Produtor rural',
  wholesaler: 'Atacadista / comerciante',
  cooperative: 'Cooperativa',
  importer: 'Importador',
  other: 'Outro',
} as const

export const purchaseModeLabels = {
  direct: 'Compra direta',
  consignment: 'Consignação',
} as const

export const supplierStatusLabels = {
  active: 'Ativo',
  inactive: 'Inativo',
  blocked: 'Bloqueado',
} as const

export const supplierStatusColors = {
  active: 'success',
  inactive: 'neutral',
  blocked: 'error',
} as const

export const taxpayerTypeLabels = {
  non_taxpayer: 'Não contribuinte',
  exempt: 'Contribuinte isento',
  taxpayer: 'Contribuinte do ICMS',
} as const

export const pixKeyTypeLabels = {
  cpf: 'CPF',
  cnpj: 'CNPJ',
  email: 'E-mail',
  phone: 'Telefone',
  random: 'Chave aleatória',
} as const

export const bankAccountTypeLabels = {
  checking: 'Conta corrente',
  savings: 'Poupança',
  payment: 'Conta de pagamento',
} as const

// Transforma { valor: 'Rótulo' } na lista que os USelect esperam.
export function toOptions<T extends Record<string, string>>(labels: T) {
  return Object.entries(labels).map(([value, label]) => ({ value: value as keyof T & string, label }))
}
