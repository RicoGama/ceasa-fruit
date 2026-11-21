-- Limite de tentativas de login (ver internal/auth/ratelimit.go).
--
-- As falhas já ficam no registro de acessos (access_log); aqui só entram:
--   - o motivo "too_many_attempts" (tentativa recusada por estar bloqueado);
--   - índices para contar rápido as falhas recentes por e-mail e por IP.

-- +goose Up
ALTER TABLE access_log DROP CONSTRAINT access_log_end_reason_check;
ALTER TABLE access_log ADD CONSTRAINT access_log_end_reason_check CHECK (end_reason IN (
    'logout', 'password_changed', 'deactivated',
    'wrong_password', 'unknown_email', 'inactive_user', 'too_many_attempts'));

CREATE INDEX access_log_email_idx ON access_log (email, started_at DESC);
CREATE INDEX access_log_ip_idx ON access_log (ip, started_at DESC) WHERE ip IS NOT NULL;

-- +goose Down
DROP INDEX access_log_ip_idx;
DROP INDEX access_log_email_idx;
DELETE FROM access_log WHERE end_reason = 'too_many_attempts';
ALTER TABLE access_log DROP CONSTRAINT access_log_end_reason_check;
ALTER TABLE access_log ADD CONSTRAINT access_log_end_reason_check CHECK (end_reason IN (
    'logout', 'password_changed', 'deactivated',
    'wrong_password', 'unknown_email', 'inactive_user'));
