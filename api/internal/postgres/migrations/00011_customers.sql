-- Clientes: o papel "cliente" de uma pessoa (parties), como suppliers é o de
-- fornecedor. O mesmo CNPJ pode ser as duas coisas (comum na CEASA): os dados
-- da pessoa (nome, endereço, contatos) ficam uma vez só em parties.
--
-- Ficam para depois, quando o cliente responder as perguntas de vendas e
-- financeiro: limite de crédito, prazo e forma de pagamento padrão, tabela de
-- preço, vendedor responsável e dados de entrega.

-- +goose Up
CREATE TABLE customers (
    party_id       BIGINT      PRIMARY KEY REFERENCES parties (id) ON DELETE CASCADE,
    customer_type  TEXT        NOT NULL DEFAULT 'retailer'
                   CHECK (customer_type IN ('retailer', 'supermarket', 'restaurant', 'street_vendor',
                                            'wholesaler', 'institutional', 'consumer', 'other')),
    -- Onde o cliente fica na CEASA, quando ele também tem box.
    ceasa_pavilion TEXT,
    ceasa_box      TEXT,
    status         TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'blocked')),
    blocked_reason TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((status = 'blocked') = (blocked_reason IS NOT NULL))
);
CREATE INDEX customers_status_idx ON customers (status);

-- Tempo real do próprio cliente.
CREATE TRIGGER customers_notify AFTER INSERT OR UPDATE OR DELETE ON customers
    FOR EACH ROW EXECUTE FUNCTION notify_change('customers', 'party_id');

-- Mudou a PESSOA (nome, endereço, contatos)? Avisa as duas telas: quem está
-- com o fornecedor aberto precisa saber que alguém editou pelo cliente, e
-- vice-versa. Avisos repetidos na mesma transação o Postgres junta em um.
-- +goose StatementBegin
CREATE FUNCTION notify_party_change() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    pid bigint := (to_jsonb(CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END) ->> TG_ARGV[0])::bigint;
BEGIN
    IF EXISTS (SELECT 1 FROM suppliers WHERE party_id = pid) THEN
        PERFORM pg_notify('app_events', json_build_object('topic', 'suppliers', 'id', pid)::text);
    END IF;
    IF EXISTS (SELECT 1 FROM customers WHERE party_id = pid) THEN
        PERFORM pg_notify('app_events', json_build_object('topic', 'customers', 'id', pid)::text);
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER parties_notify AFTER UPDATE ON parties
    FOR EACH ROW EXECUTE FUNCTION notify_party_change('id');
CREATE TRIGGER party_contacts_notify AFTER INSERT OR UPDATE OR DELETE ON party_contacts
    FOR EACH ROW EXECUTE FUNCTION notify_party_change('party_id');

-- Permissões padrão dos papéis que já existem (o administrador pode mudar).
INSERT INTO role_permissions (role_id, permission)
SELECT r.id, p.permission
FROM roles r
JOIN (VALUES
    ('manager', 'customers.view'), ('manager', 'customers.edit'),
    ('seller',  'customers.view'), ('seller',  'customers.edit'),
    ('finance', 'customers.view'), ('finance', 'customers.edit'),
    ('checker', 'customers.view'),
    ('driver',  'customers.view')
) AS p (role_key, permission) ON p.role_key = r.key
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM role_permissions WHERE permission IN ('customers.view', 'customers.edit');
DROP TRIGGER party_contacts_notify ON party_contacts;
DROP TRIGGER parties_notify ON parties;
DROP FUNCTION notify_party_change();
DROP TABLE customers;
