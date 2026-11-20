-- Gestão de usuários (tela do administrador).

-- name: ListUsers :many
SELECT * FROM users
ORDER BY active DESC, lower(name), id;

-- name: GetUser :one
SELECT * FROM users
WHERE id = @id;

-- Só altera se a versão ainda for a que a tela leu (concorrência otimista).
-- name: UpdateUser :one
UPDATE users
SET name = @name, role = @role, active = @active,
    version = version + 1, updated_at = now()
WHERE id = @id AND version = @version
RETURNING *;

-- Trava as linhas dos administradores ativos até o fim da transação.
-- Quem chegar depois ESPERA, e só então enxerga o que mudou. Ordenado por id
-- para todas as transações travarem na mesma ordem (evita deadlock).
-- name: LockActiveAdmins :many
SELECT id FROM users
WHERE role = 'admin' AND active
ORDER BY id
FOR UPDATE;

-- Usada para impedir que o sistema fique sem nenhum administrador ativo.
-- name: CountActiveAdmins :one
SELECT count(*) FROM users
WHERE role = 'admin' AND active;
