// Atalhos para os tipos gerados do OpenAPI da API (app/types/api.d.ts).
// Nunca edite api.d.ts à mão: rode `make openapi` na API e `npm run api:types` aqui.
import type { components } from './api'

type Schemas = components['schemas']

export type Me = Schemas['Me']
export type OrganizationMembership = Schemas['OrganizationMembership']
export type Supplier = Schemas['Supplier']
export type AdminOrganization = Schemas['AdminOrganization']
export type ApiErrorModel = Schemas['ErrorModel']
