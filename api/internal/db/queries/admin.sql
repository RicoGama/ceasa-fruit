-- Queries do painel do administrador da plataforma (o dono do SaaS).

-- name: AdminListOrganizations :many
SELECT o.*,
       (SELECT count(*) FROM memberships m WHERE m.organization_id = o.id) AS member_count
FROM organizations o
ORDER BY o.created_at DESC, o.id DESC
LIMIT @limit_rows OFFSET @offset_rows;

-- name: AdminCountOrganizations :one
SELECT count(*) FROM organizations;

-- name: AdminSetOrganizationStatus :one
UPDATE organizations
SET status = @status, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: PromotePlatformAdmin :execrows
UPDATE users
SET is_platform_admin = TRUE, updated_at = now()
WHERE email = @email;
