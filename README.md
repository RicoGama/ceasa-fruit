# CEASA Fruit

SaaS de controle de vendas de LFV (Legumes, Frutas e Verduras) para
distribuidoras e atacadistas.

## Escopo

0. Multi-tenant: organizações, usuários em várias organizações, login, admin da plataforma ✅ (API + telas)
1. Controle de vendas de LFV
2. Cadastro de fornecedores ✅ (API + tela)
3. Cadastro de clientes
4. Controle de estoque com aviso de perda
5. Venda
6. Rota de venda
7. (Futuro) Caminhões de entrega nas rotas via mapa

## Stack

| Camada | Tecnologia |
|---|---|
| API | Go + [chi](https://github.com/go-chi/chi) (roteador) + [Huma](https://huma.rocks) (OpenAPI, validação) |
| Banco | PostgreSQL + [sqlc](https://sqlc.dev) (SQL → Go tipado) + [goose](https://github.com/pressly/goose) (migrations) |
| Front | [Nuxt 4](https://nuxt.com) + [Nuxt UI 4](https://ui.nuxt.com) (Tailwind CSS 4) + cliente tipado gerado do OpenAPI |

Convenção: **código em inglês** (identificadores, banco, rotas, JSON); **comentários e documentação em português**.

## Estrutura

```
ceasa-fruit/
├── api/                # API em Go (ver api/README.md)
├── web/                # front em Nuxt (ver web/README.md)
├── docker/             # inicialização do Postgres no Docker
├── scripts/            # db-setup-local.sql (Postgres sem Docker)
├── Caddyfile           # vhosts locais: ceasa-fruit.test:6240 e api.ceasa-fruit.test:6240
└── docker-compose.yml  # PostgreSQL de desenvolvimento (+ banco ceasa_test)
```

## Rodando

Pré-requisitos: Go 1.26+ e PostgreSQL 14+ (instalado na máquina **ou** via Docker).

```bash
cd api
```

```bash
make tools   # instala sqlc e goose (uma vez)
```

### Banco: Postgres instalado na máquina

Cria o usuário `ceasa` (senha `ceasa`) e os bancos `ceasa` e `ceasa_test`. Pode rodar de novo sem problema.

```bash
make db-setup-local
```

A busca sem acento usa a extensão `unaccent`, que vem no pacote *contrib* do
Postgres. No Ubuntu/Debian ela já vem junto; no Fedora instale `postgresql-contrib`.

### Banco: via Docker (alternativa)

```bash
make db-up
```

### Subindo a API

```bash
make run     # aplica migrations e sobe a API em :8080
```

Se o seu Postgres não estiver em `localhost:5432`, copie `.env.example` para `.env` e ajuste `DATABASE_URL`.

### Subindo o front (em outro terminal)

Pré-requisito: Node.js 22+.

```bash
cd web && npm install && npm run dev
```

Abra http://localhost:3000, crie uma conta e use o sistema.

### URL local com vhost (opcional): http://ceasa-fruit.test:6240

Em vez de `localhost:3000`, dá para usar `http://ceasa-fruit.test:6240` (front) e
`http://api.ceasa-fruit.test:6240/docs` (API). Um proxy, o [Caddy](https://caddyserver.com),
atende na porta 6240 e repassa para o Nuxt e para o Go (veja o `Caddyfile` na raiz).
A porta 6240 convive com um Nginx/Apache na porta 80 e não precisa de `sudo`.

**1. Apontar os nomes para a sua máquina** (uma vez):

```bash
echo "127.0.0.1 ceasa-fruit.test api.ceasa-fruit.test" | sudo tee -a /etc/hosts
```

**2. Instalar o Caddy** (uma vez). No Ubuntu/Debian:

```bash
sudo apt install caddy
```

O pacote do Ubuntu sobe um Caddy como serviço do sistema. Não precisamos dele
(usamos o do projeto), então pode desligar:

```bash
sudo systemctl disable --now caddy
```

**3. Rodar** (na raiz do projeto, com API e front já rodando em outros terminais):

```bash
caddy run
```

Abra http://ceasa-fruit.test:6240. O hot reload do Nuxt funciona pelo vhost também.

> Por que `.test` e não `.local`? O `.local` é reservado para descoberta de
> dispositivos na rede (mDNS) e pode demorar ou falhar para resolver. O `.test`
> é reservado para testes e nunca vai existir na internet.

### Endereços da API

- http://localhost:8080/docs — documentação interativa (dá para testar os endpoints)
- http://localhost:8080/health — health check
- http://localhost:8080/openapi.json — contrato OpenAPI (vai gerar o cliente do Nuxt)
