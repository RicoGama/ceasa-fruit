# SEASA Fruit

Sistema de controle de vendas de LFV (Legumes, Frutas e Verduras).

## Escopo

1. Controle de vendas de LFV
2. Cadastro de fornecedores ✅ (API)
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
| Front | Nuxt 4 (SPA + PWA) + Nuxt UI / Tailwind — *a fazer* |

Convenção: **todo o código (identificadores, banco, rotas, JSON, comentários) em inglês**; documentação de estudo em português.

## Estrutura

```
seasa-fruit/
├── api/                # API em Go (ver api/README.md)
├── web/                # (a fazer) Nuxt
└── docker-compose.yml  # PostgreSQL de desenvolvimento
```

## Rodando

Pré-requisitos: Go 1.26+, Docker.

```bash
cd api
```

```bash
make tools   # instala sqlc e goose (uma vez)
```

```bash
make db-up   # sobe o PostgreSQL
```

```bash
make run     # aplica migrations e sobe a API em :8080
```

Depois abra:

- http://localhost:8080/docs — documentação interativa (dá para testar os endpoints)
- http://localhost:8080/health — health check
- http://localhost:8080/openapi.json — contrato OpenAPI (vai gerar o cliente do Nuxt)
