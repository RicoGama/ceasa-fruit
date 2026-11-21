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

const dateTimeFormatter = new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'short' })
const timeFormatter = new Intl.DateTimeFormat('pt-BR', { timeStyle: 'short' })

// 21/11/2026 14:32
export function formatDateTime(iso: string): string {
  return dateTimeFormatter.format(new Date(iso))
}

// "hoje 14:32", "ontem 09:10" ou "18/11/2026 16:05"
export function formatWhen(iso: string): string {
  const d = new Date(iso)
  const today = new Date()
  const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1)
  if (d.toDateString() === today.toDateString()) return `hoje ${timeFormatter.format(d)}`
  if (d.toDateString() === yesterday.toDateString()) return `ontem ${timeFormatter.format(d)}`
  return dateTimeFormatter.format(d)
}

// Duração entre duas datas: "menos de 1 min", "8 min", "2 h 15 min", "3 dias"
export function formatDuration(fromIso: string, toIso: string): string {
  const min = Math.round((new Date(toIso).getTime() - new Date(fromIso).getTime()) / 60000)
  if (min < 1) return 'menos de 1 min'
  if (min < 60) return `${min} min`
  const h = Math.floor(min / 60)
  if (h < 24) return min % 60 ? `${h} h ${min % 60} min` : `${h} h`
  const days = Math.round(h / 24)
  return days === 1 ? '1 dia' : `${days} dias`
}

// Resumo do navegador e do aparelho a partir do User-Agent:
// "Chrome · Android", "Safari · iPhone", "Edge · Windows".
export function describeDevice(ua?: string | null): { label: string, icon: string } {
  if (!ua) return { label: 'Desconhecido', icon: 'i-lucide-circle-help' }
  const browser
    = /Edg\//.test(ua) ? 'Edge'
      : /OPR\//.test(ua) ? 'Opera'
        : /SamsungBrowser/.test(ua) ? 'Samsung Internet'
          : /Firefox\//.test(ua) ? 'Firefox'
            : /Chrome\//.test(ua) ? 'Chrome'
              : /Safari\//.test(ua) ? 'Safari'
                : ua.split(/[ /]/)[0] || 'Navegador'
  const system
    = /iPhone/.test(ua) ? 'iPhone'
      : /iPad/.test(ua) ? 'iPad'
        : /Android/.test(ua) ? 'Android'
          : /Windows/.test(ua) ? 'Windows'
            : /Mac OS X|Macintosh/.test(ua) ? 'Mac'
              : /Linux/.test(ua) ? 'Linux'
                : ''
  const mobile = /iPhone|Android.*Mobile|Mobile Safari/.test(ua)
  const tablet = /iPad|Android(?!.*Mobile)/.test(ua)
  return {
    label: system ? `${browser} · ${system}` : browser,
    icon: mobile ? 'i-lucide-smartphone' : tablet ? 'i-lucide-tablet' : 'i-lucide-monitor',
  }
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
