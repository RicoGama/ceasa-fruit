-- Empresa desta instalação (um registro só, id = 1).

-- name: GetCompany :one
SELECT c.*, ci.name AS city_name, ci.state_code AS city_state
FROM company c
LEFT JOIN cities ci ON ci.id = c.city_id
WHERE c.id = 1;

-- name: UpsertCompany :exec
INSERT INTO company (
    id, name, trade_name, document, state_registration, tax_regime, phone, email,
    postal_code, street, number, complement, district, city_id
) VALUES (
    1, @name, @trade_name, @document, @state_registration, @tax_regime, @phone, @email,
    @postal_code, @street, @number, @complement, @district, @city_id
)
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name, trade_name = EXCLUDED.trade_name, document = EXCLUDED.document,
    state_registration = EXCLUDED.state_registration, tax_regime = EXCLUDED.tax_regime,
    phone = EXCLUDED.phone, email = EXCLUDED.email, postal_code = EXCLUDED.postal_code,
    street = EXCLUDED.street, number = EXCLUDED.number, complement = EXCLUDED.complement,
    district = EXCLUDED.district, city_id = EXCLUDED.city_id, updated_at = now();

-- Cria a empresa só com o nome, se ainda não existir (usado pelo make seed).
-- name: EnsureCompany :exec
INSERT INTO company (id, name)
VALUES (1, @name)
ON CONFLICT (id) DO NOTHING;
