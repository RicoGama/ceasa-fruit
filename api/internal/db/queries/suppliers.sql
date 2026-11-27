-- @nome é atalho do sqlc para sqlc.arg('nome'); sqlc.narg = pode ser NULL.

-- name: CreateParty :one
INSERT INTO parties (
    person_type, document, legal_name, trade_name,
    state_registration, taxpayer_type, email, phone, whatsapp,
    postal_code, street, number, complement, district, city_id,
    pix_key_type, pix_key, bank, bank_branch, bank_account, bank_account_type,
    bank_account_holder, bank_account_holder_document, notes
) VALUES (
    @person_type, @document, @legal_name, @trade_name,
    @state_registration, @taxpayer_type, @email, @phone, @whatsapp,
    @postal_code, @street, @number, @complement, @district, @city_id,
    @pix_key_type, @pix_key, @bank, @bank_branch, @bank_account, @bank_account_type,
    @bank_account_holder, @bank_account_holder_document, @notes
)
RETURNING id;

-- name: UpdateParty :execrows
UPDATE parties
SET person_type = @person_type, document = @document, legal_name = @legal_name,
    trade_name = @trade_name, state_registration = @state_registration,
    taxpayer_type = @taxpayer_type, email = @email, phone = @phone, whatsapp = @whatsapp,
    postal_code = @postal_code, street = @street, number = @number,
    complement = @complement, district = @district, city_id = @city_id,
    pix_key_type = @pix_key_type, pix_key = @pix_key, bank = @bank,
    bank_branch = @bank_branch, bank_account = @bank_account,
    bank_account_type = @bank_account_type, bank_account_holder = @bank_account_holder,
    bank_account_holder_document = @bank_account_holder_document, notes = @notes,
    updated_at = now()
WHERE id = @id;

-- "Reserva" a edição da pessoa: só passa se ninguém salvou depois que a tela
-- leu (version igual) e já soma 1. 0 linhas = alterada por outra pessoa (ou
-- inexistente). Toda gravação de fornecedor/cliente começa por aqui.
-- name: ClaimPartyVersion :execrows
UPDATE parties
SET version = version + 1, updated_at = now()
WHERE id = @id AND version = @version;

-- Soma 1 na versão sem conferir (ações que não vêm de um formulário, como
-- inativar): quem estiver com a tela aberta recebe 409 ao salvar.
-- name: BumpPartyVersion :exec
UPDATE parties
SET version = version + 1, updated_at = now()
WHERE id = @id;

-- name: CreateSupplier :exec
INSERT INTO suppliers (
    party_id, supplier_type, default_purchase_mode,
    consignment_commission, payment_term_days, ceasa_pavilion, ceasa_box,
    status, blocked_reason
) VALUES (
    @party_id, @supplier_type, @default_purchase_mode,
    @consignment_commission, @payment_term_days, @ceasa_pavilion, @ceasa_box,
    @status, @blocked_reason
);

-- name: UpdateSupplier :execrows
UPDATE suppliers
SET supplier_type = @supplier_type, default_purchase_mode = @default_purchase_mode,
    consignment_commission = @consignment_commission, payment_term_days = @payment_term_days,
    ceasa_pavilion = @ceasa_pavilion, ceasa_box = @ceasa_box,
    status = @status, blocked_reason = @blocked_reason, updated_at = now()
WHERE party_id = @party_id;

-- sqlc.embed devolve as colunas de cada tabela agrupadas (row.Party, row.Supplier).
-- name: GetSupplier :one
SELECT sqlc.embed(p), sqlc.embed(s),
       c.name       AS city_name,
       c.state_code AS city_state,
       EXISTS (SELECT 1 FROM customers cu WHERE cu.party_id = s.party_id) AS is_customer
FROM suppliers s
JOIN parties p ON p.id = s.party_id
LEFT JOIN cities c ON c.id = p.city_id
WHERE s.party_id = @id;

-- name: ListSuppliers :many
SELECT s.party_id, p.person_type, p.document, p.legal_name, p.trade_name,
       p.phone, p.whatsapp, c.name AS city_name, c.state_code AS city_state,
       s.supplier_type, s.default_purchase_mode, s.status,
       EXISTS (SELECT 1 FROM customers cu WHERE cu.party_id = s.party_id) AS is_customer
FROM suppliers s
JOIN parties p ON p.id = s.party_id
LEFT JOIN cities c ON c.id = p.city_id
WHERE (sqlc.narg('search')::text IS NULL
       OR unaccent(p.legal_name) ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%'
       OR unaccent(p.trade_name) ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%'
       OR p.document LIKE like_escape(upper(sqlc.narg('search'))) || '%')
  AND (sqlc.narg('status')::text IS NULL OR s.status = sqlc.narg('status'))
  AND (sqlc.narg('product_id')::bigint IS NULL OR EXISTS (
        SELECT 1 FROM supplier_products sp
        WHERE sp.supplier_id = s.party_id AND sp.product_id = sqlc.narg('product_id')))
ORDER BY lower(coalesce(p.trade_name, p.legal_name)), s.party_id
LIMIT @limit_rows OFFSET @offset_rows;

-- name: CountSuppliers :one
SELECT count(*)
FROM suppliers s
JOIN parties p ON p.id = s.party_id
WHERE (sqlc.narg('search')::text IS NULL
       OR unaccent(p.legal_name) ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%'
       OR unaccent(p.trade_name) ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%'
       OR p.document LIKE like_escape(upper(sqlc.narg('search'))) || '%')
  AND (sqlc.narg('status')::text IS NULL OR s.status = sqlc.narg('status'))
  AND (sqlc.narg('product_id')::bigint IS NULL OR EXISTS (
        SELECT 1 FROM supplier_products sp
        WHERE sp.supplier_id = s.party_id AND sp.product_id = sqlc.narg('product_id')));

-- name: DeactivateSupplier :execrows
UPDATE suppliers
SET status = 'inactive', blocked_reason = NULL, updated_at = now()
WHERE party_id = @party_id;

-- name: ListPartyContacts :many
SELECT * FROM party_contacts
WHERE party_id = @party_id
ORDER BY position, id;

-- name: UpdatePartyContact :exec
UPDATE party_contacts
SET name = @name, role = @role, phone = @phone, is_whatsapp = @is_whatsapp, email = @email, position = @position
WHERE id = @id;

-- name: DeletePartyContact :exec
DELETE FROM party_contacts WHERE id = @id;

-- name: CreatePartyContact :exec
INSERT INTO party_contacts (party_id, name, role, phone, is_whatsapp, email, position)
VALUES (@party_id, @name, @role, @phone, @is_whatsapp, @email, @position);

-- Pessoa pelo CPF/CNPJ, com os papéis que ela já tem. Usado ao cadastrar um
-- cliente que já é fornecedor (ou o contrário): aproveita o mesmo cadastro.
-- name: GetPartyByDocument :one
SELECT sqlc.embed(p),
       c.name       AS city_name,
       c.state_code AS city_state,
       EXISTS (SELECT 1 FROM suppliers su WHERE su.party_id = p.id) AS is_supplier,
       EXISTS (SELECT 1 FROM customers cu WHERE cu.party_id = p.id) AS is_customer
FROM parties p
LEFT JOIN cities c ON c.id = p.city_id
WHERE p.document = @document;
