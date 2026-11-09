# API — guia de estudo

Este README é um mapa para aprender Go pelo próprio projeto. Leia os arquivos
na ordem abaixo; os comentários no código explicam cada conceito.

## Ordem de leitura

| # | Arquivo | O que você aprende |
|---|---|---|
| 1 | `cmd/api/main.go` | `package main`, `func main`, tratamento de erro com `if err != nil`, `defer`, goroutines, channels, `select`, graceful shutdown |
| 2 | `internal/config/config.go` | structs, exportado vs. privado (maiúscula/minúscula), variáveis de ambiente |
| 3 | `internal/postgres/postgres.go` | `context.Context`, pool de conexões, `//go:embed`, erros com `%w` |
| 4 | `internal/postgres/migrations/*.sql` | migrations com goose (`-- +goose Up/Down`) |
| 5 | `internal/db/queries/suppliers.sql` | SQL que o sqlc transforma em Go |
| 6 | `internal/db/*.go` | código **gerado** — leia, mas nunca edite à mão |
| 7 | `internal/document/document.go` | pacote "puro", `rune`, `strings.Builder`, `range` |
| 8 | `internal/server/server.go` | `net/http` cru (health check), middlewares, closures, chi, onde o Huma entra |
| 9 | `internal/supplier/supplier.go` | Huma: struct tags, validação, interfaces, métodos, ponteiros, embedding |
| 10 | `*_test.go` | testes table-driven, fakes, `httptest`, `humatest` |

## Conceitos-chave em 1 linha cada

- **Erros são valores**: funções retornam `(resultado, error)`; não existe try/catch.
- **Ponteiro (`*string`)**: aqui usado para "pode ser nulo" — `nil` vira `NULL` no banco e `null` no JSON.
- **Interface é implícita**: `*db.Queries` satisfaz `supplier.Store` só por ter os métodos certos, sem `implements`.
- **`internal/`**: pacotes aqui dentro não podem ser importados por outros módulos.
- **`context.Context`**: carrega cancelamento/timeout de uma requisição até o banco.

## Fluxo de uma requisição

```
HTTP → chi (middlewares: request id, log, recover, CORS)
     → Huma (lê path/query/body → valida tags → Resolve() → chama o handler)
     → handler (supplier.go) → Store (interface) → sqlc → PostgreSQL
     ← Huma serializa a resposta (ou o erro no formato RFC 9457)
```

## Comandos

```bash
make help
```

| Comando | O que faz |
|---|---|
| `make run` | sobe a API (migrations aplicadas no start) |
| `make test` | roda os testes (não precisam de banco) |
| `make sqlc` | regenera `internal/db` depois de mexer em `queries/*.sql` ou migrations |
| `make migration name=create_customers` | cria um novo arquivo de migration |

## Adicionando um novo recurso (ex.: clientes)

1. `make migration name=create_customers` e escreva o `CREATE TABLE`.
2. Escreva as queries em `internal/db/queries/customers.sql`.
3. `make sqlc`.
4. Crie `internal/customer/customer.go` seguindo o modelo de `supplier`.
5. Registre em `internal/server/server.go`: `customer.Register(api, queries)`.
6. Testes em `internal/customer/customer_test.go`.

## Decisões

- **Fornecedor nunca é apagado**: `DELETE /suppliers/{id}` só inativa (`active = false`), porque estoque e compras vão referenciá-lo.
- **CPF ou CNPJ**: produtor rural costuma ter só CPF. O CNPJ alfanumérico (julho/2026) já é aceito.
- **Documento salvo sem máscara**: a API aceita `11.222.333/0001-81` e grava `11222333000181`.
- **Busca ignora acentos** (`unaccent` do PostgreSQL): "sitio" encontra "Sítio".
