-- +goose Up
-- unaccent lets searches ignore accents ("sitio" finds "Sítio").
CREATE EXTENSION IF NOT EXISTS unaccent;

CREATE TABLE suppliers (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL,
    -- Brazilian tax id without mask: CPF (11 digits, e.g. a rural producer) or
    -- CNPJ (14 chars; since July 2026 new CNPJs may contain letters in the first 12).
    document   VARCHAR(14) NOT NULL UNIQUE
               CHECK (document ~ '^([0-9]{11}|[0-9A-Z]{12}[0-9]{2})$'),
    phone      TEXT,
    email      TEXT,
    city       TEXT,
    state      TEXT CHECK (state ~ '^[A-Z]{2}$'),
    notes      TEXT,
    -- Suppliers are never hard-deleted: purchase/stock history depends on them.
    active     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX suppliers_name_idx ON suppliers (lower(name));

-- +goose Down
DROP TABLE suppliers;
