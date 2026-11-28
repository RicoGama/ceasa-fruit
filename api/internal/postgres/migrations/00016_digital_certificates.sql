-- Certificado digital A1 da empresa (arquivo .pfx/.p12 + senha), usado para
-- assinar as notas fiscais e para falar com a SEFAZ.
--
-- Onde fica: AQUI, no banco, criptografado (internal/secrets, chave em
-- SECRETS_KEY). Assim ele entra nos backups junto com o resto, sem ficar
-- legível para quem tiver só o banco ou só o backup.
--
-- Só um certificado vale por vez (active). Ao trocar, o antigo fica como
-- histórico (titular, validade, quem enviou), mas o arquivo e a senha dele
-- são apagados: não há motivo para guardar uma chave privada que saiu de uso.

-- +goose Up
CREATE TABLE digital_certificates (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    active             BOOLEAN     NOT NULL DEFAULT true,
    holder_name        TEXT        NOT NULL,          -- titular (razão social)
    document           TEXT,                          -- CNPJ do titular
    issuer             TEXT        NOT NULL,          -- autoridade certificadora
    serial_number      TEXT        NOT NULL,
    fingerprint        TEXT        NOT NULL,          -- SHA-256 do certificado
    not_before         TIMESTAMPTZ NOT NULL,
    not_after          TIMESTAMPTZ NOT NULL,          -- vencimento
    encrypted_pfx      BYTEA,                         -- arquivo .pfx, criptografado
    encrypted_password BYTEA,                         -- senha, criptografada
    key_id             TEXT,                          -- qual chave criptografou
    uploaded_by        BIGINT      REFERENCES users (id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    replaced_at        TIMESTAMPTZ,
    -- O ativo tem arquivo, senha e chave; o substituído, nenhum dos três.
    CHECK ((active AND encrypted_pfx IS NOT NULL AND encrypted_password IS NOT NULL AND key_id IS NOT NULL)
        OR (NOT active AND encrypted_pfx IS NULL AND encrypted_password IS NULL)),
    CHECK (not_after > not_before)
);
-- No máximo um ativo.
CREATE UNIQUE INDEX digital_certificates_one_active ON digital_certificates (active) WHERE active;

CREATE TRIGGER digital_certificates_notify AFTER INSERT OR UPDATE OR DELETE ON digital_certificates
    FOR EACH ROW EXECUTE FUNCTION notify_change('digital_certificates', 'id');
-- O histórico registra envio e troca, nunca o arquivo nem a senha.
CREATE TRIGGER digital_certificates_audit AFTER INSERT OR UPDATE OR DELETE ON digital_certificates
    FOR EACH ROW EXECUTE FUNCTION audit_row('certificate', 'id', 'encrypted_pfx,encrypted_password,key_id');

-- +goose Down
DROP TABLE digital_certificates;
