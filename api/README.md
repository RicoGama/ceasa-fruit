# API — guia de estudo

Este README é um mapa para aprender Go pelo próprio projeto. Leia os arquivos
na ordem abaixo; os comentários no código explicam cada conceito.

## Ordem de leitura

| # | Arquivo | O que você aprende |
|---|---|---|
| 1 | `cmd/api/main.go` | `package main`, `func main`, tratamento de erro com `if err != nil`, `defer`, goroutines, channels, `select`, graceful shutdown |
| 2 | `internal/config/config.go` | structs, exportado vs. privado (maiúscula/minúscula), variáveis de ambiente |
| 3 | `internal/postgres/postgres.go` | `context.Context`, pool de conexões, `//go:embed`, erros com `%w` |
| 4 | `internal/postgres/migrations/*.sql` | migrations com goose (`-- +goose Up/Down`), Row-Level Security |
| 5 | `internal/db/queries/*.sql` | SQL que o sqlc transforma em Go |
| 6 | `internal/db/*.go` | código **gerado** — leia, mas nunca edite à mão |
| 7 | `internal/document/document.go` | pacote "puro", `rune`, `strings.Builder`, `range` |
| 8 | `internal/server/server.go` | `net/http` cru (health check), middlewares, closures, chi, grupos de rotas do Huma |
| 9 | `internal/auth/password.go` | hash de senha (Argon2id), comparação em tempo constante, `sync.OnceValue` |
| 10 | `internal/auth/session.go` | tokens, cookies, valores no `context.Context`, middleware net/http |
| 11 | `internal/auth/middleware.go` | middlewares do Huma (controle de acesso) |
| 12 | `internal/auth/handlers.go` | transações (`pgx.BeginFunc`), cadastro/login, cookies na resposta |
| 13 | `internal/postgres/tenant.go` | transação "presa" à organização (RLS) |
| 14 | `internal/supplier/supplier.go` | Huma: struct tags, validação, interfaces, métodos, ponteiros, embedding, closures |
| 15 | `internal/admin/admin.go` | um recurso simples, para fixar o padrão |
| 16 | `*_test.go` | testes table-driven, fakes, `httptest`, `humatest`, teste de integração |

## Conceitos-chave em 1 linha cada

- **Erros são valores**: funções retornam `(resultado, error)`; não existe try/catch.
- **Ponteiro (`*string`)**: aqui usado para "pode ser nulo" — `nil` vira `NULL` no banco e `null` no JSON.
- **Interface é implícita**: `*db.Queries` satisfaz `supplier.Store` só por ter os métodos certos, sem `implements`.
- **`internal/`**: pacotes aqui dentro não podem ser importados por outros módulos.
- **`context.Context`**: carrega cancelamento/timeout e o usuário logado de uma requisição até o banco.

## Multi-tenant: como funciona

| Conceito | Tabela | Quem é |
|---|---|---|
| Cliente do SaaS | `organizations` | a distribuidora que assina a plataforma |
| Usuário | `users` | quem faz login; pode participar de **várias** organizações |
| Vínculo | `memberships` | usuário + organização + papel (`owner`, `admin`, `member`) |
| Sessão | `sessions` | login ativo + organização selecionada no momento |
| Cliente do cliente | `customers` | *(próximo passo)* |

Toda tabela de negócio tem `organization_id` e é protegida em **duas camadas**:

1. **No código**: toda query do sqlc filtra por `organization_id`, que vem da
   sessão (nunca do corpo da requisição).
2. **No banco (RLS)**: as rotas de negócio rodam dentro de `postgres.InTenant`,
   que troca para o papel `ceasa_app` e define `app.organization_id`. O Postgres
   então esconde as linhas das outras organizações, **mesmo que uma query
   esqueça o filtro**. O teste de integração prova isso com SQL cru.

### Grupos de rotas (`server.go`)

| Grupo | Exige | Rotas |
|---|---|---|
| `public` | nada | `POST /auth/signup`, `POST /auth/login`, `POST /auth/logout` |
| `authed` | login | `GET /auth/me`, `PUT /auth/organization` |
| `tenant` | login + organização ativa | `/suppliers/...` (e tudo de negócio que vier) |
| `platformAdmin` | admin da plataforma | `/admin/organizations/...` |

### Login

- Senha com **Argon2id**; o banco guarda só o hash.
- Sessão em **cookie HttpOnly + SameSite=Lax**: o JavaScript não lê o token
  (protege de XSS) e outros sites não conseguem usá-lo (protege de CSRF).
- O banco guarda só o **SHA-256 do token**: se o banco vazar, as sessões não vazam junto.
- Login com uma organização já entra nela; com várias, o front chama
  `PUT /auth/organization` com a escolhida.

### Primeiro admin da plataforma

Não há rota para virar admin, de propósito. Cadastre-se pela API e rode:

```bash
make promote-admin email=voce@exemplo.com
```

## Fluxo de uma requisição

```
HTTP → chi (request id, log, recover, CORS, LoadSession: cookie → sessão no context)
     → Huma (grupo: Require* barra quem não pode → valida tags → Resolve())
     → handler (supplier.go) → InTenant (transação com RLS) → sqlc → PostgreSQL
     ← Huma serializa a resposta (ou o erro no formato RFC 9457)
```

## Comandos

```bash
make help
```

| Comando | O que faz |
|---|---|
| `make run` | sobe a API (migrations aplicadas no start) |
| `make test` | testes unitários (não precisam de banco) |
| `make test-integration` | todos os testes, incluindo os de integração no banco `ceasa_test` |
| `make sqlc` | regenera `internal/db` depois de mexer em `queries/*.sql` ou migrations |
| `make migration name=create_customers` | cria um novo arquivo de migration |
| `make promote-admin email=...` | torna um usuário admin da plataforma |

## Adicionando um novo recurso de negócio (ex.: clientes)

1. `make migration name=create_customers`: `CREATE TABLE` com
   `organization_id BIGINT NOT NULL REFERENCES organizations (id)`, mais os
   `GRANT ... TO ceasa_app`, `ENABLE ROW LEVEL SECURITY` e a `POLICY`
   (copie o bloco da migration `00002`).
2. Queries em `internal/db/queries/customers.sql`, todas com `organization_id = @organization_id`.
3. `make sqlc`.
4. `internal/customer/customer.go` seguindo o modelo de `supplier` (usa `auth.OrganizationID(ctx)` e `TxFunc`).
5. Registre no grupo `tenant` em `internal/server/server.go`.
6. Testes em `internal/customer/customer_test.go` e um caso no teste de integração.

## Decisões

- **Fornecedor nunca é apagado**: `DELETE /suppliers/{id}` só inativa (`active = false`), porque estoque e compras vão referenciá-lo.
- **CPF ou CNPJ**: produtor rural costuma ter só CPF. O CNPJ alfanumérico (julho/2026) já é aceito.
- **Documento único por organização**: duas distribuidoras podem cadastrar o mesmo produtor.
- **Documento salvo sem máscara**: a API aceita `11.222.333/0001-81` e grava `11222333000181`.
- **Busca ignora acentos** (`unaccent` do PostgreSQL): "sitio" encontra "Sítio".
- **Organização suspensa** bloqueia todos os usuários dela nas rotas de negócio (403).
