-- Papéis de acesso configuráveis.
--
--   - PERMISSÕES ficam no código (internal/auth/permissions.go): uma permissão
--     só tem efeito se o código conferir. Aqui elas são só texto.
--   - PAPÉIS ficam no banco: o administrador cria pela tela e marca quais
--     permissões cada um tem (role_permissions).
--   - Cada usuário tem UM papel (users.role_id).
--   - O papel com key = 'admin' é do sistema: tem todas as permissões (sem
--     precisar de linhas em role_permissions) e não pode ser alterado.
--
-- Os seis papéis que existiam como texto viram linhas, com as permissões
-- padrão de cada um, e os usuários passam a apontar para eles.

-- +goose Up
CREATE TABLE roles (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Identificador fixo dos papéis que vêm com o sistema; NULL nos criados
    -- pela tela. Só 'admin' tem significado especial no código.
    key         TEXT UNIQUE,
    name        TEXT NOT NULL,
    description TEXT,
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX roles_name_key ON roles (lower(name));

CREATE TABLE role_permissions (
    role_id    BIGINT NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);

INSERT INTO roles (key, name, description) VALUES
    ('admin',   'Administrador', 'Acesso total, inclusive usuários, papéis e dados da empresa'),
    ('manager', 'Gerente',       'Acompanha a operação e mantém os cadastros'),
    ('seller',  'Vendedor',      'Atende clientes e lança pedidos'),
    ('checker', 'Conferente',    'Confere entrada e saída de mercadoria'),
    ('finance', 'Financeiro',    'Boletos, cheques, contas a pagar e receber'),
    ('driver',  'Motorista',     'Vê as entregas da sua rota');

-- Permissões padrão (o administrador não precisa: tem todas).
INSERT INTO role_permissions (role_id, permission)
SELECT r.id, p.permission
FROM roles r
JOIN (VALUES
    ('manager', 'suppliers.view'), ('manager', 'suppliers.edit'),
    ('manager', 'products.view'),  ('manager', 'products.edit'),
    ('seller',  'suppliers.view'), ('seller',  'products.view'),
    ('checker', 'suppliers.view'), ('checker', 'products.view'),
    ('finance', 'suppliers.view'), ('finance', 'suppliers.edit'), ('finance', 'products.view'),
    ('driver',  'products.view')
) AS p (role_key, permission) ON p.role_key = r.key;

ALTER TABLE users ADD COLUMN role_id BIGINT REFERENCES roles (id);
UPDATE users u SET role_id = r.id FROM roles r WHERE r.key = u.role;
ALTER TABLE users ALTER COLUMN role_id SET NOT NULL;
ALTER TABLE users DROP COLUMN role;
CREATE INDEX users_role_id_idx ON users (role_id);

-- +goose Down
ALTER TABLE users ADD COLUMN role TEXT;
UPDATE users u SET role = coalesce(r.key, 'seller') FROM roles r WHERE r.id = u.role_id;
ALTER TABLE users ALTER COLUMN role SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('admin', 'manager', 'seller', 'checker', 'finance', 'driver'));
ALTER TABLE users DROP COLUMN role_id;
DROP TABLE role_permissions;
DROP TABLE roles;
