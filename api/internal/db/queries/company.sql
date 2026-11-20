-- Empresa desta instalação (um registro só, id = 1).

-- name: GetCompany :one
SELECT c.*, ci.name AS city_name, ci.state_code AS city_state
FROM company c
LEFT JOIN cities ci ON ci.id = c.city_id
WHERE c.id = 1;

-- Só altera se a versão ainda for a que a tela leu (concorrência otimista).
-- name: UpdateCompany :execrows
UPDATE company
SET name = @name, trade_name = @trade_name, document = @document,
    state_registration = @state_registration, tax_regime = @tax_regime,
    phone = @phone, email = @email, postal_code = @postal_code,
    street = @street, number = @number, complement = @complement,
    district = @district, city_id = @city_id,
    version = version + 1, updated_at = now()
WHERE id = 1 AND version = @version;

-- Cria a empresa só com o nome, se ainda não existir (usado pelo make seed).
-- name: EnsureCompany :exec
INSERT INTO company (id, name)
VALUES (1, @name)
ON CONFLICT (id) DO NOTHING;
