import type { Permission } from '~/types'

// Permite marcar uma página com a permissão que ela exige:
//   definePageMeta({ permission: 'users.manage' })
// O middleware auth.global.ts confere antes de abrir.
declare module '#app' {
  interface PageMeta {
    permission?: Permission
  }
}

export {}
