// Atalhos para os tipos gerados do OpenAPI da API (app/types/api.d.ts).
// Nunca edite api.d.ts à mão: rode `make openapi` na API e `npm run api:types` aqui.
import type { components } from './api'

type Schemas = components['schemas']

export type Me = Schemas['Me']
export type Role = Schemas['CurrentUser']['role']
export type User = Schemas['User']
export type Company = Schemas['Company']
export type Supplier = Schemas['Supplier']
export type SupplierSummary = Schemas['SupplierSummary']
export type SupplierFields = Schemas['SupplierFields']
export type City = Schemas['City']
export type Product = Schemas['Product']
export type ProductSummary = Schemas['ProductSummary']
export type ProductFields = Schemas['ProductFields']
export type ApiErrorModel = Schemas['ErrorModel']
