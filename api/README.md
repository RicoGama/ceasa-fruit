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
| 15b | `internal/product/product.go` | cadastro com lista filha (embalagens): atualizar, criar e remover numa transação; papéis na rota |
| 15a | `internal/customer/customer.go` + `internal/party/save.go` | o mesmo desenho do fornecedor; um papel novo em cima de uma pessoa que já existe (`party.Existing`) |
| 15e | `internal/audit/audit.go` | histórico de alterações: gatilho no banco + context com o usuário + agrupamento por transação |
| 15d | `internal/fleet/` | frota: cliente HTTP de outro sistema (Traccar), goroutine de sincronização com o banco, generics (`deref[T]`) |
| 15c | `internal/supplierproduct/supplierproduct.go` | relação muitos-para-muitos com dados na ligação, rotas aninhadas (`/suppliers/{id}/products`), permissão dupla (`Protect` duas vezes = E) |
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
| Papel | `roles`, `role_permissions` | conjunto de permissões, editável pela tela |
| Sessão | `sessions` | login ativo (só o hash do token) |
| Pessoa (física/jurídica) | `parties` | dados comuns: documento, nomes, endereço, PIX/banco |
| Contatos da pessoa | `party_contacts` | dono, gerente, motorista... |
| Fornecedor | `suppliers` | papel "fornecedor" de uma pessoa (consignação, prazo, box) |
| Produto | `products` | nome + variedade + classificação, unidade de estoque, peso médio, NCM |
| Embalagem | `product_packagings` | pilha, pallet, caixa de 30: quantas unidades cabem (vazio = varia) |
| Fornecedor × produto | `supplier_products` | o que cada fornecedor traz: nome na nota dele, safra (meses), modalidade própria, preferencial |
| Cliente | `customers` | papel "cliente" de uma pessoa (tipo, box na CEASA, situação) |
| Motorista | `drivers` | nome, CPF, CNH (categoria e validade) |
| Veículo | `vehicles` | placa, tipo, motorista habitual, IMEI do rastreador e a última posição |
| Posição | `vehicle_positions` | o que o rastreador mandou (via Traccar): hora do GPS, lugar, velocidade, ignição |
| Sincronização | `fleet_sync` | situação da conexão com o Traccar (o mapa avisa quando cai) |
| Municípios/UFs | `cities`, `states` | lista oficial do IBGE |

### Papéis e permissões

- **Permissões ficam no código** (`internal/auth/permissions.go`): uma permissão
  só tem efeito se alguma rota a conferir. O catálogo hoje:

  | Permissão | O que libera |
  |---|---|
  | `suppliers.view` / `suppliers.edit` | ver / cadastrar, editar e inativar fornecedores |
  | `customers.view` / `customers.edit` | ver / cadastrar, editar e inativar clientes |
  | `products.view` / `products.edit` | ver / cadastrar, editar e inativar produtos |
  | `fleet.view` / `fleet.edit` | ver o mapa da frota / cadastrar veículos e motoristas |
  | `users.manage` | usuários, papéis e permissões |
  | `company.edit` | alterar os dados da empresa |
  | `queues.view` | painel da fila de tarefas (River UI) |
  | `audit.view` | registro de acessos (nenhum papel tem por padrão: só o admin) |

- **Papéis ficam no banco** (`roles`, `role_permissions`) e o administrador cria
  e edita pela tela *Administração → Papéis e permissões*. Cada usuário tem
  **um** papel (`users.role_id`). Os seis papéis iniciais (Administrador,
  Gerente, Vendedor, Conferente, Financeiro, Motorista) vêm da migration `00006`.
- O **Administrador** (`key = 'admin'`) é do sistema: tem todas as permissões,
  não pode ser alterado nem excluído, e o sistema não deixa ficar sem nenhum ativo.
- Permissões são lidas **junto com a sessão, a cada requisição**: mudar um papel
  vale na hora para todos os usuários dele.
- `edit` implica `view` (quem edita fornecedores também os vê): `auth.Implies`.

Na rota:

```go
huma.Register(api, auth.Protect(api, huma.Operation{...}, auth.ProductsEdit), h.update)
```

`Protect` barra quem não tem (403) e anota `x-permissions` no OpenAPI. O teste
`TestEveryRouteHasPermission` reprova qualquer rota que exige login sem
declarar permissão (fora as poucas liberadas para todo usuário logado:
`/auth/me`, `/cities`, `GET /company`).

**Permissão nova:** constante + `All` em `permissions.go`, `auth.Protect` na
rota, e o texto em português em `web/app/utils/permissions.ts` (o TypeScript
não compila enquanto faltar).

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

## Registro de acessos

Tabela `access_log` (migration `00009`): uma linha por sessão e uma por
tentativa de login que falhou. Tela: *Administração → Acessos* (permissão
`audit.view`).

- **Entrou**: hora, e-mail, IP real (ver "IP do cliente") e navegador.
- **Última atividade**: gravada no máximo 1x por minuto por sessão, a cada
  requisição e enquanto a tela está aberta (pelo canal de tempo real). Daí sai
  "Online agora" (atividade há menos de 3 min) e "Sem atividade desde...".
- **Saiu**: `logout`, `password_changed` ou `deactivated`. As sessões só
  terminam por `EndSession` / `EndUserSessions`, que exigem o motivo: o
  compilador não deixa encerrar sessão sem registrar.
- **Falhas**: `wrong_password`, `unknown_email`, `inactive_user`
  (e `too_many_attempts` durante o bloqueio).
- **Limite de tentativas** (`internal/auth/ratelimit.go`): 5 falhas em 15 min
  bloqueiam o e-mail (mesmo inexistente; um login certo zera) e 20 falhas em
  15 min bloqueiam o IP. Resposta 429 com `Retry-After` e os minutos restantes.
  Tentativas durante o bloqueio são registradas (`too_many_attempts`) mas não
  renovam o bloqueio. Rajadas não furam o limite: as tentativas do mesmo e-mail
  passam por uma trava no banco (`pg_advisory_xact_lock`), e a falha é gravada
  na mesma transação da contagem (teste: 30 tentativas simultâneas, 5 contam).
- Quem só fecha o navegador não tem hora de saída (não há como saber): a tela
  mostra a última atividade, ou "Sessão expirou" quando a sessão venceu.

## Tempo real

Tudo o que alguém grava aparece na hora para quem está com a tela aberta.

```
gravação -> gatilho no Postgres -> NOTIFY app_events (só depois do COMMIT)
  -> internal/realtime (uma conexão com LISTEN) -> GET /events (Server-Sent Events)
  -> o front recarrega a lista ou o cadastro aberto
```

- **Os avisos vêm de gatilhos no banco** (migration `00007`), não do código Go:
  nenhuma gravação escapa (tela, seed, script), um ROLLBACK não avisa nada, e
  avisos iguais na mesma transação viram um só.
- **Tudo é tempo real: criar, alterar, inativar e excluir.** A inativação
  (exclusão lógica) é um UPDATE, e a exclusão de verdade tem gatilho de DELETE.
  O teste `TestEveryTableNotifiesChanges` (`internal/postgres`) reprova tabela
  nova sem gatilho; exceções ficam listadas no próprio teste, com o motivo.
- Na tela: a lista recarrega sozinha; o cadastro aberto recarrega sozinho se
  você não mexeu nele, avisa "alterado por outra pessoa" se você tem algo não
  salvo, e avisa "excluído" se ele sumiu. Inativar também muda a versão, então
  ninguém reativa sem perceber ao salvar uma tela antiga.
- O aviso é só `{"topic": "suppliers", "id": 42}`; os dados o front busca pela
  API, com as permissões de sempre. Mesmo assim, cada pessoa só recebe os
  assuntos que pode ver (`topicPermission` em `realtime.go`).
- Mudança no próprio usuário ou no papel dele chega como `"me"`: o front relê
  as permissões (menus e botões mudam na hora).
- Se a API perder a conexão com o banco, ao voltar manda `"resync"` e as telas
  recarregam tudo.

**Tabela nova que deve aparecer em tempo real:** um `CREATE TRIGGER ... EXECUTE
FUNCTION notify_change('<assunto>', '<coluna do id>')` numa migration (tabela
filha avisa com o id do pai), a permissão do assunto em `topicPermission`, e
`useLiveRefresh` / `useLiveRecord` na tela.

## IP do cliente

O IP de quem usa o sistema vai para o log (`client_ip`) e servirá para
auditoria. Ele vem do cabeçalho `X-Forwarded-For`, que **só é aceito quando a
conexão chega de um proxy confiável** (`TRUSTED_PROXIES`, padrão: a própria
máquina, onde roda o Nuxt). Vindo de qualquer outro lugar, o cabeçalho é
ignorado e vale o IP da conexão (`middleware.ClientIPFromXFF` do chi; o antigo
`middleware.RealIP` confiava em qualquer um e foi descontinuado por isso).

Do outro lado, o Nuxt nunca repassa o cabeçalho que o navegador mandou: ele o
troca pelo IP real da conexão (`web/server/utils/api-proxy.ts`). No
`npm run dev` isso não é garantido (o servidor de desenvolvimento tem um
salto interno); em produção é.

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
5. Crie as permissões do módulo (`internal/auth/permissions.go`), proteja cada
   rota com `auth.Protect` e registre no grupo `authed` em `internal/server/server.go`.
6. Testes em `internal/customer/customer_test.go` e um caso no teste de integração.

## Decisões

- **Cadastro único de pessoas** (`parties`): na CEASA o mesmo CNPJ costuma ser
  fornecedor e cliente. Os dados comuns ficam uma vez só; cada papel tem sua tabela.
  - Ao cadastrar, a tela consulta `GET /parties/by-document/{doc}`; se o CPF/CNPJ
    já existe, oferece "usar este cadastro" e manda `partyId` + `partyVersion`
    no `POST /customers` (ou `/suppliers`). A versão evita sobrescrever quem
    editou a pessoa no meio tempo (409).
  - Editar nome, endereço ou contatos pelo cliente muda o fornecedor também (é
    a mesma linha). O gatilho `notify_party_change` (migration `00011`) avisa as
    duas telas em tempo real. Inativar o cliente **não** inativa o fornecedor.
  - Quem pode cadastrar fornecedor **ou** cliente pode consultar o documento, e
    vê os dados da pessoa (inclusive banco/PIX): é o mesmo cadastro.
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

## Frota (rastreamento)

```text
caminhão (FMC130) -> Traccar na nuvem -> fleet.Syncer (a cada 15 s) -> vehicle_positions -> tempo real -> mapa
```

- **Por que o Traccar fica na nuvem**: o rastreador precisa de um endereço
  fixo no ar 24 h; o box não tem. Como subir: `deploy/traccar/LEIAME.md`.
- **Configuração**: `TRACCAR_URL`, `TRACCAR_TOKEN` (sem eles, frota só como
  cadastro), `FLEET_SYNC_SECONDS` (15), `FLEET_RETENTION_DAYS` (180).
- **O sincronizador (`internal/fleet/sync.go`)**:
  - cadastra no Traccar o aparelho de cada veículo com IMEI (nome = placa);
  - só pede posições de quem tem novidade (`lastUpdate` do aparelho);
  - busca **a partir da última posição gravada**: posições guardadas sem sinal
    chegam depois, com o horário antigo; com a internet do box fora, o
    Traccar acumula e a primeira rodada boa traz tudo;
  - continua conferindo por 30 s depois de cada novidade, porque o Traccar
    marca o aparelho como atualizado um instante antes de gravar a posição;
  - posição repetida não duplica (`UNIQUE (vehicle_id, traccar_position_id)`).
- **Testes**: `internal/server/fleet_test.go` usa um Traccar falso (posições
  atrasadas, internet caindo, token errado). O teste de ponta a ponta usou um
  Traccar 6.17 de verdade com o rastreador simulado pelo protocolo OsmAnd.

## Histórico de alterações

"Quem mudou o preço, quando, de quanto para quanto", em todo cadastro.

- **Gatilho `audit_row`** (migration `00013`) em cada tabela de cadastro grava
  em `audit_log`, na mesma transação, só os campos que mudaram:
  `{"phone": ["21999990000", "21988887777"]}`. Salvar sem mudar nada não grava.
  Senha, `version`, `updated_at` e a última posição dos veículos ficam de fora.
- **Quem fez**: o middleware de sessão põe o usuário no `context`
  (`postgres.WithActor`) e toda transação de escrita faz
  `set_config('app.actor_id', ...)`. Sem usuário (sincronização da frota, seed,
  script) aparece como "Sistema".
- **Um "salvar" = um evento**: tudo o que a transação gravou (pessoa +
  fornecedor + contatos) tem o mesmo `tx_id` e a API junta.
- **Contatos e permissões são salvos pela diferença** (`party.SaveContacts`,
  `role.savePermissions`): o histórico mostra "trocou o telefone do Seu
  Jorge", e não "apagou e criou todos".
- **Rotas**: `GET /audit/{tipo}/{id}` (quem vê o cadastro vê o histórico) e
  `GET /audit` (geral, `audit.view`). Ids de outros cadastros (produto,
  papel, município) vêm com o nome em `refs`.
- **Regra**: `TestEveryTableIsAudited` reprova tabela nova sem o gatilho;
  exceções listadas no teste com o motivo.

## Consulta de CNPJ (Receita)

`GET /cnpj/{cnpj}` (`internal/cnpj`) devolve razão social, nome fantasia,
situação, endereço com o município do IBGE, telefone e e-mail públicos.

- **Quem consulta é a API do box**, não o navegador: o resultado fica 24 h em
  memória (os serviços gratuitos limitam consultas) e, sem internet, a tela
  recebe 503 e pede o preenchimento à mão.
- **Dois serviços, um de reserva do outro**: BrasilAPI e CNPJá (aberta). 404
  = CNPJ não existe; erro de rede, limite (429) ou 5xx = tenta o próximo.
  Endereços trocáveis por `CNPJ_BRASILAPI_URL` e `CNPJ_CNPJA_URL`.
- **Município**: pelo código do IBGE quando vem; senão pelo nome + UF, sem acento.
- **A tela nunca preenche sozinha**: mostra o que achou (com aviso se o CNPJ
  estiver BAIXADO/INAPTO) e a pessoa clica em "Preencher". Telefone e e-mail
  já digitados não são trocados.
- **Testes** (`internal/server/cnpj_test.go`) usam servidores falsos no formato
  documentado de cada serviço; os serviços de verdade não estavam acessíveis
  do ambiente de desenvolvimento.
