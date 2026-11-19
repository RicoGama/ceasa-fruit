-- +goose Up

-- Busca sem acento ("sitio" encontra "Sítio").
CREATE EXTENSION IF NOT EXISTS unaccent;

-- ---------------------------------------------------------------------------
-- Empresa desta instalação. Cada cliente tem o próprio servidor e o próprio
-- banco, então existe UMA empresa só: o CHECK (id = 1) garante isso.
-- ---------------------------------------------------------------------------
CREATE TABLE company (
    id                 SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    name               TEXT        NOT NULL,           -- razão social
    trade_name         TEXT,                           -- nome fantasia
    document           VARCHAR(14) CHECK (document ~ '^[0-9A-Z]{12}[0-9]{2}$'), -- CNPJ
    state_registration TEXT,                           -- inscrição estadual
    tax_regime         TEXT        CHECK (tax_regime IN ('simples', 'presumido', 'real')),
    phone              TEXT,
    email              TEXT,
    postal_code        VARCHAR(8)  CHECK (postal_code ~ '^[0-9]{8}$'),
    street             TEXT,
    number             TEXT,
    complement         TEXT,
    district           TEXT,
    city_id            INTEGER     REFERENCES cities (id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Usuários e acesso
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          TEXT        NOT NULL,
    -- Sempre gravado em minúsculas, para o UNIQUE funcionar sem depender de maiúsculas.
    email         TEXT        NOT NULL UNIQUE CHECK (email = lower(email)),
    password_hash TEXT        NOT NULL,
    -- Papel do usuário. As permissões de cada papel ficam no código (pacote auth).
    --   admin: tudo, inclusive usuários e configurações
    --   manager: gerente; seller: vendedor; checker: conferente (entrada/saída);
    --   finance: financeiro; driver: motorista
    role          TEXT        NOT NULL
                  CHECK (role IN ('admin', 'manager', 'seller', 'checker', 'finance', 'driver')),
    active        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sessões de login. O navegador guarda o token num cookie; aqui guardamos só o
-- hash SHA-256 dele, então um vazamento do banco não entrega sessões válidas.
CREATE TABLE sessions (
    token_hash BYTEA       PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

-- Links de redefinição de senha (e de convite para criar a senha). Também só o hash.
CREATE TABLE password_reset_tokens (
    token_hash BYTEA       PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,             -- preenchido quando o link é usado (uso único)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX password_reset_tokens_user_idx ON password_reset_tokens (user_id);

-- ---------------------------------------------------------------------------
-- Cadastro único de pessoas (física ou jurídica).
--
-- Na CEASA o mesmo CNPJ costuma ser fornecedor E cliente (um box compra de
-- outro e também vende para ele). Os dados comuns ficam aqui, uma vez só, e
-- cada papel tem sua tabela: suppliers agora, customers depois.
-- ---------------------------------------------------------------------------
CREATE TABLE parties (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    -- Identificação
    person_type     TEXT        NOT NULL CHECK (person_type IN ('individual', 'company')),
    -- Opcional: produtor pequeno às vezes é cadastrado antes de informar o CPF.
    document        VARCHAR(14) CHECK (document ~ '^([0-9]{11}|[0-9A-Z]{12}[0-9]{2})$'),
    legal_name      TEXT        NOT NULL, -- razão social (PJ) ou nome completo (PF)
    trade_name      TEXT,                 -- nome fantasia ou apelido ("Zé do Tomate")

    -- Fiscal (opcional por enquanto)
    state_registration TEXT,
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
    bank_account_holder TEXT,
    bank_account_holder_document VARCHAR(14),

    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- CPF (11) só para pessoa física; CNPJ (14) só para jurídica.
    CHECK (document IS NULL OR (person_type = 'individual') = (length(document) = 11)),
    -- Chave PIX: tipo e chave andam juntos.
    CHECK ((pix_key_type IS NULL) = (pix_key IS NULL))
);
-- Documento único, mas só quando informado (índice parcial).
CREATE UNIQUE INDEX parties_document_key ON parties (document) WHERE document IS NOT NULL;
CREATE INDEX parties_legal_name_idx ON parties (lower(legal_name));

-- Contatos adicionais: dono, gerente, motorista...
CREATE TABLE party_contacts (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    party_id    BIGINT  NOT NULL REFERENCES parties (id) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    role        TEXT,
    phone       TEXT,
    is_whatsapp BOOLEAN NOT NULL DEFAULT FALSE,
    email       TEXT,
    position    INTEGER NOT NULL DEFAULT 0 -- ordem de exibição
);
CREATE INDEX party_contacts_party_idx ON party_contacts (party_id);

-- Fornecedor: o papel "fornecedor" de uma pessoa (1 pessoa = no máximo 1 fornecedor).
CREATE TABLE suppliers (
    party_id        BIGINT      PRIMARY KEY REFERENCES parties (id) ON DELETE CASCADE,
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
    CHECK (default_purchase_mode <> 'consignment' OR consignment_commission IS NOT NULL),
    CHECK ((status = 'blocked') = (blocked_reason IS NOT NULL))
);

-- +goose Down
DROP TABLE suppliers;
DROP TABLE party_contacts;
DROP TABLE parties;
DROP TABLE password_reset_tokens;
DROP TABLE sessions;
DROP TABLE users;
DROP TABLE company;
