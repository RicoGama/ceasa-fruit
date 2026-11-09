// CPF/CNPJ no front: mesma regra do pacote Go internal/document.
// Validar aqui é só conforto para o usuário (erro na hora); quem garante é a API.

export function normalizeDocument(value: string): string {
  return value.toUpperCase().replace(/[^0-9A-Z]/g, '')
}

// Aplica a máscara para exibição: 529.982.247-25 / 11.222.333/0001-81
export function formatDocument(value: string): string {
  const d = normalizeDocument(value)
  if (d.length === 11) return d.replace(/^(.{3})(.{3})(.{3})(.{2})$/, '$1.$2.$3-$4')
  if (d.length === 14) return d.replace(/^(.{2})(.{3})(.{3})(.{4})(.{2})$/, '$1.$2.$3/$4-$5')
  return value
}

// Cada caractere vale (código ASCII - 48): '0'..'9' = 0..9, 'A'..'Z' = 17..42.
function checkDigit(base: string, weights: number[]): number {
  let sum = 0
  for (let i = 0; i < base.length; i++) sum += (base.charCodeAt(i) - 48) * weights[i]!
  const remainder = sum % 11
  return remainder < 2 ? 0 : 11 - remainder
}

export function isValidDocument(value: string): boolean {
  const d = normalizeDocument(value)
  if (/^(.)\1*$/.test(d)) return false // todos iguais

  if (d.length === 11) {
    if (!/^\d{11}$/.test(d)) return false
    return checkDigit(d.slice(0, 9), [10, 9, 8, 7, 6, 5, 4, 3, 2]) === Number(d[9])
      && checkDigit(d.slice(0, 10), [11, 10, 9, 8, 7, 6, 5, 4, 3, 2]) === Number(d[10])
  }
  if (d.length === 14) {
    if (!/^[0-9A-Z]{12}\d{2}$/.test(d)) return false
    return checkDigit(d.slice(0, 12), [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2]) === Number(d[12])
      && checkDigit(d.slice(0, 13), [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2]) === Number(d[13])
  }
  return false
}
