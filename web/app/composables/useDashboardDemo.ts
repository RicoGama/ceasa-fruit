// Dados ILUSTRATIVOS do painel, enquanto os módulos de cotações, pedidos,
// compras e vendas não existem. Cada bloco será trocado por dados reais da
// API conforme o módulo correspondente for construído.
export function useDashboardDemo() {
  const kpis = [
    { label: 'Cotações em aberto', value: '14', caption: '6 aguardam ação', icon: 'i-lucide-clipboard-list', tone: 'brand' },
    { label: 'Pedidos fechados', value: '9', caption: 'Confirmados até a entrega', icon: 'i-lucide-check', tone: 'brand' },
    { label: 'Cotações perdidas', value: '2', caption: 'Motivos registrados', icon: 'i-lucide-trending-down', tone: 'warning' },
    { label: 'Valor confirmado', value: 'R$ 249.322,40', caption: 'Total dos pedidos fechados', icon: 'i-lucide-chart-column', tone: 'brand' },
  ] as const

  // Volume em kg por fruta: mês atual × mês anterior.
  const chart = {
    purchases: [
      { label: 'Maçã', current: 8000, previous: 6800 },
      { label: 'Pera', current: 4300, previous: 3600 },
      { label: 'Banana', current: 3700, previous: 3000 },
      { label: 'Uva', current: 3100, previous: 2700 },
      { label: 'Manga', current: 2500, previous: 2200 },
    ],
    sales: [
      { label: 'Maçã', current: 7400, previous: 6500 },
      { label: 'Pera', current: 3900, previous: 3500 },
      { label: 'Banana', current: 3500, previous: 3100 },
      { label: 'Uva', current: 2600, previous: 2500 },
      { label: 'Manga', current: 2300, previous: 1900 },
    ],
  }

  const attention = [
    { customer: 'Hortifruti Jardim', code: 'FQ-054', status: 'Aceita, sem reserva', tone: 'brand' },
    { customer: 'Hortifruti Jardim', code: 'FQ-052', status: 'Aceita, sem reserva', tone: 'brand' },
    { customer: 'Mercado Renan', code: 'FQ-051', status: 'Aguardando análise', tone: 'warning' },
    { customer: 'Hortifruti Jardim', code: 'FQ-048', status: 'Aceita, sem reserva', tone: 'brand' },
  ] as const

  return { kpis, chart, attention }
}
