-- +goose Up
-- Pedidos de redefinição de senha. Como nas sessões, o link enviado por
-- e-mail leva o token, e aqui fica só o hash SHA-256 dele.
CREATE TABLE password_reset_tokens (
    token_hash BYTEA       PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,             -- preenchido quando o link é usado (uso único)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX password_reset_tokens_user_idx ON password_reset_tokens (user_id);

-- +goose Down
DROP TABLE password_reset_tokens;
