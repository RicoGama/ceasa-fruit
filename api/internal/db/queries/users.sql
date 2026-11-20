-- Gestão de usuários (tela do administrador).

-- name: ListUsers :many
SELECT sqlc.embed(u), r.name AS role_name
FROM users u
JOIN roles r ON r.id = u.role_id
ORDER BY u.active DESC, lower(u.name), u.id;

-- name: GetUser :one
SELECT * FROM users
WHERE id = @id;

-- Só altera se a versão ainda for a que a tela leu (concorrência otimista).
-- name: UpdateUser :one
UPDATE users
SET name = @name, role_id = @role_id, active = @active,
    version = version + 1, updated_at = now()
WHERE id = @id AND version = @version
RETURNING *;

-- Trava as linhas dos administradores ativos até o fim da transação.
-- Quem chegar depois ESPERA, e só então enxerga o que mudou. Ordenado por id
-- para todas as transações travarem na mesma ordem (evita deadlock).
-- name: LockActiveAdmins :many
SELECT u.id FROM users u
JOIN roles r ON r.id = u.role_id
WHERE r.key = 'admin' AND u.active
ORDER BY u.id
FOR UPDATE OF u;

-- Usada para impedir que o sistema fique sem nenhum administrador ativo.
-- name: CountActiveAdmins :one
SELECT count(*) FROM users u
JOIN roles r ON r.id = u.role_id
WHERE r.key = 'admin' AND u.active;

-- name: GetUserWithRole :one
SELECT sqlc.embed(u), r.name AS role_name, r.key AS role_key
FROM users u
JOIN roles r ON r.id = u.role_id
WHERE u.id = @id;
