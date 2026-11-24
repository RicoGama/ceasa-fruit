-- Idempotência: o mesmo pedido enviado duas vezes não grava duas vezes.
--
-- O Wi-Fi do box falha: a pessoa clica em "Salvar", o pedido chega e é
-- gravado, mas a resposta se perde. A tela tenta de novo com a MESMA chave
-- (cabeçalho Idempotency-Key) e a API devolve a resposta guardada, sem gravar
-- outra venda, outro pagamento, outra baixa no estoque.
--
-- A chave é gravada DENTRO da transação da gravação de verdade
-- (internal/postgres/tx.go): ou fica tudo, ou nada. Ver internal/idempotency.

-- +goose Up
CREATE TABLE idempotency_keys (
    key           TEXT        PRIMARY KEY CHECK (length(key) BETWEEN 16 AND 100),
    user_id       BIGINT,
    method        TEXT        NOT NULL,
    path          TEXT        NOT NULL,
    request_hash  BYTEA       NOT NULL, -- sha256 de usuário + método + caminho + corpo
    -- Preenchidos quando a resposta é guardada. NULL = em andamento (ou a API
    -- caiu entre gravar e responder: a gravação existe, a resposta não).
    status        SMALLINT,
    content_type  TEXT,
    response_body BYTEA,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at  TIMESTAMPTZ
);
CREATE INDEX idempotency_keys_created_idx ON idempotency_keys (created_at);

-- +goose Down
DROP TABLE idempotency_keys;
