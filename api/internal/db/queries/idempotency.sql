-- Idempotência (migration 00014).

-- name: GetIdempotencyKey :one
SELECT * FROM idempotency_keys WHERE key = @key;

-- Grava a chave na transação da gravação de verdade. ON CONFLICT: se outro
-- pedido com a mesma chave chegou junto, este espera aquele terminar e não
-- grava (0 linhas).
-- name: ClaimIdempotencyKey :execrows
INSERT INTO idempotency_keys (key, user_id, method, path, request_hash)
VALUES (@key, @user_id, @method, @path, @request_hash)
ON CONFLICT (key) DO NOTHING;

-- name: CompleteIdempotencyKey :exec
UPDATE idempotency_keys
SET status = @status, content_type = @content_type, response_body = @response_body, completed_at = now()
WHERE key = @key;

-- name: DeleteOldIdempotencyKeys :execrows
DELETE FROM idempotency_keys WHERE created_at < @before;
