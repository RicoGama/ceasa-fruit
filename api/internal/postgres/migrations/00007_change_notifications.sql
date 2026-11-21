-- Tempo real: o banco avisa quando algo muda.
--
-- Cada tabela importante tem um gatilho (trigger) que, a cada INSERT, UPDATE
-- ou DELETE, manda um aviso no canal "app_events" com o assunto (topic) e o
-- id do registro. A API escuta esse canal (internal/realtime) e repassa aos
-- navegadores, que recarregam o que estão mostrando.
--
-- Por que no banco, e não no código Go?
--   - nenhuma gravação escapa: tela, seed, script, migração... todas avisam;
--   - o NOTIFY só é entregue DEPOIS do COMMIT: se a transação desfizer
--     (ROLLBACK), ninguém é avisado de algo que não aconteceu;
--   - avisos iguais na mesma transação viram um só (o Postgres junta).
--
-- O aviso leva só assunto + id, nunca os dados: quem recebe busca o registro
-- pela API, que confere as permissões normalmente.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION notify_change() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    topic  text := TG_ARGV[0];  -- assunto: suppliers, products, users...
    id_col text := TG_ARGV[1];  -- coluna com o id do registro "pai"
    rec    jsonb;
BEGIN
    rec := to_jsonb(CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END);
    PERFORM pg_notify('app_events', json_build_object(
        'topic', topic,
        'id',    (rec ->> id_col)::bigint
    )::text);
    RETURN NULL; -- gatilho AFTER: o valor de retorno é ignorado
END
$$;
-- +goose StatementEnd

-- Tabelas filhas (contatos, embalagens, permissões) avisam com o id do pai:
-- quem está vendo o produto 7 quer saber que uma embalagem dele mudou.
CREATE TRIGGER suppliers_notify AFTER INSERT OR UPDATE OR DELETE ON suppliers
    FOR EACH ROW EXECUTE FUNCTION notify_change('suppliers', 'party_id');
CREATE TRIGGER products_notify AFTER INSERT OR UPDATE OR DELETE ON products
    FOR EACH ROW EXECUTE FUNCTION notify_change('products', 'id');
CREATE TRIGGER product_packagings_notify AFTER INSERT OR UPDATE OR DELETE ON product_packagings
    FOR EACH ROW EXECUTE FUNCTION notify_change('products', 'product_id');
CREATE TRIGGER users_notify AFTER INSERT OR UPDATE OR DELETE ON users
    FOR EACH ROW EXECUTE FUNCTION notify_change('users', 'id');
CREATE TRIGGER roles_notify AFTER INSERT OR UPDATE OR DELETE ON roles
    FOR EACH ROW EXECUTE FUNCTION notify_change('roles', 'id');
CREATE TRIGGER role_permissions_notify AFTER INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION notify_change('roles', 'role_id');
CREATE TRIGGER company_notify AFTER INSERT OR UPDATE ON company
    FOR EACH ROW EXECUTE FUNCTION notify_change('company', 'id');

-- +goose Down
DROP TRIGGER company_notify ON company;
DROP TRIGGER role_permissions_notify ON role_permissions;
DROP TRIGGER roles_notify ON roles;
DROP TRIGGER users_notify ON users;
DROP TRIGGER product_packagings_notify ON product_packagings;
DROP TRIGGER products_notify ON products;
DROP TRIGGER suppliers_notify ON suppliers;
DROP FUNCTION notify_change();
