const dateFormatter = new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short' })

export function formatDate(iso: string): string {
  return dateFormatter.format(new Date(iso))
}

export function onlyDigits(value?: string | null): string {
  return (value ?? '').replace(/\D/g, '')
}

// (21) 99999-0000, (21) 3333-0000, +55 (21) 99999-0000
export function formatPhone(value?: string | null): string {
  const d = onlyDigits(value)
  if (d.length === 11) return d.replace(/^(\d{2})(\d{5})(\d{4})$/, '($1) $2-$3')
  if (d.length === 10) return d.replace(/^(\d{2})(\d{4})(\d{4})$/, '($1) $2-$3')
  if (d.length === 13 && d.startsWith('55')) return '+55 ' + formatPhone(d.slice(2))
  return value ?? ''
}

// 25960-000
export function formatPostalCode(value?: string | null): string {
  const d = onlyDigits(value)
  return d.length === 8 ? `${d.slice(0, 5)}-${d.slice(5)}` : (value ?? '')
}

// Safra: meses em que o fornecedor tem o produto.
//   []              -> "Ano todo"
//   [1,2,3,12]      -> "dez–mar"      (atravessa a virada do ano)
//   [5,6,9]         -> "mai–jun, set"
export const monthShort = ['jan', 'fev', 'mar', 'abr', 'mai', 'jun', 'jul', 'ago', 'set', 'out', 'nov', 'dez']

export function formatMonths(months?: number[] | null): string {
  const sorted = [...new Set(months ?? [])].sort((a, b) => a - b)
  if (!sorted.length || sorted.length === 12) return 'Ano todo'
  const has = new Set(sorted)
  // Começa num mês cujo anterior não está na lista (dez–mar sai inteiro).
  const start = sorted.findIndex(m => !has.has(m === 1 ? 12 : m - 1))
  const ordered = [...sorted.slice(start), ...sorted.slice(0, start)]

  const ranges: string[] = []
  let first = ordered[0]!
  let last = first
  const close = () => ranges.push(first === last ? monthShort[first - 1]! : `${monthShort[first - 1]}–${monthShort[last - 1]}`)
  for (const m of ordered.slice(1)) {
    if (m === (last % 12) + 1) {
      last = m // continua a sequência
      continue
    }
    close()
    first = m
    last = m
  }
  close()
  return ranges.join(', ')
}

// NCM: 0702.00.00
export function formatNcm(value?: string | null): string {
  const d = onlyDigits(value)
  return d.length === 8 ? `${d.slice(0, 4)}.${d.slice(4, 6)}.${d.slice(6)}` : (value ?? '')
}

// Na CEASA todo mundo é conhecido pelo apelido: ele aparece primeiro.
export function displayName(p: { tradeName?: string | null, legalName: string }): string {
  return p.tradeName || p.legalName
}
