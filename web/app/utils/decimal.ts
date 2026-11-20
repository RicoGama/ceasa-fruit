// Dinheiro, quantidade e percentual no front.
//
// A API manda e recebe esses valores como TEXTO no formato canônico, com ponto
// decimal e sem separador de milhar: "1234.56" (R$), "12.5" (kg), "12.5" (%).
// Aqui NUNCA convertemos dinheiro para number: o JavaScript usa float, e
// 0.1 + 0.2 daria 0.30000000000000004. Formatar e ler são só manipulação de
// texto; contas de dinheiro ficam na API (Go, money.Money).

// Separa "-1234.5" em sinal, parte inteira e parte decimal.
function split(value: string) {
  const negative = value.startsWith('-')
  const [intPart = '0', frac = ''] = value.replace(/^-/, '').split('.')
  return { negative, intPart, frac }
}

// "1234567" -> "1.234.567"
function groupThousands(digits: string) {
  return digits.replace(/\B(?=(\d{3})+(?!\d))/g, '.')
}

// Formato brasileiro de um decimal canônico.
//   minDecimals: casas sempre mostradas (dinheiro = 2); as outras só se não forem zero.
function formatDecimal(value: string, minDecimals: number) {
  const { negative, intPart, frac } = split(value)
  let decimals = frac.replace(/0+$/, '')
  if (decimals.length < minDecimals) decimals = decimals.padEnd(minDecimals, '0')
  const text = groupThousands(intPart) + (decimals ? `,${decimals}` : '')
  return negative ? `-${text}` : text
}

// "1234.5" -> "R$ 1.234,50"
export function formatMoney(value?: string | null): string {
  if (value == null || value === '') return '—'
  return `R$ ${formatDecimal(value, 2)}`
}

// "12.500" -> "12,5"  (a unidade, kg ou cx, vem do produto)
export function formatQuantity(value?: string | null): string {
  if (value == null || value === '') return '—'
  return formatDecimal(value, 0)
}

// "12.5" -> "12,5%"
export function formatPercent(value?: string | null): string {
  if (value == null || value === '') return '—'
  return `${formatDecimal(value, 0)}%`
}

// Lê o que o usuário digitou no padrão brasileiro e devolve o formato da API.
// "1.234,56" -> "1234.56"; "12,5" -> "12.5"; "10" -> "10".
// Devolve null se não for um número válido ou tiver mais casas que `scale`.
// (Com ponto e sem vírgula, o ponto é milhar: "1.234" = mil duzentos e trinta e quatro.)
export function parseDecimalInput(input: string, scale: number): string | null {
  const text = input.trim().replace(/^R\$\s*/, '').replace(/%$/, '').trim()
  const match = /^(-)?(\d{1,3}(?:\.\d{3})*|\d+)(?:,(\d+))?$/.exec(text)
  if (!match) return null
  const [, sign = '', intPart = '', frac = ''] = match
  if (frac.length > scale) return null
  const digits = intPart.replace(/\./g, '').replace(/^0+(?=\d)/, '')
  return `${sign}${digits}${frac ? `.${frac}` : ''}`
}

// Para campos UInputNumber (percentuais com poucas casas): number <-> texto da API.
// Seguro para valores digitados com até `scale` casas; não use para somar dinheiro.
export function numberToDecimal(value: number | null | undefined, scale: number): string | undefined {
  if (value == null || Number.isNaN(value)) return undefined
  return value.toFixed(scale).replace(/\.?0+$/, '')
}

export function decimalToNumber(value?: string | null): number | null {
  return value == null || value === '' ? null : Number(value)
}
