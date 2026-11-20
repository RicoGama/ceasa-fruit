-- Tipos decimais do sistema, como DOMAINs do PostgreSQL.
--
-- Um domain é um "apelido com regras" para um tipo do banco. Com ele cada
-- coluna diz O QUE guarda (dinheiro, quantidade, percentual), e não só
-- "um NUMERIC qualquer". O sqlc enxerga o nome do domain e gera o tipo Go
-- certo sozinho (ver overrides no sqlc.yaml):
--
--   money_amount    -> money.Money        R$ com 2 casas (até 999 bilhões)
--   quantity_amount -> quantity.Quantity  quantidade com 3 casas (kg até o grama)
--   percent_rate    -> money.Percent      percentual com 4 casas (alíquotas IBS/CBS)
--
-- NUMERIC "puro" continua existindo, mas o sqlc o gera como pgtype.Numeric:
-- é o sinal de que a coluna deveria usar um destes domains.

-- +goose Up
CREATE DOMAIN money_amount AS NUMERIC(14, 2);
CREATE DOMAIN quantity_amount AS NUMERIC(14, 3);
CREATE DOMAIN percent_rate AS NUMERIC(7, 4);

ALTER TABLE suppliers ALTER COLUMN consignment_commission TYPE percent_rate;

-- +goose Down
ALTER TABLE suppliers ALTER COLUMN consignment_commission TYPE NUMERIC(5, 2);
DROP DOMAIN percent_rate;
DROP DOMAIN quantity_amount;
DROP DOMAIN money_amount;
