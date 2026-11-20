// Trilha de navegação (breadcrumb) de cada tela, num lugar só.
//
// Cada regra casa o caminho da URL e devolve os níveis acima da tela atual.
// O último nível é a própria tela; quando o nome dela vem do banco (ex.: o
// nome do fornecedor), a página passa esse nome em <AppNavbar :title="...">.
// Níveis sem "to" são só agrupadores (como as seções do menu lateral).

export interface Crumb {
  label: string
  to?: string
  icon?: string
}

const home: Crumb = { label: 'Início', to: '/app', icon: 'i-lucide-house' }
const registries: Crumb = { label: 'Cadastros' }
const admin: Crumb = { label: 'Administração' }
const suppliers: Crumb = { label: 'Fornecedores', to: '/app/suppliers' }
const products: Crumb = { label: 'Produtos', to: '/app/products' }

const rules: [RegExp, Crumb[]][] = [
  [/^\/app\/?$/, [{ ...home, label: 'Visão geral' }]],
  [/^\/app\/suppliers\/?$/, [home, registries, { label: 'Fornecedores' }]],
  [/^\/app\/suppliers\/new$/, [home, registries, suppliers, { label: 'Novo fornecedor' }]],
  [/^\/app\/suppliers\/[^/]+$/, [home, registries, suppliers, { label: 'Fornecedor' }]],
  [/^\/app\/products\/?$/, [home, registries, { label: 'Produtos' }]],
  [/^\/app\/products\/new$/, [home, registries, products, { label: 'Novo produto' }]],
  [/^\/app\/products\/[^/]+$/, [home, registries, products, { label: 'Produto' }]],
  [/^\/app\/settings\/users$/, [home, admin, { label: 'Usuários' }]],
  [/^\/app\/settings\/company$/, [home, admin, { label: 'Dados da empresa' }]],
]

// breadcrumbTrail('/app/suppliers/42', 'Sítio do Zé')
//   -> Início / Cadastros / Fornecedores / Sítio do Zé
export function breadcrumbTrail(path: string, title?: string): Crumb[] {
  const rule = rules.find(([re]) => re.test(path))
  const trail = rule ? rule[1].map(c => ({ ...c })) : [home]
  const current = trail[trail.length - 1]!
  if (title) current.label = title
  delete current.to // a tela atual não é link
  return trail
}

// O nível "pai" com link (para o botão voltar no celular).
export function breadcrumbParent(trail: Crumb[]): Crumb | undefined {
  return trail.slice(0, -1).reverse().find(c => c.to)
}
