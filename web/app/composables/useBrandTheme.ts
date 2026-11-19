// Tema de cor do sistema (azul ou verde), lembrado num cookie para valer
// também na primeira renderização. Aplicado em <html data-theme="...">.
export type BrandTheme = 'blue' | 'green'

export const brandThemes: { value: BrandTheme, label: string, swatch: string }[] = [
  { value: 'blue', label: 'Azul', swatch: '#0149a2' },
  { value: 'green', label: 'Verde', swatch: '#276848' },
]

export function useBrandTheme() {
  return useCookie<BrandTheme>('brand-theme', {
    default: () => 'blue',
    maxAge: 60 * 60 * 24 * 365,
    sameSite: 'lax',
  })
}
