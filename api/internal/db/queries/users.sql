-- Gestão de usuários (tela do administrador).

-- name: ListUsers :many
SELECT * FROM users
ORDER BY active DESC, lower(name), id;

-- name: GetUser :one
SELECT * FROM users
WHERE id = @id;

-- name: UpdateUser :one
UPDATE users
SET name = @name, role = @role, active = @active, updated_at = now()
WHERE id = @id
RETURNING *;

-- Usada para impedir que o sistema fique sem nenhum administrador ativo.
-- name: CountActiveAdmins :one
SELECT count(*) FROM users
WHERE role = 'admin' AND active;
