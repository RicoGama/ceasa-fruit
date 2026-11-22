// Textos em português para os valores que a API usa em inglês (enums).
// Um lugar só para todas as telas usarem os mesmos nomes.

export const taxRegimeLabels = {
  simples: 'Simples Nacional',
  presumido: 'Lucro Presumido',
  real: 'Lucro Real',
} as const

// --- Produtos -------------------------------------------------------------------

export const productCategoryLabels = {
  fruit: 'Fruta',
  vegetable: 'Legume',
  leafy: 'Verdura',
  egg: 'Ovos',
  other: 'Outros',
} as const

export const productCategoryColors = {
  fruit: 'warning',
  vegetable: 'success',
  leafy: 'primary',
  egg: 'neutral',
  other: 'neutral',
} as const

// Unidade de estoque: o que se CONTA no box.
export const stockUnitLabels = {
  box: 'Caixa',
  kg: 'Quilo (kg)',
  unit: 'Unidade',
  dozen: 'Dúzia',
  bag: 'Saco',
  bunch: 'Maço',
  tray: 'Bandeja',
  pack: 'Pacote',
} as const

// Abreviações para listas e etiquetas ("18 kg por cx").
export const stockUnitShort = {
  box: 'cx',
  kg: 'kg',
  unit: 'un',
  dozen: 'dz',
  bag: 'sc',
  bunch: 'mç',
  tray: 'bdj',
  pack: 'pct',
} as const

// Origem da mercadoria na NF-e (tabela oficial, resumida).
export const productOriginLabels: Record<number, string> = {
  0: '0 - Nacional',
  1: '1 - Estrangeira, importação direta',
  2: '2 - Estrangeira, comprada no mercado interno',
  3: '3 - Nacional, com mais de 40% de conteúdo importado',
  4: '4 - Nacional, processo produtivo básico',
  5: '5 - Nacional, até 40% de conteúdo importado',
  6: '6 - Estrangeira, importação direta, sem similar nacional',
  7: '7 - Estrangeira, mercado interno, sem similar nacional',
  8: '8 - Nacional, mais de 70% de conteúdo importado',
}

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

// Quem compra no box.
export const customerTypeLabels = {
  retailer: 'Varejão / sacolão / hortifrúti',
  supermarket: 'Supermercado',
  restaurant: 'Restaurante / lanchonete',
  street_vendor: 'Feirante / ambulante',
  wholesaler: 'Atacadista / outro box',
  institutional: 'Instituição (escola, hospital...)',
  consumer: 'Consumidor final',
  other: 'Outro',
} as const

export const purchaseModeLabels = {
  direct: 'Compra direta',
  consignment: 'Consignação',
} as const

// Situação de fornecedor e de cliente (os mesmos três valores).
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
