-- name: CreateSupplier :one
INSERT INTO suppliers (name, document, phone, email, city, state, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSupplier :one
SELECT * FROM suppliers
WHERE id = $1;

-- name: ListSuppliers :many
SELECT * FROM suppliers
WHERE (sqlc.narg('search')::text IS NULL
       OR unaccent(name) ILIKE '%' || unaccent(sqlc.narg('search')) || '%'
       OR document LIKE sqlc.narg('search') || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR active = sqlc.narg('active'))
ORDER BY lower(name), id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountSuppliers :one
SELECT count(*) FROM suppliers
WHERE (sqlc.narg('search')::text IS NULL
       OR unaccent(name) ILIKE '%' || unaccent(sqlc.narg('search')) || '%'
       OR document LIKE sqlc.narg('search') || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR active = sqlc.narg('active'));

-- name: UpdateSupplier :one
UPDATE suppliers
SET name       = $2,
    document   = $3,
    phone      = $4,
    email      = $5,
    city       = $6,
    state      = $7,
    notes      = $8,
    active     = $9,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeactivateSupplier :execrows
UPDATE suppliers
SET active = FALSE, updated_at = now()
WHERE id = $1;
