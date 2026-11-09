-- +goose Up

-- ---------------------------------------------------------------------------
-- Cadastro único de pessoas (física ou jurídica).
--
-- Na CEASA o mesmo CNPJ costuma ser fornecedor E cliente (um box compra de
-- outro e também vende para ele). Por isso os dados comuns (documento, nomes,
-- endereço, contatos, banco) ficam aqui, uma vez só, e cada papel tem sua
-- tabela: suppliers agora, customers depois.
-- ---------------------------------------------------------------------------
CREATE TABLE parties (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id BIGINT      NOT NULL REFERENCES organizations (id),

    -- Identificação
    person_type     TEXT        NOT NULL CHECK (person_type IN ('individual', 'company')),
    -- Opcional: produtor pequeno às vezes é cadastrado antes de informar o CPF.
    document        VARCHAR(14) CHECK (document ~ '^([0-9]{11}|[0-9A-Z]{12}[0-9]{2})$'),
    legal_name      TEXT        NOT NULL, -- razão social (PJ) ou nome completo (PF)
    trade_name      TEXT,                 -- nome fantasia ou apelido ("Zé do Tomate")

    -- Fiscal (opcional por enquanto; regras de nota fiscal virão depois)
    state_registration TEXT,              -- inscrição estadual / de produtor rural
    taxpayer_type   TEXT        NOT NULL DEFAULT 'non_taxpayer'
                    CHECK (taxpayer_type IN ('taxpayer', 'exempt', 'non_taxpayer')),

    -- Contato principal (outros contatos ficam em party_contacts)
    email           TEXT,
    phone           TEXT,
    whatsapp        TEXT,

    -- Endereço. A UF vem do município (cities.state_code).
    postal_code     VARCHAR(8)  CHECK (postal_code ~ '^[0-9]{8}$'),
    street          TEXT,
    number          TEXT,
    complement      TEXT,
    district        TEXT,
    city_id         INTEGER     REFERENCES cities (id),

    -- Financeiro
    pix_key_type    TEXT        CHECK (pix_key_type IN ('cpf', 'cnpj', 'email', 'phone', 'random')),
    pix_key         TEXT,
    bank            TEXT,
    bank_branch     TEXT,
    bank_account    TEXT,
    bank_account_type TEXT      CHECK (bank_account_type IN ('checking', 'savings', 'payment')),
    bank_account_holder TEXT,                 -- titular, quando não é a própria pessoa
    bank_account_holder_document VARCHAR(14),

    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- CPF (11) só para pessoa física; CNPJ (14) só para jurídica.
    CHECK (document IS NULL OR (person_type = 'individual') = (length(document) = 11)),
    -- Chave PIX: tipo e chave andam juntos.
    CHECK ((pix_key_type IS NULL) = (pix_key IS NULL)),
    -- Permite que outras tabelas exijam "esta pessoa E desta organização"
    -- numa chave estrangeira composta (ver party_contacts e suppliers).
    UNIQUE (id, organization_id)
);
-- Documento único por organização, mas só quando informado (índice parcial).
CREATE UNIQUE INDEX parties_organization_document_key
    ON parties (organization_id, document) WHERE document IS NOT NULL;
CREATE INDEX parties_organization_name_idx ON parties (organization_id, lower(legal_name));

-- Contatos adicionais: dono, gerente, motorista...
CREATE TABLE party_contacts (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id BIGINT  NOT NULL,
    party_id        BIGINT  NOT NULL,
    name            TEXT    NOT NULL,
    role            TEXT,
    phone           TEXT,
    is_whatsapp     BOOLEAN NOT NULL DEFAULT FALSE,
    email           TEXT,
    position        INTEGER NOT NULL DEFAULT 0, -- ordem de exibição
    -- A chave composta garante que o contato é da mesma organização da pessoa.
    FOREIGN KEY (party_id, organization_id) REFERENCES parties (id, organization_id) ON DELETE CASCADE
);
CREATE INDEX party_contacts_party_idx ON party_contacts (party_id);

-- ---------------------------------------------------------------------------
-- Fornecedor: o papel "fornecedor" de uma pessoa. A chave é a própria pessoa
-- (1 pessoa = no máximo 1 cadastro de fornecedor).
-- ---------------------------------------------------------------------------

-- Guarda os fornecedores atuais para migrar os dados para o modelo novo.
CREATE TEMPORARY TABLE legacy_suppliers ON COMMIT DROP AS SELECT * FROM suppliers;
DROP TABLE suppliers;

CREATE TABLE suppliers (
    party_id        BIGINT      PRIMARY KEY,
    organization_id BIGINT      NOT NULL,
    supplier_type   TEXT        NOT NULL DEFAULT 'producer'
                    CHECK (supplier_type IN ('producer', 'wholesaler', 'cooperative', 'importer', 'other')),
    -- Compra direta (paga pela mercadoria) ou consignação (vende e paga depois,
    -- descontando a comissão). É o padrão do fornecedor; cada compra pode mudar.
    default_purchase_mode TEXT  NOT NULL DEFAULT 'direct'
                    CHECK (default_purchase_mode IN ('direct', 'consignment')),
    consignment_commission NUMERIC(5, 2) CHECK (consignment_commission BETWEEN 0 AND 100), -- em %
    payment_term_days INTEGER   NOT NULL DEFAULT 0 CHECK (payment_term_days BETWEEN 0 AND 365), -- 0 = à vista
    -- Onde o fornecedor fica na CEASA, quando ele também tem box.
    ceasa_pavilion  TEXT,
    ceasa_box       TEXT,
    status          TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'blocked')),
    blocked_reason  TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (party_id, organization_id) REFERENCES parties (id, organization_id) ON DELETE CASCADE,
    CHECK (default_purchase_mode <> 'consignment' OR consignment_commission IS NOT NULL),
    CHECK ((status = 'blocked') = (blocked_reason IS NOT NULL))
);

-- Migra os fornecedores antigos: cada um vira uma pessoa + um fornecedor.
-- A cidade era texto livre; tentamos achar o município pelo nome sem acento + UF.
INSERT INTO parties (organization_id, person_type, document, legal_name, email, phone,
                     city_id, notes, created_at, updated_at)
SELECT l.organization_id,
       CASE WHEN length(l.document) = 11 THEN 'individual' ELSE 'company' END,
       l.document, l.name, l.email, l.phone,
       (SELECT c.id FROM cities c
         WHERE c.state_code = l.state
           AND lower(unaccent(c.name)) = lower(unaccent(l.city))
         LIMIT 1),
       -- Se a cidade não foi reconhecida, ela vai para as observações.
       CASE
           WHEN l.city IS NOT NULL AND NOT EXISTS (
               SELECT 1 FROM cities c
                WHERE c.state_code = l.state
                  AND lower(unaccent(c.name)) = lower(unaccent(l.city)))
           THEN concat_ws(E'\n', l.notes, 'Cidade antiga: ' || l.city || coalesce('/' || l.state, ''))
           ELSE l.notes
       END,
       l.created_at, l.updated_at
FROM legacy_suppliers l;

INSERT INTO suppliers (party_id, organization_id, status, created_at, updated_at)
SELECT p.id, p.organization_id,
       CASE WHEN l.active THEN 'active' ELSE 'inactive' END,
       l.created_at, l.updated_at
FROM legacy_suppliers l
JOIN parties p ON p.organization_id = l.organization_id AND p.document = l.document;

-- ---------------------------------------------------------------------------
-- RLS nas três tabelas, igual à migration 00002.
-- ---------------------------------------------------------------------------
GRANT SELECT, INSERT, UPDATE, DELETE ON parties, party_contacts, suppliers TO ceasa_app;
GRANT USAGE ON SEQUENCE parties_id_seq, party_contacts_id_seq TO ceasa_app;

ALTER TABLE parties ENABLE ROW LEVEL SECURITY;
ALTER TABLE party_contacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE suppliers ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON parties
    USING (organization_id = NULLIF(current_setting('app.organization_id', true), '')::BIGINT)
    WITH CHECK (organization_id = NULLIF(current_setting('app.organization_id', true), '')::BIGINT);
CREATE POLICY tenant_isolation ON party_contacts
    USING (organization_id = NULLIF(current_setting('app.organization_id', true), '')::BIGINT)
    WITH CHECK (organization_id = NULLIF(current_setting('app.organization_id', true), '')::BIGINT);
CREATE POLICY tenant_isolation ON suppliers
    USING (organization_id = NULLIF(current_setting('app.organization_id', true), '')::BIGINT)
    WITH CHECK (organization_id = NULLIF(current_setting('app.organization_id', true), '')::BIGINT);

-- +goose Down
-- Volta ao fornecedor simples (perde os campos novos).
CREATE TEMPORARY TABLE new_suppliers ON COMMIT DROP AS
SELECT p.organization_id, p.legal_name AS name, p.document, p.phone, p.email,
       c.name AS city, c.state_code AS state, p.notes,
       s.status <> 'inactive' AS active, s.created_at, s.updated_at
FROM suppliers s
JOIN parties p ON p.id = s.party_id
LEFT JOIN cities c ON c.id = p.city_id
WHERE p.document IS NOT NULL;

DROP TABLE suppliers;
DROP TABLE party_contacts;
DROP TABLE parties;

CREATE TABLE suppliers (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name            TEXT        NOT NULL,
    document        VARCHAR(14) NOT NULL CHECK (document ~ '^([0-9]{11}|[0-9A-Z]{12}[0-9]{2})$'),
    phone           TEXT,
    email           TEXT,
    city            TEXT,
    state           TEXT CHECK (state ~ '^[A-Z]{2}$'),
    notes           TEXT,
    active          BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    organization_id BIGINT      NOT NULL REFERENCES organizations (id),
    CONSTRAINT suppliers_organization_document_key UNIQUE (organization_id, document)
);
CREATE INDEX suppliers_organization_name_idx ON suppliers (organization_id, lower(name));

INSERT INTO suppliers (organization_id, name, document, phone, email, city, state, notes, active, created_at, updated_at)
SELECT organization_id, name, document, phone, email, city, state, notes, active, created_at, updated_at
FROM new_suppliers;

GRANT SELECT, INSERT, UPDATE, DELETE ON suppliers TO ceasa_app;
GRANT USAGE ON SEQUENCE suppliers_id_seq TO ceasa_app;
ALTER TABLE suppliers ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON suppliers
    USING (organization_id = NULLIF(current_setting('app.organization_id', true), '')::BIGINT)
    WITH CHECK (organization_id = NULLIF(current_setting('app.organization_id', true), '')::BIGINT);
