-- name: GetActiveCertificate :one
SELECT id, holder_name, document, issuer, serial_number, fingerprint, not_before, not_after,
       uploaded_by, created_at
FROM digital_certificates
WHERE active;

-- name: GetActiveCertificateSecret :one
-- O arquivo e a senha (criptografados): só para quem vai assinar.
SELECT id, encrypted_pfx, encrypted_password, key_id, not_after
FROM digital_certificates
WHERE active;

-- name: ListCertificates :many
SELECT c.id, c.active, c.holder_name, c.document, c.issuer, c.serial_number, c.fingerprint,
       c.not_before, c.not_after, c.created_at, c.replaced_at,
       c.uploaded_by, u.name AS uploaded_by_name
FROM digital_certificates c
LEFT JOIN users u ON u.id = c.uploaded_by
ORDER BY c.id DESC
LIMIT 10;

-- name: LockCompany :one
-- Uma troca de certificado por vez: quem chegar depois espera.
SELECT id, document FROM company ORDER BY id LIMIT 1 FOR UPDATE;

-- name: RetireActiveCertificate :execrows
UPDATE digital_certificates
SET active = false, replaced_at = now(),
    encrypted_pfx = NULL, encrypted_password = NULL
WHERE active;

-- name: InsertCertificate :one
INSERT INTO digital_certificates (
    holder_name, document, issuer, serial_number, fingerprint, not_before, not_after,
    encrypted_pfx, encrypted_password, key_id, uploaded_by
) VALUES (
    @holder_name, @document, @issuer, @serial_number, @fingerprint, @not_before, @not_after,
    @encrypted_pfx, @encrypted_password, @key_id, sqlc.narg('uploaded_by')
)
RETURNING id;
