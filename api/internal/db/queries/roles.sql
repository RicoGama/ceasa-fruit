-- Papéis de acesso e suas permissões (ver migration 00006).

-- name: ListRoles :many
SELECT r.*,
       ARRAY(SELECT rp.permission FROM role_permissions rp
             WHERE rp.role_id = r.id ORDER BY rp.permission)::text[] AS permissions,
       (SELECT count(*) FROM users u WHERE u.role_id = r.id AND u.active) AS active_users
FROM roles r
ORDER BY r.key = 'admin' DESC, lower(r.name);

-- name: GetRole :one
SELECT r.*,
       ARRAY(SELECT rp.permission FROM role_permissions rp
             WHERE rp.role_id = r.id ORDER BY rp.permission)::text[] AS permissions,
       (SELECT count(*) FROM users u WHERE u.role_id = r.id AND u.active) AS active_users
FROM roles r
WHERE r.id = @id;

-- name: GetRoleIDByKey :one
SELECT id FROM roles WHERE key = @key;

-- name: CreateRole :one
INSERT INTO roles (name, description)
VALUES (@name, @description)
RETURNING id;

-- Papel do sistema (key = 'admin') nunca é alterado por aqui.
-- name: UpdateRole :execrows
UPDATE roles
SET name = @name, description = @description,
    version = version + 1, updated_at = now()
WHERE id = @id AND version = @version AND key IS DISTINCT FROM 'admin';

-- name: DeleteRolePermissions :exec
DELETE FROM role_permissions WHERE role_id = @role_id;

-- name: AddRolePermissions :exec
INSERT INTO role_permissions (role_id, permission)
SELECT @role_id, unnest(@permissions::text[]);

-- Só apaga papel sem nenhum usuário (nem desativado) e que não seja o admin.
-- name: DeleteRole :execrows
DELETE FROM roles r
WHERE r.id = @id AND r.key IS DISTINCT FROM 'admin'
  AND NOT EXISTS (SELECT 1 FROM users u WHERE u.role_id = r.id);
