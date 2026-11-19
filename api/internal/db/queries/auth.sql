-- Queries de autenticação e senha.

-- name: CreateUser :one
INSERT INTO users (name, email, password_hash, role)
VALUES (@name, @email, @password_hash, @role)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = @email;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at)
VALUES (@token_hash, @user_id, @expires_at);

-- Carrega o que uma requisição autenticada precisa numa consulta só.
-- name: GetSession :one
SELECT u.id AS user_id, u.name AS user_name, u.email AS user_email, u.role
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = @token_hash
  AND s.expires_at > now()
  AND u.active;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = @token_hash;

-- Encerra todas as sessões do usuário (ex.: troca de senha, usuário desativado).
-- name: DeleteUserSessions :exec
DELETE FROM sessions
WHERE user_id = @user_id;

-- name: CreatePasswordResetToken :exec
INSERT INTO password_reset_tokens (token_hash, user_id, expires_at)
VALUES (@token_hash, @user_id, @expires_at);

-- Marca o token como usado e devolve o usuário, numa operação só: dois
-- pedidos simultâneos com o mesmo link não conseguem usá-lo duas vezes.
-- name: ConsumePasswordResetToken :one
UPDATE password_reset_tokens t
SET used_at = now()
FROM users u
WHERE t.token_hash = @token_hash
  AND t.used_at IS NULL
  AND t.expires_at > now()
  AND u.id = t.user_id
  AND u.active
RETURNING t.user_id;

-- Depois de redefinir, os outros links pendentes deixam de valer.
-- name: InvalidatePasswordResetTokens :exec
UPDATE password_reset_tokens
SET used_at = now()
WHERE user_id = @user_id AND used_at IS NULL;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = @password_hash, updated_at = now()
WHERE id = @id;
