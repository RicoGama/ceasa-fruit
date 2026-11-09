-- +goose Up

-- Organizações: os clientes do SaaS (a distribuidora/atacadista que paga a assinatura).
CREATE TABLE organizations (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL,
    -- 'suspended' bloqueia o acesso de todos os usuários da organização.
    status     TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Usuários são globais: a mesma pessoa pode participar de várias organizações.
CREATE TABLE users (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name              TEXT        NOT NULL,
    -- Sempre gravado em minúsculas, para o UNIQUE funcionar sem depender de maiúsculas.
    email             TEXT        NOT NULL UNIQUE CHECK (email = lower(email)),
    password_hash     TEXT        NOT NULL,
    -- Administrador da plataforma (o dono do SaaS): enxerga todas as organizações.
    is_platform_admin BOOLEAN     NOT NULL DEFAULT FALSE,
    active            BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ligação muitos-para-muitos entre usuários e organizações, com o papel do usuário.
CREATE TABLE memberships (
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role            TEXT        NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);
CREATE INDEX memberships_user_idx ON memberships (user_id);

-- Sessões de login. O navegador guarda o token num cookie; aqui guardamos só o
-- hash SHA-256 dele, então um vazamento do banco não entrega sessões válidas.
CREATE TABLE sessions (
    token_hash      BYTEA       PRIMARY KEY,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Organização selecionada no momento (NULL = usuário ainda não escolheu).
    organization_id BIGINT      REFERENCES organizations (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

-- Fornecedores passam a pertencer a uma organização.
-- Ainda não há produção: fornecedores de teste sem dono são descartados.
DELETE FROM suppliers;
ALTER TABLE suppliers ADD COLUMN organization_id BIGINT NOT NULL REFERENCES organizations (id);
-- O mesmo produtor pode vender para várias distribuidoras: documento único POR organização.
ALTER TABLE suppliers DROP CONSTRAINT suppliers_document_key;
ALTER TABLE suppliers ADD CONSTRAINT suppliers_organization_document_key UNIQUE (organization_id, document);
DROP INDEX suppliers_name_idx;
CREATE INDEX suppliers_organization_name_idx ON suppliers (organization_id, lower(name));

-- ---------------------------------------------------------------------------
-- Row-Level Security (RLS): rede de segurança contra vazamento entre organizações.
--
-- As requisições de uma organização rodam numa transação que faz
--   SET LOCAL ROLE ceasa_app
--   set_config('app.organization_id', '<id>', true)
-- e o Postgres só deixa ler/gravar linhas daquela organização, mesmo que a
-- query esqueça o WHERE organization_id. (Superusuários e o dono da tabela
-- ignoram RLS; por isso trocamos para o papel ceasa_app.)
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ceasa_app') THEN
        CREATE ROLE ceasa_app NOLOGIN;
    END IF;
END
$$;
-- +goose StatementEnd

-- Permite que o usuário da aplicação faça SET ROLE ceasa_app.
GRANT ceasa_app TO CURRENT_USER;

GRANT USAGE ON SCHEMA public TO ceasa_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON suppliers TO ceasa_app;
GRANT USAGE ON SEQUENCE suppliers_id_seq TO ceasa_app;

ALTER TABLE suppliers ENABLE ROW LEVEL SECURITY;

-- NULLIF: fora de uma transação de organização a configuração vale '' e,
-- convertida para NULL, não casa com nenhuma linha.
CREATE POLICY tenant_isolation ON suppliers
    USING (organization_id = NULLIF(current_setting('app.organization_id', true), '')::BIGINT)
    WITH CHECK (organization_id = NULLIF(current_setting('app.organization_id', true), '')::BIGINT);

-- +goose Down
DROP POLICY tenant_isolation ON suppliers;
ALTER TABLE suppliers DISABLE ROW LEVEL SECURITY;
REVOKE ALL ON suppliers FROM ceasa_app;
REVOKE ALL ON SEQUENCE suppliers_id_seq FROM ceasa_app;
REVOKE USAGE ON SCHEMA public FROM ceasa_app;

DROP INDEX suppliers_organization_name_idx;
CREATE INDEX suppliers_name_idx ON suppliers (lower(name));
ALTER TABLE suppliers DROP CONSTRAINT suppliers_organization_document_key;
DELETE FROM suppliers;
ALTER TABLE suppliers ADD CONSTRAINT suppliers_document_key UNIQUE (document);
ALTER TABLE suppliers DROP COLUMN organization_id;

DROP TABLE sessions;
DROP TABLE memberships;
DROP TABLE users;
DROP TABLE organizations;
