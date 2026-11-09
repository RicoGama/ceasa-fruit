-- Queries de autenticação. Estas tabelas não têm RLS: são usadas antes de
-- sabermos qual organização está ativa.

-- name: CreateUser :one
INSERT INTO users (name, email, password_hash)
VALUES (@name, @email, @password_hash)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = @email;

-- name: CreateOrganization :one
INSERT INTO organizations (name)
VALUES (@name)
RETURNING *;

-- name: CreateMembership :exec
INSERT INTO memberships (organization_id, user_id, role)
VALUES (@organization_id, @user_id, @role);

-- name: ListUserOrganizations :many
SELECT o.id, o.name, o.status, m.role
FROM memberships m
JOIN organizations o ON o.id = m.organization_id
WHERE m.user_id = @user_id
ORDER BY lower(o.name), o.id;

-- name: GetMembership :one
SELECT m.role, o.status AS organization_status
FROM memberships m
JOIN organizations o ON o.id = m.organization_id
WHERE m.user_id = @user_id AND m.organization_id = @organization_id;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, organization_id, expires_at)
VALUES (@token_hash, @user_id, @organization_id, @expires_at);

-- Carrega tudo o que uma requisição autenticada precisa numa consulta só.
-- LEFT JOIN: a sessão pode ainda não ter organização selecionada.
-- name: GetSession :one
SELECT s.user_id,
       s.organization_id,
       u.name              AS user_name,
       u.email             AS user_email,
       u.is_platform_admin,
       m.role,
       o.status            AS organization_status
FROM sessions s
JOIN users u ON u.id = s.user_id
LEFT JOIN memberships m ON m.user_id = s.user_id AND m.organization_id = s.organization_id
LEFT JOIN organizations o ON o.id = s.organization_id
WHERE s.token_hash = @token_hash
  AND s.expires_at > now()
  AND u.active;

-- name: SetSessionOrganization :exec
UPDATE sessions
SET organization_id = @organization_id
WHERE token_hash = @token_hash;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = @token_hash;

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

-- Encerra todas as sessões do usuário (ex.: depois de trocar a senha).
-- name: DeleteUserSessions :exec
DELETE FROM sessions
WHERE user_id = @user_id;
