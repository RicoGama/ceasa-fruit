-- Registro de acessos (ver migration 00009).

-- name: CreateSessionLog :one
INSERT INTO access_log (kind, user_id, email, ip, user_agent)
VALUES ('session', @user_id, @email, @ip, @user_agent)
RETURNING id;

-- Tentativa de login que falhou: já nasce encerrada, com o motivo.
-- name: CreateFailedLoginLog :exec
INSERT INTO access_log (kind, user_id, email, ip, user_agent, ended_at, end_reason)
VALUES ('failed_login', @user_id, @email, @ip, @user_agent, now(), @reason);

-- Última atividade (chamada no máximo uma vez por minuto por sessão).
-- name: TouchSessionLog :exec
UPDATE access_log SET last_seen_at = now()
WHERE id = @id AND ended_at IS NULL;

-- session_open: a sessão ainda existe e não venceu (para "online" e "expirou").
-- name: ListAccessLog :many
SELECT a.*, u.name AS user_name,
       EXISTS (SELECT 1 FROM sessions s
               WHERE s.access_log_id = a.id AND s.expires_at > now()) AS session_open
FROM access_log a
LEFT JOIN users u ON u.id = a.user_id
WHERE (sqlc.narg('user_id')::bigint IS NULL OR a.user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('kind')::text IS NULL OR a.kind = sqlc.narg('kind'))
  AND a.started_at >= @since
ORDER BY a.started_at DESC, a.id DESC
LIMIT @limit_rows OFFSET @offset_rows;

-- name: CountAccessLog :one
SELECT count(*) FROM access_log a
WHERE (sqlc.narg('user_id')::bigint IS NULL OR a.user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('kind')::text IS NULL OR a.kind = sqlc.narg('kind'))
  AND a.started_at >= @since;
