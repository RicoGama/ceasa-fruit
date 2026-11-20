-- Cadastro de produtos (legumes, frutas, verduras e ovos).
--
-- Um produto é o item que se compra, estoca e vende: "Tomate Italiano Extra AA".
-- Ele tem:
--   - identificação: nome + variedade + classificação (cada combinação é um
--     produto, porque preço e cliente mudam), categoria e um código curto opcional;
--   - unidade de estoque: o que se CONTA no box (caixa, saco, maço, bandeja,
--     unidade, dúzia, kg). Sem balança, a venda a quilo usa o peso médio:
--     o padrão fica aqui (nominal_weight_kg) e cada lote poderá ter o seu;
--   - embalagens maiores (pilha, pallet, caixa de 30): quantas unidades de
--     estoque cabem em cada uma; vazio = varia, informada na hora;
--   - dados fiscais para a NF-e: NCM e origem da mercadoria.

-- +goose Up
CREATE TABLE products (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code              TEXT,             -- código curto opcional, ex.: "TOM-ITA"
    name              TEXT NOT NULL,    -- Tomate
    variety           TEXT,             -- Italiano
    grade             TEXT,             -- Extra AA (classificação)
    category          TEXT NOT NULL CHECK (category IN ('fruit', 'vegetable', 'leafy', 'egg', 'other')),
    stock_unit        TEXT NOT NULL CHECK (stock_unit IN ('box', 'kg', 'unit', 'dozen', 'bag', 'bunch', 'tray', 'pack')),
    -- Peso médio (kg) de UMA unidade de estoque (ex.: caixa de tomate ≈ 18 kg).
    nominal_weight_kg quantity_amount CHECK (nominal_weight_kg > 0),
    sells_by_weight   BOOLEAN NOT NULL DEFAULT false, -- pode vender a quilo
    ncm               TEXT CHECK (ncm ~ '^[0-9]{8}$'),
    -- Origem da mercadoria na NF-e: 0 = nacional, 1 = importação direta,
    -- 2 = importado comprado no mercado interno... (tabela oficial 0 a 8).
    origin            SMALLINT NOT NULL DEFAULT 0 CHECK (origin BETWEEN 0 AND 8),
    active            BOOLEAN NOT NULL DEFAULT true,
    notes             TEXT,
    version           INTEGER NOT NULL DEFAULT 1,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Vender a quilo sem saber o peso médio não faz sentido (exceto se a
    -- própria unidade de estoque já for o kg).
    CHECK (NOT sells_by_weight OR stock_unit = 'kg' OR nominal_weight_kg IS NOT NULL)
);

-- O mesmo nome + variedade + classificação não pode existir duas vezes
-- (sem diferenciar maiúsculas). coalesce: NULL não conta como "diferente".
CREATE UNIQUE INDEX products_identity_key ON products (
    lower(name), lower(coalesce(variety, '')), lower(coalesce(grade, ''))
);
CREATE UNIQUE INDEX products_code_key ON products (upper(code)) WHERE code IS NOT NULL;

CREATE TABLE product_packagings (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,          -- Pilha, Pallet, Caixa de 30
    -- Quantas unidades de estoque cabem nela; NULL = varia (informada na venda).
    quantity   quantity_amount CHECK (quantity > 0),
    position   SMALLINT NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX product_packagings_name_key ON product_packagings (product_id, lower(name));

-- +goose Down
DROP TABLE product_packagings;
DROP TABLE products;
