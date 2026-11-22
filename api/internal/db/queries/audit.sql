-- Histórico de alterações (migration 00013).

-- Histórico de um cadastro, do mais novo para o mais antigo.
-- before_id: paginação ("carregar mais" a partir do último que a tela tem).
-- name: ListAuditForEntity :many
SELECT * FROM audit_log
WHERE entity_type = @entity_type AND entity_id = @entity_id
  AND (sqlc.narg('before_id')::bigint IS NULL OR id < sqlc.narg('before_id'))
ORDER BY id DESC
LIMIT @limit_rows;

-- Histórico geral (tela do administrador), com filtros.
-- name: ListAudit :many
SELECT * FROM audit_log
WHERE occurred_at >= @since
  AND (sqlc.narg('actor_user_id')::bigint IS NULL OR actor_user_id = sqlc.narg('actor_user_id'))
  AND (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type'))
  AND (sqlc.narg('before_id')::bigint IS NULL OR id < sqlc.narg('before_id'))
ORDER BY id DESC
LIMIT @limit_rows;

-- Nomes para mostrar no lugar dos ids (quem fez, de qual cadastro, qual produto...).

-- name: AuditUserNames :many
SELECT id, name FROM users WHERE id = ANY (@ids::bigint[]);

-- name: AuditPartyNames :many
SELECT id, coalesce(trade_name, legal_name)::text AS name FROM parties WHERE id = ANY (@ids::bigint[]);

-- name: AuditProductNames :many
SELECT id, concat_ws(' ', name, variety, grade)::text AS name FROM products WHERE id = ANY (@ids::bigint[]);

-- name: AuditRoleNames :many
SELECT id, name FROM roles WHERE id = ANY (@ids::bigint[]);

-- name: AuditDriverNames :many
SELECT id, name FROM drivers WHERE id = ANY (@ids::bigint[]);

-- name: AuditVehicleNames :many
SELECT id, plate AS name FROM vehicles WHERE id = ANY (@ids::bigint[]);

-- name: AuditCompanyNames :many
SELECT id::bigint AS id, coalesce(trade_name, name)::text AS name FROM company WHERE id::bigint = ANY (@ids::bigint[]);

-- name: AuditCityNames :many
SELECT id::bigint AS id, (name || '/' || state_code)::text AS name FROM cities WHERE id::bigint = ANY (@ids::bigint[]);
