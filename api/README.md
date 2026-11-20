# API — guia de estudo

Este README é um mapa para aprender Go pelo próprio projeto. Leia os arquivos
na ordem abaixo; os comentários no código explicam cada conceito.

## Ordem de leitura

| # | Arquivo | O que você aprende |
|---|---|---|
| 1 | `cmd/api/main.go` | `package main`, `func main`, tratamento de erro com `if err != nil`, `defer`, goroutines, channels, `select`, graceful shutdown |
| 2 | `internal/config/config.go` | structs, exportado vs. privado (maiúscula/minúscula), variáveis de ambiente |
| 3 | `internal/postgres/postgres.go`, `tx.go` | `context.Context`, pool de conexões, `//go:embed`, erros com `%w`, **transações** (a regra do projeto) |
| 4 | `internal/postgres/migrations/*.sql` | migrations com goose (`-- +goose Up/Down`), `CHECK`, índices parciais |
| 5 | `internal/db/queries/*.sql` | SQL que o sqlc transforma em Go |
| 6 | `internal/db/*.go` | código **gerado** — leia, mas nunca edite à mão |
| 7 | `internal/document/document.go` | pacote "puro", `rune`, `strings.Builder`, `range` |
| 7b | `internal/fixed`, `internal/money`, `internal/quantity` | tipos próprios (`type Money int64`), métodos, interfaces do JSON/Huma/pgx, `math/big` |
| 8 | `internal/server/server.go` | `net/http` cru (health check), middlewares, closures, chi, grupos de rotas do Huma |
| 9 | `internal/auth/password.go` | hash de senha (Argon2id), comparação em tempo constante, `sync.OnceValue` |
| 10 | `internal/auth/session.go` | tokens, cookies, valores no `context.Context`, middleware net/http |
| 11 | `internal/auth/middleware.go`, `roles.go` | middlewares do Huma (login e papéis) |
| 12 | `internal/auth/handlers.go` | transações (`pgx.BeginFunc`), login, recuperação de senha, cookies na resposta |
| 13 | `internal/user/user.go` | CRUD de usuários só para admin, convite por e-mail na mesma transação |
| 14 | `internal/party/party.go` | dados comuns de pessoa (endereço, contatos, PIX), normalização, structs aninhadas |
| 15 | `internal/supplier/supplier.go` | Huma: struct tags, validação, interfaces, embedding, closures, transação com várias tabelas |
| 16 | `internal/city/city.go` | uma rota pequena de consulta (municípios do IBGE) |
| 17 | `internal/company/company.go` | registro único (singleton), `Resolve()` para validar, erro de chave estrangeira |
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

## Um servidor por cliente (sem multi-tenant)

O sistema roda num **servidor Linux dentro do box**, porque a internet da
CEASA não é confiável. Cada cliente tem o seu servidor e o seu banco; por isso
não existe "organização" nas tabelas: o banco inteiro é de uma empresa só.

| Conceito | Tabela | Quem é |
|---|---|---|
| Empresa | `company` | a dona do servidor; uma linha só (`id = 1`, garantido por `CHECK`) |
| Usuário | `users` | quem faz login, com **um papel** e ativo/desativado |
| Sessão | `sessions` | login ativo (só o hash do token) |
| Pessoa (física/jurídica) | `parties` | dados comuns: documento, nomes, endereço, PIX/banco |
| Contatos da pessoa | `party_contacts` | dono, gerente, motorista... |
| Fornecedor | `suppliers` | papel "fornecedor" de uma pessoa (consignação, prazo, box) |
| Cliente | `customers` | *(próximo passo: outro papel da mesma pessoa)* |
| Municípios/UFs | `cities`, `states` | lista oficial do IBGE |

### Papéis

| Papel | Código | Para quem |
|---|---|---|
| Administrador | `admin` | acesso total; único que gerencia usuários e dados da empresa |
| Gerente | `manager` | acompanha a operação, preços e relatórios |
| Vendedor | `seller` | atende clientes e lança pedidos |
| Conferente | `checker` | confere entrada e saída de mercadoria |
| Financeiro | `finance` | boletos, cheques, contas a pagar e receber |
| Motorista | `driver` | vê as entregas da sua rota |

Nas rotas, `auth.RequireRole(api, auth.RoleFinance, ...)` libera só os papéis
listados; o **admin sempre passa**. Por enquanto, fora as telas de
administração, todos os papéis podem tudo; as permissões finas entram com cada
módulo (ex.: motorista só consulta).

### Grupos de rotas (`server.go`)

| Grupo | Exige | Rotas |
|---|---|---|
| `public` | nada | `POST /auth/login`, `POST /auth/logout`, `POST /auth/password/forgot`, `POST /auth/password/reset` |
| `authed` | login | `GET /auth/me`, `GET /company`, `/cities`, `/suppliers/...` |
| `adminOnly` | papel `admin` | `/users/...`, `PUT /company` |

### Login

- Senha com **Argon2id**; o banco guarda só o hash.
- Sessão em **cookie HttpOnly + SameSite=Lax**: o JavaScript não lê o token
  (protege de XSS) e outros sites não conseguem usá-lo (protege de CSRF).
- O banco guarda só o **SHA-256 do token**: se o banco vazar, as sessões não vazam junto.
- **Não há cadastro aberto**: quem cria usuários é o admin. Sem informar senha,
  o usuário recebe um **convite por e-mail** (link válido por 3 dias) para criar
  a própria senha.
- Desativar um usuário derruba as sessões dele na hora. O sistema não deixa o
  admin se desativar nem ficar sem nenhum admin ativo.

### Primeiro acesso (empresa + admin)

```bash
make seed                                   # empresa "Irmãos Faisão Frutícola" + admin@ceasafruit.test com senha aleatória
make seed email=voce@exemplo.com password=uma-senha-boa
```

O seed é idempotente: rodar de novo só redefine a senha e garante que o
usuário é admin ativo. Os dados completos da empresa (CNPJ, IE, endereço) são
preenchidos na tela *Administração → Dados da empresa*.

## Regra: transação para tudo

**Todo acesso ao banco acontece dentro de uma transação**, inclusive leituras.

```go
// gravação (READ COMMITTED): tudo ou nada
err := h.db.InTx(ctx, func(tx postgres.Tx) error {
    user, err := tx.CreateUser(ctx, ...)            // queries do sqlc
    ...
    return emails.EnqueueEmail(ctx, tx.Raw, msg)    // a fila entra junto
})

// leitura (REPEATABLE READ, só leitura): todas as consultas veem o mesmo instante
err := h.db.InReadTx(ctx, func(tx postgres.Tx) error { ... })
```

- Os pacotes de negócio recebem um `postgres.DB`, nunca o pool nem as queries
  soltas: sem transação, não há como chegar no banco.
- Devolver erro dentro da função = `ROLLBACK` de tudo; devolver `nil` = `COMMIT`.
- Disputa pela mesma linha (estoque, número da nota): `SELECT ... FOR UPDATE`
  na query, dentro do `InTx`.
- O teste `internal/postgres/rules_test.go` lê o código e reprova quem abrir
  transação por conta própria (`pgx.BeginFunc`, `.Begin`), criar queries
  soltas (`db.New`) ou receber o `pgxpool` num pacote de negócio.

## Concorrência

Muita gente mexendo ao mesmo tempo (vendedores, conferentes, financeiro). As
ferramentas, de acordo com o problema:

| Problema | Ferramenta | Onde já está |
|---|---|---|
| Duas pessoas editam o mesmo cadastro; a segunda apaga a alteração da primeira | **Versão da linha** (concorrência otimista): a tela manda a `version` que leu; o `UPDATE ... WHERE version = @version` só passa se ninguém salvou no meio. Senão, **409** e o front oferece recarregar | fornecedor, empresa, usuários |
| Duas transações disputam a mesma linha (estoque, próximo número da nota, último admin) | **`SELECT ... FOR UPDATE`**: a segunda espera a primeira terminar e enxerga o valor novo. Travar sempre na mesma ordem (por id) | último admin (`LockActiveAdmins`) |
| Deadlock / falha de serialização | `InTx` **repete a transação sozinho** (até 3 vezes). Por isso a função dentro do `InTx` só mexe no banco: SEFAZ, banco e e-mail vão pela fila | todas as transações |
| Trava presa por muito tempo | `lock_timeout` 5s → **503** "tente de novo"; `statement_timeout` 30s; transação esquecida aberta cai em 60s | conexão (`postgres.Connect`) |
| Mesmo documento cadastrado duas vezes ao mesmo tempo | **índice único** no banco: a segunda recebe 409 | CPF/CNPJ, e-mail |
| Disputa de memória dentro do Go | testes com **`-race`** (`make test`, `make test-integration`) | sempre |

Testes que provam cada caso: `internal/user/concurrency_test.go` (corrida do
último admin, forçada de forma determinística), `internal/server/concurrency_test.go`
(10 pessoas salvando o mesmo fornecedor: exatamente 1 vence) e
`internal/postgres/tx_test.go` (deadlock repetido sozinho, trava demorada vira 503).

Ao criar uma tela de edição nova: devolva `version` no GET, exija no PUT e use
`ClaimPartyVersion` (pessoas) ou `WHERE version = @version` na query.

## Dinheiro, quantidade e percentual

**Nunca `float64` para valores.** Em float, `0.1 + 0.2` dá `0.30000000000000004`;
em nota fiscal isso vira rejeição na SEFAZ, e no caixa, centavo sobrando.

| Tipo Go | Guarda | Coluna no banco | JSON da API |
|---|---|---|---|
| `money.Money` | centavos (`int64`): R$ 12,34 = 1234 | `money_amount` (NUMERIC(14,2)) | `"12.34"` |
| `quantity.Quantity` | milésimos: 1,5 kg = 1500 | `quantity_amount` (NUMERIC(14,3)) | `"1.5"` |
| `money.Percent` | 4 casas: 12,5% = 125000 | `percent_rate` (NUMERIC(7,4)) | `"12.5"` |

- As colunas usam esses **domains** (migration `00003`) e o sqlc gera o tipo Go
  certo sozinho. `NUMERIC` sem domain vira `pgtype.Numeric`: sinal de que a
  coluna deveria usar um dos três.
- No JSON o valor vai como **texto** com ponto decimal, porque o JavaScript
  também usa float. O front só formata (`app/utils/decimal.ts`); contas de
  dinheiro são feitas aqui.
- Soma e subtração são exatas. Só multiplicação arredonda, sempre "meio para
  cima", num lugar só (`fixed.MulDiv`):
  `preço.Times(quantidade)` (o valor do item na NF-e), `valor.Percent(p)`.
- `valor.Allocate(n)` divide em parcelas que somam exatamente o total
  (R$ 100 em 3 = 33,34 + 33,33 + 33,33).
- Mais casas que a escala é **erro**, não arredondamento escondido
  (`"1.005"` em reais é recusado).

## Fila de jobs (River) e e-mail

Equivalente às Jobs/Queues do Laravel, com o [River](https://riverqueue.com),
que guarda a fila no próprio PostgreSQL (sem Redis).

| Laravel | Aqui |
|---|---|
| classe Job + `handle()` | `EmailArgs` (dados) + `EmailWorker.Work()` em `internal/queue` |
| `dispatch()` | `queue.EnqueueEmail(ctx, tx, msg)` — **dentro da transação** |
| `queue:work` | os workers sobem junto com a API (`WORKERS_ENABLED=true`) ou `make worker` |
| `$tries` / backoff | `MaxAttempts: 10`, espera crescente entre tentativas |
| Horizon | River UI em `/admin/queues` (só papel admin) |

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

## Fluxo de uma requisição

```
HTTP → chi (request id, log, recover, CORS, LoadSession: cookie → sessão no context)
     → Huma (grupo: Require* barra quem não pode → valida tags → Resolve())
     → handler (supplier.go) → DB.InTx / DB.InReadTx (transação) → sqlc → PostgreSQL
     ← Huma serializa a resposta (ou o erro no formato RFC 9457)
```

## Comandos

```bash
make help
```

| Comando | O que faz |
|---|---|
| `make run` | sobe a API (migrations aplicadas no start) |
| `make test` | testes unitários (não precisam de banco), com detector de corridas |
| `make test-integration` | todos os testes, incluindo integração e concorrência no banco `ceasa_test` |
| `make sqlc` | regenera `internal/db` depois de mexer em `queries/*.sql` ou migrations |
| `make migration name=create_customers` | cria um novo arquivo de migration |
| `make seed [email=...] [password=...]` | cria a empresa e cria/atualiza um admin (senha aleatória se não informar) |
| `make set-password email=... [password=...]` | redefine a senha de um usuário pelo servidor, sem e-mail |
| `make mailpit` | SMTP de desenvolvimento (e-mails em http://localhost:8025) |
| `make worker` | só os workers da fila (opcional; a API já executa os jobs) |

## Adicionando um novo recurso de negócio (ex.: clientes)

1. `make migration name=create_customers`: `CREATE TABLE customers (...)`
   (veja `suppliers` na migration `00002` como modelo).
2. Queries em `internal/db/queries/customers.sql`.
3. `make sqlc`.
4. `internal/customer/customer.go` recebendo um `postgres.DB` e acessando o banco só por `InTx`/`InReadTx`
   (ou, como `supplier`, um `Store` + `TxFunc` quando quiser testar com banco falso).
5. Registre no grupo certo em `internal/server/server.go` (`authed`, ou um
   grupo com `auth.RequireRole(...)` se só alguns papéis podem usar).
6. Testes em `internal/customer/customer_test.go` e um caso no teste de integração.

## Decisões

- **Cadastro único de pessoas** (`parties`): na CEASA o mesmo CNPJ costuma ser
  fornecedor e cliente. Os dados comuns ficam uma vez só; cada papel tem sua tabela.
- **Município pelo código IBGE** (`cities.id`): é o código que a nota fiscal exige.
  A lista vem da migration `00001`, gerada por `scripts/gen-cities-migration.py`.
- **Consignação ou compra direta** por fornecedor (padrão), com comissão em %
  obrigatória na consignação. Cada compra poderá mudar a modalidade.
- **Documento opcional**: produtor pequeno pode ser cadastrado antes de informar
  o CPF; quando informado, é único e precisa combinar com o tipo de pessoa.

- **Fornecedor nunca é apagado**: `DELETE /suppliers/{id}` só inativa (`active = false`), porque estoque e compras vão referenciá-lo.
- **CPF ou CNPJ**: produtor rural costuma ter só CPF. O CNPJ alfanumérico (julho/2026) já é aceito.
- **Documento salvo sem máscara**: a API aceita `11.222.333/0001-81` e grava `11222333000181`.
- **Busca ignora acentos** (`unaccent` do PostgreSQL): "sitio" encontra "Sítio".
- **Recuperação de senha**: link por e-mail com token de uso único (1h), só o
  hash fica no banco; ao trocar a senha, todas as sessões do usuário caem. A rota
  "esqueci a senha" responde igual exista o e-mail ou não.
- **Sem multi-tenant**: um servidor e um banco por cliente, dentro do box.
  Novo cliente = nova instalação (mesmo código, outro servidor).
