-- +goose Up
-- unaccent permite buscas que ignoram acentos ("sitio" encontra "Sítio").
CREATE EXTENSION IF NOT EXISTS unaccent;

CREATE TABLE suppliers (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL,
    -- Documento sem máscara: CPF (11 dígitos, ex.: produtor rural) ou CNPJ
    -- (14 caracteres; desde jul/2026 novos CNPJs podem ter letras nas 12 primeiras posições).
    document   VARCHAR(14) NOT NULL UNIQUE
               CHECK (document ~ '^([0-9]{11}|[0-9A-Z]{12}[0-9]{2})$'),
    phone      TEXT,
    email      TEXT,
    city       TEXT,
    state      TEXT CHECK (state ~ '^[A-Z]{2}$'),
    notes      TEXT,
    -- Fornecedor nunca é apagado de verdade: o histórico de compras/estoque depende dele.
    active     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX suppliers_name_idx ON suppliers (lower(name));

-- +goose Down
DROP TABLE suppliers;
