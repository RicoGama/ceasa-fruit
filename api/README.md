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
| 14 | `internal/party/party.go` | dados comuns de pessoa (endereço, contatos, PIX), normalização, structs aninhadas |
| 15 | `internal/supplier/supplier.go` | Huma: struct tags, validação, interfaces, embedding, closures, transação com várias tabelas |
| 16 | `internal/city/city.go` | uma rota pequena de consulta (municípios do IBGE) |
| 17 | `internal/admin/admin.go` | um recurso simples, para fixar o padrão |
| 18 | `internal/mail/` | e-mail por SMTP, modelos com `html/template` e `text/template`, `//go:embed` |
| 19 | `internal/queue/queue.go` | fila de jobs com River: job, worker, enfileirar dentro da transação |
| 20 | `cmd/worker/main.go` | um segundo executável, só com os workers |
| 21 | `*_test.go` | testes table-driven, fakes, `httptest`, `humatest`, teste de integração |

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
| Pessoa (física/jurídica) | `parties` | dados comuns: documento, nomes, endereço, PIX/banco |
| Contatos da pessoa | `party_contacts` | dono, gerente, motorista... |
| Fornecedor | `suppliers` | papel "fornecedor" de uma pessoa (consignação, prazo, box) |
| Cliente do cliente | `customers` | *(próximo passo: outro papel da mesma pessoa)* |
| Municípios/UFs | `cities`, `states` | lista oficial do IBGE (sem organização, sem RLS) |

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

## Fila de jobs (River) e e-mail

Equivalente às Jobs/Queues do Laravel, com o [River](https://riverqueue.com),
que guarda a fila no próprio PostgreSQL (sem Redis).

| Laravel | Aqui |
|---|---|
| classe Job + `handle()` | `EmailArgs` (dados) + `EmailWorker.Work()` em `internal/queue` |
| `dispatch()` | `queue.EnqueueEmail(ctx, tx, msg)` — **dentro da transação** |
| `queue:work` | os workers sobem junto com a API (`WORKERS_ENABLED=true`) ou `make worker` |
| `$tries` / backoff | `MaxAttempts: 10`, espera crescente entre tentativas |
| Horizon | River UI em `/admin/queues` (só admin da plataforma) |

Regras do projeto:

- **Todo e-mail sai por SMTP** (`internal/mail`, biblioteca go-mail) e **sempre
  pela fila**: nenhuma rota espera o servidor de e-mail.
- **Enfileire dentro da transação** do que gerou o e-mail. Se der rollback, o
  job some junto; se gravar, o e-mail com certeza sai (mesmo que o SMTP esteja
  fora do ar: ele é tentado de novo).
- Modelos de e-mail em `internal/mail/templates/`: `<nome>.html.tmpl` (usa o
  layout) e `<nome>.txt.tmpl` (assunto + texto puro).

### Novo tipo de job

1. Em `internal/queue`, crie `XxxArgs` com `Kind()` (nome fixo, não mude depois)
   e `InsertOpts()` (fila, tentativas).
2. Crie `XxxWorker` com `Work(ctx, job)`; devolver erro = tentar de novo.
3. Registre em `queue.New` com `river.AddWorker(workers, &XxxWorker{...})`.
4. Enfileire com `q.Client.InsertTx(ctx, tx, XxxArgs{...}, nil)`.
5. Se o job mexe em dados de uma organização, guarde o `organization_id` nos
   argumentos e use `postgres.InTenant` dentro do `Work`.

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
| `make mailpit` | SMTP de desenvolvimento (e-mails em http://localhost:8025) |
| `make worker` | só os workers da fila (opcional; a API já executa os jobs) |

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

- **Cadastro único de pessoas** (`parties`): na CEASA o mesmo CNPJ costuma ser
  fornecedor e cliente. Os dados comuns ficam uma vez só; cada papel tem sua tabela.
- **Município pelo código IBGE** (`cities.id`): é o código que a nota fiscal exige.
  A lista vem da migration `00003`, gerada por `scripts/gen-cities-migration.py`.
- **Consignação ou compra direta** por fornecedor (padrão), com comissão em %
  obrigatória na consignação. Cada compra poderá mudar a modalidade.
- **Documento opcional**: produtor pequeno pode ser cadastrado antes de informar
  o CPF; quando informado, é único na organização e precisa combinar com o tipo de pessoa.

- **Fornecedor nunca é apagado**: `DELETE /suppliers/{id}` só inativa (`active = false`), porque estoque e compras vão referenciá-lo.
- **CPF ou CNPJ**: produtor rural costuma ter só CPF. O CNPJ alfanumérico (julho/2026) já é aceito.
- **Documento único por organização**: duas distribuidoras podem cadastrar o mesmo produtor.
- **Documento salvo sem máscara**: a API aceita `11.222.333/0001-81` e grava `11222333000181`.
- **Busca ignora acentos** (`unaccent` do PostgreSQL): "sitio" encontra "Sítio".
- **Recuperação de senha**: link por e-mail com token de uso único (1h), só o
  hash fica no banco; ao trocar a senha, todas as sessões do usuário caem. A rota
  "esqueci a senha" responde igual exista o e-mail ou não.
- **Organização suspensa** bloqueia todos os usuários dela nas rotas de negócio (403).
