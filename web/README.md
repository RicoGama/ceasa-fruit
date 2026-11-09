# Web — guia de estudo

Front em **Nuxt 4 + Nuxt UI 4** (Tailwind CSS 4), consumindo a API em Go.

## Rodando

Com a API rodando em `:8080` (`cd ../api && make run`):

```bash
npm install
```

```bash
npm run dev
```

Abra http://localhost:3000 (ou http://ceasa-fruit.test, se configurou o vhost; ver README da raiz).

## Como o front fala com a API

```
Navegador ──► Nuxt (:3000) ──/api/*──► API Go (:8080)
```

O navegador só conversa com o Nuxt. Tudo que começa com `/api` é repassado
para o Go (`routeRules` em `nuxt.config.ts`). Por ser a **mesma origem**, o
cookie de sessão (HttpOnly) funciona sem CORS e o JavaScript nunca vê o token.

Para a API em outro endereço: `NUXT_API_URL=http://outro-host:8080 npm run dev`.

## Tipos gerados da API

Os tipos de `app/types/api.d.ts` vêm do OpenAPI da API. Quando a API mudar:

```bash
cd ../api && make openapi && cd ../web && npm run api:types
```

Depois rode `npm run typecheck`: se um campo mudou de nome ou tipo no Go, o
TypeScript aponta cada lugar do front que precisa de ajuste.

## Ordem de leitura

| # | Arquivo | O que você aprende |
|---|---|---|
| 1 | `nuxt.config.ts` | módulos, renderização por rota (SSR, SPA, pré-render), proxy |
| 2 | `app/app.vue`, `app/app.config.ts` | raiz do app, tema e idioma do Nuxt UI |
| 3 | `app/plugins/api.ts` | cliente HTTP tipado (openapi-fetch) e interceptação de 401/403 |
| 4 | `app/composables/useAuth.ts` | estado global com `useState`, login/logout/troca de organização |
| 5 | `app/middleware/auth.global.ts` | proteção de rotas antes de cada navegação |
| 6 | `app/layouts/dashboard.vue` | layout com barra lateral (`UDashboard*`) |
| 7 | `app/pages/login.vue`, `signup.vue` | formulários com validação (Zod + `UAuthForm`) |
| 8 | `app/pages/app/suppliers/index.vue` | listagem com busca, filtro, paginação e `useAsyncData` |
| 9 | `app/components/suppliers/SupplierFormModal.vue` | formulário em modal, `defineModel`, erros da API nos campos |
| 10 | `app/utils/*.ts` | funções puras (CPF/CNPJ, tradução de erros, datas) |

## Convenções

- Páginas e componentes usam `<script setup lang="ts">`.
- Arquivos em `components/`, `composables/` e `utils/` são **importados automaticamente**
  pelo Nuxt (por isso não há `import` de `useAuth`, `formatDocument`...).
  Componente em subpasta ganha o prefixo da pasta: `components/suppliers/SupplierFormModal.vue`
  vira `<SuppliersSupplierFormModal>`.
- Textos da tela em português; código em inglês; comentários em português.
- Erros da API chegam em inglês e são traduzidos em `app/utils/api-error.ts`.

## Estrutura de rotas

| Rota | Renderização | Acesso |
|---|---|---|
| `/` | pré-renderizada no build (SEO) | público |
| `/login`, `/signup` | SPA | só deslogado |
| `/app/select-organization` | SPA | logado |
| `/app`, `/app/suppliers` | SPA | logado + organização ativa |
| `/app/admin/organizations` | SPA | admin da plataforma |
