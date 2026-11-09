-- Municípios do IBGE (tabela de referência, sem organização).

-- Busca sem acento; quem começa com o texto aparece primeiro.
-- name: SearchCities :many
SELECT id, name, state_code
FROM cities
WHERE (sqlc.narg('state')::text IS NULL OR state_code = sqlc.narg('state'))
  AND unaccent(lower(name)) LIKE '%' || unaccent(lower(@search)) || '%'
ORDER BY unaccent(lower(name)) LIKE unaccent(lower(@search)) || '%' DESC,
         length(name), name
LIMIT @limit_rows;
