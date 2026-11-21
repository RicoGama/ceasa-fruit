import type { Permission } from '~/types'

// Como as permissões aparecem na tela de papéis: agrupadas por área, com o
// nome do recurso na linha e as ações nas colunas. O catálogo em si (os
// códigos) vem da API; aqui ficam só os textos em português e a arrumação.
//
// Permissão nova na API: o TypeScript reclama aqui embaixo até ela ganhar um
// lugar nesta lista (veja o _everyPermissionListed).

export interface PermissionAction { permission: Permission, label: string }
export interface PermissionResource { label: string, description?: string, actions: PermissionAction[] }
export interface PermissionGroup { label: string, resources: PermissionResource[] }

export const permissionGroups = [
  {
    label: 'Cadastros',
    resources: [
      {
        label: 'Fornecedores',
        actions: [
          { permission: 'suppliers.view', label: 'Ver' },
          { permission: 'suppliers.edit', label: 'Cadastrar e editar' },
        ],
      },
      {
        label: 'Produtos',
        actions: [
          { permission: 'products.view', label: 'Ver' },
          { permission: 'products.edit', label: 'Cadastrar e editar' },
        ],
      },
    ],
  },
  {
    label: 'Administração',
    resources: [
      {
        label: 'Usuários e papéis',
        description: 'Criar usuários, definir papéis e permissões',
        actions: [{ permission: 'users.manage', label: 'Gerenciar' }],
      },
      {
        label: 'Dados da empresa',
        description: 'CNPJ, endereço e dados fiscais',
        actions: [{ permission: 'company.edit', label: 'Alterar' }],
      },
      {
        label: 'Registro de acessos',
        description: 'Quem entrou, quando, de onde e como saiu',
        actions: [{ permission: 'audit.view', label: 'Ver' }],
      },
      {
        label: 'Fila de tarefas',
        description: 'Painel técnico dos envios em segundo plano',
        actions: [{ permission: 'queues.view', label: 'Ver' }],
      },
    ],
  },
] as const satisfies readonly PermissionGroup[]

type Listed = (typeof permissionGroups)[number]['resources'][number]['actions'][number]['permission']
// Falha na compilação se alguma permissão da API não estiver na lista acima.
const _everyPermissionListed: Exclude<Permission, Listed> extends never ? true : never = true
void _everyPermissionListed

// Todos os recursos numa lista só (para resumos, como nos cartões de papéis).
export const permissionResources: readonly PermissionResource[] = (permissionGroups as readonly PermissionGroup[]).flatMap(g => g.resources)

// Igual à API (auth.Implies): quem edita também precisa ver.
export const permissionImplies: Partial<Record<Permission, Permission>> = {
  'suppliers.edit': 'suppliers.view',
  'products.edit': 'products.view',
}

// Marca/desmarca respeitando as dependências: marcar "editar" marca "ver";
// desmarcar "ver" desmarca "editar".
export function togglePermission(current: Permission[], p: Permission, on: boolean): Permission[] {
  const set = new Set(current)
  if (on) {
    set.add(p)
    const implied = permissionImplies[p]
    if (implied) set.add(implied)
  }
  else {
    set.delete(p)
    for (const [dependent, required] of Object.entries(permissionImplies)) {
      if (required === p) set.delete(dependent as Permission)
    }
  }
  return [...set]
}
