-- Cadastro de produtos e suas embalagens.

-- name: CreateProduct :one
INSERT INTO products (
    code, name, variety, grade, category, stock_unit, nominal_weight_kg,
    sells_by_weight, ncm, origin, active, notes
) VALUES (
    @code, @name, @variety, @grade, @category, @stock_unit, @nominal_weight_kg,
    @sells_by_weight, @ncm, @origin, @active, @notes
)
RETURNING id;

-- Só altera se a versão ainda for a que a tela leu (concorrência otimista).
-- name: UpdateProduct :execrows
UPDATE products
SET code = @code, name = @name, variety = @variety, grade = @grade,
    category = @category, stock_unit = @stock_unit,
    nominal_weight_kg = @nominal_weight_kg, sells_by_weight = @sells_by_weight,
    ncm = @ncm, origin = @origin, active = @active, notes = @notes,
    version = version + 1, updated_at = now()
WHERE id = @id AND version = @version;

-- name: GetProduct :one
SELECT * FROM products WHERE id = @id;

-- name: ListProducts :many
SELECT * FROM products
WHERE (sqlc.narg('search')::text IS NULL
       OR unaccent(name || ' ' || coalesce(variety, '') || ' ' || coalesce(grade, ''))
          ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%'
       OR upper(code) LIKE like_escape(upper(sqlc.narg('search'))) || '%')
  AND (sqlc.narg('category')::text IS NULL OR category = sqlc.narg('category'))
  AND (sqlc.narg('active')::boolean IS NULL OR active = sqlc.narg('active'))
ORDER BY lower(name), lower(coalesce(variety, '')), lower(coalesce(grade, '')), id
LIMIT @limit_rows OFFSET @offset_rows;

-- name: CountProducts :one
SELECT count(*) FROM products
WHERE (sqlc.narg('search')::text IS NULL
       OR unaccent(name || ' ' || coalesce(variety, '') || ' ' || coalesce(grade, ''))
          ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%'
       OR upper(code) LIKE like_escape(upper(sqlc.narg('search'))) || '%')
  AND (sqlc.narg('category')::text IS NULL OR category = sqlc.narg('category'))
  AND (sqlc.narg('active')::boolean IS NULL OR active = sqlc.narg('active'));

-- Inativa (não apaga: estoque e vendas vão apontar para o produto) e muda a
-- versão, para quem estiver com a tela aberta não reativar sem perceber.
-- name: DeactivateProduct :execrows
UPDATE products
SET active = false, version = version + 1, updated_at = now()
WHERE id = @id;

-- name: ListProductPackagings :many
SELECT * FROM product_packagings
WHERE product_id = @product_id
ORDER BY position, id;

-- name: CreateProductPackaging :exec
INSERT INTO product_packagings (product_id, name, quantity, position)
VALUES (@product_id, @name, @quantity, @position);

-- name: UpdateProductPackaging :execrows
UPDATE product_packagings
SET name = @name, quantity = @quantity, position = @position
WHERE id = @id AND product_id = @product_id;

-- Apaga as embalagens que saíram da lista (as que ficaram vêm em keep_ids).
-- name: DeleteProductPackagingsExcept :exec
DELETE FROM product_packagings
WHERE product_id = @product_id AND NOT (id = ANY(@keep_ids::bigint[]));
