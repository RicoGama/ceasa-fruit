-- +goose Up
-- like_escape faz "%" e "_" digitados na busca valerem como texto, e não como
-- coringa do LIKE ("qualquer coisa" / "qualquer letra"). Não é questão de
-- SQL injection (a busca já vai como parâmetro): é só para "10%" procurar
-- "10%" mesmo. A barra invertida é o caractere de escape padrão do LIKE.
CREATE FUNCTION like_escape(s text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    RETURN replace(replace(replace(s, '\', '\\'), '%', '\%'), '_', '\_');

-- +goose Down
DROP FUNCTION like_escape(text);
