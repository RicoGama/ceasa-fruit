-- Histórico de alterações: quem mudou o quê, quando, de quanto para quanto.
--
-- Um gatilho em cada tabela de cadastro grava, na MESMA transação da
-- alteração, uma linha em audit_log com:
--   - o que aconteceu: I (criou), U (alterou) ou D (apagou);
--   - só os campos que mudaram, com o valor antigo e o novo:
--       {"phone": ["21999990000", "21988887777"]}
--     (na criação e na exclusão, a linha inteira);
--   - de qual cadastro é (entity_type + entity_id): o contato 7 da pessoa 42
--     entra como party/42, para aparecer no histórico do fornecedor 42;
--   - quem fez: a API informa o usuário logado em cada transação
--     (set_config('app.actor_id'), em internal/postgres/tx.go). Gravação sem
--     usuário (sincronização da frota, seed, script) fica com actor vazio:
--     "Sistema" na tela.
--
-- Por que no banco: nenhuma gravação escapa (tela, script, outro programa) e
-- se a transação for desfeita, o histórico também é. O histórico nunca é
-- alterado nem apagado.

-- +goose Up
CREATE TABLE audit_log (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- now() é o início da transação: tudo o que um "salvar" gravou tem a
    -- mesma hora e o mesmo tx_id (a tela junta numa alteração só).
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    tx_id         BIGINT      NOT NULL DEFAULT txid_current(),
    actor_user_id BIGINT,
    entity_type   TEXT        NOT NULL,
    entity_id     BIGINT      NOT NULL,
    table_name    TEXT        NOT NULL,
    row_id        BIGINT,
    action        CHAR(1)     NOT NULL CHECK (action IN ('I', 'U', 'D')),
    changes       JSONB       NOT NULL
);
CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id, id DESC);
CREATE INDEX audit_log_occurred_idx ON audit_log (occurred_at DESC, id DESC);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_user_id, occurred_at DESC);

-- audit_row(entity_type, coluna_do_id_da_entidade, colunas_ignoradas)
-- Sempre ignoradas: created_at, updated_at e version (mudam a cada salvar).
-- +goose StatementBegin
CREATE FUNCTION audit_row() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    entity_type text   := TG_ARGV[0];
    id_col      text   := TG_ARGV[1];
    ignored     text[] := ARRAY['created_at', 'updated_at', 'version']
                          || coalesce(string_to_array(nullif(TG_ARGV[2], ''), ','), '{}');
    old_j       jsonb;
    new_j       jsonb;
    row_j       jsonb;
    changes     jsonb;
BEGIN
    IF TG_OP <> 'INSERT' THEN old_j := to_jsonb(OLD) - ignored; END IF;
    IF TG_OP <> 'DELETE' THEN new_j := to_jsonb(NEW) - ignored; END IF;

    IF TG_OP = 'UPDATE' THEN
        SELECT jsonb_object_agg(k, jsonb_build_array(old_j -> k, new_j -> k))
        INTO changes
        FROM jsonb_object_keys(new_j) AS k
        WHERE (old_j -> k) IS DISTINCT FROM (new_j -> k);
        -- Salvou sem mudar nada (ou só colunas ignoradas): não registra.
        IF changes IS NULL THEN
            RETURN NULL;
        END IF;
        row_j := new_j;
    ELSIF TG_OP = 'INSERT' THEN
        changes := jsonb_strip_nulls(new_j);
        row_j := new_j;
    ELSE
        changes := jsonb_strip_nulls(old_j);
        row_j := old_j;
    END IF;

    INSERT INTO audit_log (actor_user_id, entity_type, entity_id, table_name, row_id, action, changes)
    VALUES (
        nullif(current_setting('app.actor_id', true), '')::bigint,
        entity_type,
        -- O id da entidade pode estar entre as colunas ignoradas? Nunca: lê da linha original.
        (to_jsonb(CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END) ->> id_col)::bigint,
        TG_TABLE_NAME,
        (row_j ->> 'id')::bigint,
        left(TG_OP, 1),
        changes
    );
    RETURN NULL;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER parties_audit AFTER INSERT OR UPDATE OR DELETE ON parties
    FOR EACH ROW EXECUTE FUNCTION audit_row('party', 'id', '');
CREATE TRIGGER party_contacts_audit AFTER INSERT OR UPDATE OR DELETE ON party_contacts
    FOR EACH ROW EXECUTE FUNCTION audit_row('party', 'party_id', 'position');
CREATE TRIGGER suppliers_audit AFTER INSERT OR UPDATE OR DELETE ON suppliers
    FOR EACH ROW EXECUTE FUNCTION audit_row('party', 'party_id', '');
CREATE TRIGGER customers_audit AFTER INSERT OR UPDATE OR DELETE ON customers
    FOR EACH ROW EXECUTE FUNCTION audit_row('party', 'party_id', '');
CREATE TRIGGER supplier_products_audit AFTER INSERT OR UPDATE OR DELETE ON supplier_products
    FOR EACH ROW EXECUTE FUNCTION audit_row('party', 'supplier_id', '');
CREATE TRIGGER products_audit AFTER INSERT OR UPDATE OR DELETE ON products
    FOR EACH ROW EXECUTE FUNCTION audit_row('product', 'id', '');
CREATE TRIGGER product_packagings_audit AFTER INSERT OR UPDATE OR DELETE ON product_packagings
    FOR EACH ROW EXECUTE FUNCTION audit_row('product', 'product_id', 'position');
-- A senha nunca entra no histórico (nem o hash).
CREATE TRIGGER users_audit AFTER INSERT OR UPDATE OR DELETE ON users
    FOR EACH ROW EXECUTE FUNCTION audit_row('user', 'id', 'password_hash');
CREATE TRIGGER roles_audit AFTER INSERT OR UPDATE OR DELETE ON roles
    FOR EACH ROW EXECUTE FUNCTION audit_row('role', 'id', '');
CREATE TRIGGER role_permissions_audit AFTER INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION audit_row('role', 'role_id', '');
CREATE TRIGGER company_audit AFTER INSERT OR UPDATE OR DELETE ON company
    FOR EACH ROW EXECUTE FUNCTION audit_row('company', 'id', '');
CREATE TRIGGER drivers_audit AFTER INSERT OR UPDATE OR DELETE ON drivers
    FOR EACH ROW EXECUTE FUNCTION audit_row('driver', 'id', '');
-- A última posição e o vínculo com o Traccar mudam a cada 15 s pela
-- sincronização: não são "alterações do cadastro".
CREATE TRIGGER vehicles_audit AFTER INSERT OR UPDATE OR DELETE ON vehicles
    FOR EACH ROW EXECUTE FUNCTION audit_row('vehicle', 'id',
        'traccar_device_id,traccar_last_update,last_fix_at,last_latitude,last_longitude,last_speed_kmh,last_course,last_ignition,last_motion');

-- Tempo real: o histórico aberto numa tela atualiza sozinho.
CREATE TRIGGER audit_log_notify AFTER INSERT ON audit_log
    FOR EACH ROW EXECUTE FUNCTION notify_change('audit_log', 'entity_id');

-- +goose Down
DROP TRIGGER audit_log_notify ON audit_log;
DROP TRIGGER vehicles_audit ON vehicles;
DROP TRIGGER drivers_audit ON drivers;
DROP TRIGGER company_audit ON company;
DROP TRIGGER role_permissions_audit ON role_permissions;
DROP TRIGGER roles_audit ON roles;
DROP TRIGGER users_audit ON users;
DROP TRIGGER product_packagings_audit ON product_packagings;
DROP TRIGGER products_audit ON products;
DROP TRIGGER supplier_products_audit ON supplier_products;
DROP TRIGGER customers_audit ON customers;
DROP TRIGGER suppliers_audit ON suppliers;
DROP TRIGGER party_contacts_audit ON party_contacts;
DROP TRIGGER parties_audit ON parties;
DROP FUNCTION audit_row();
DROP TABLE audit_log;
