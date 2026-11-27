-- Clientes. A pessoa (parties, party_contacts) usa as consultas de
-- suppliers.sql: CreateParty, UpdateParty, ClaimPartyVersion...

-- name: CreateCustomer :exec
INSERT INTO customers (party_id, customer_type, ceasa_pavilion, ceasa_box, status, blocked_reason)
VALUES (@party_id, @customer_type, @ceasa_pavilion, @ceasa_box, @status, @blocked_reason);

-- name: UpdateCustomer :execrows
UPDATE customers
SET customer_type = @customer_type, ceasa_pavilion = @ceasa_pavilion, ceasa_box = @ceasa_box,
    status = @status, blocked_reason = @blocked_reason, updated_at = now()
WHERE party_id = @party_id;

-- name: GetCustomer :one
SELECT sqlc.embed(p), sqlc.embed(cu),
       c.name       AS city_name,
       c.state_code AS city_state,
       EXISTS (SELECT 1 FROM suppliers su WHERE su.party_id = cu.party_id) AS is_supplier
FROM customers cu
JOIN parties p ON p.id = cu.party_id
LEFT JOIN cities c ON c.id = p.city_id
WHERE cu.party_id = @id;

-- name: ListCustomers :many
SELECT cu.party_id, p.person_type, p.document, p.legal_name, p.trade_name,
       p.phone, p.whatsapp, c.name AS city_name, c.state_code AS city_state,
       cu.customer_type, cu.ceasa_pavilion, cu.ceasa_box, cu.status,
       EXISTS (SELECT 1 FROM suppliers su WHERE su.party_id = cu.party_id) AS is_supplier
FROM customers cu
JOIN parties p ON p.id = cu.party_id
LEFT JOIN cities c ON c.id = p.city_id
WHERE (sqlc.narg('search')::text IS NULL
       OR unaccent(p.legal_name) ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%'
       OR unaccent(p.trade_name) ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%'
       OR p.document LIKE like_escape(upper(sqlc.narg('search'))) || '%')
  AND (sqlc.narg('status')::text IS NULL OR cu.status = sqlc.narg('status'))
  AND (sqlc.narg('customer_type')::text IS NULL OR cu.customer_type = sqlc.narg('customer_type'))
ORDER BY lower(coalesce(p.trade_name, p.legal_name)), cu.party_id
LIMIT @limit_rows OFFSET @offset_rows;

-- name: CountCustomers :one
SELECT count(*)
FROM customers cu
JOIN parties p ON p.id = cu.party_id
WHERE (sqlc.narg('search')::text IS NULL
       OR unaccent(p.legal_name) ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%'
       OR unaccent(p.trade_name) ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%'
       OR p.document LIKE like_escape(upper(sqlc.narg('search'))) || '%')
  AND (sqlc.narg('status')::text IS NULL OR cu.status = sqlc.narg('status'))
  AND (sqlc.narg('customer_type')::text IS NULL OR cu.customer_type = sqlc.narg('customer_type'));

-- name: DeactivateCustomer :execrows
UPDATE customers
SET status = 'inactive', blocked_reason = NULL, updated_at = now()
WHERE party_id = @party_id;
