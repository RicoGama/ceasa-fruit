-- Toda query filtra por organization_id explicitamente, MESMO com o RLS ativo.
-- São duas camadas: se uma falhar, a outra segura.
-- @nome é atalho do sqlc para sqlc.arg('nome').

-- name: CreateSupplier :one
INSERT INTO suppliers (organization_id, name, document, phone, email, city, state, notes)
VALUES (@organization_id, @name, @document, @phone, @email, @city, @state, @notes)
RETURNING *;

-- name: GetSupplier :one
SELECT * FROM suppliers
WHERE organization_id = @organization_id AND id = @id;

-- name: ListSuppliers :many
SELECT * FROM suppliers
WHERE organization_id = @organization_id
  AND (sqlc.narg('search')::text IS NULL
       OR unaccent(name) ILIKE '%' || unaccent(sqlc.narg('search')) || '%'
       OR document LIKE sqlc.narg('search') || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR active = sqlc.narg('active'))
ORDER BY lower(name), id
LIMIT @limit_rows OFFSET @offset_rows;

-- name: CountSuppliers :one
SELECT count(*) FROM suppliers
WHERE organization_id = @organization_id
  AND (sqlc.narg('search')::text IS NULL
       OR unaccent(name) ILIKE '%' || unaccent(sqlc.narg('search')) || '%'
       OR document LIKE sqlc.narg('search') || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR active = sqlc.narg('active'));

-- name: UpdateSupplier :one
UPDATE suppliers
SET name       = @name,
    document   = @document,
    phone      = @phone,
    email      = @email,
    city       = @city,
    state      = @state,
    notes      = @notes,
    active     = @active,
    updated_at = now()
WHERE organization_id = @organization_id AND id = @id
RETURNING *;

-- name: DeactivateSupplier :execrows
UPDATE suppliers
SET active = FALSE, updated_at = now()
WHERE organization_id = @organization_id AND id = @id;
