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

// Na CEASA todo mundo é conhecido pelo apelido: ele aparece primeiro.
export function displayName(p: { tradeName?: string | null, legalName: string }): string {
  return p.tradeName || p.legalName
}
