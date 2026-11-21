-- O que cada fornecedor fornece (fornecedor × produto, muitos para muitos).
--
-- A linha guarda o que o comprador sabe de cabeça sobre a dupla:
--   - o nome que o fornecedor usa para o produto (para conferir a nota dele);
--   - os meses em que ele tem (safra); vazio = o ano todo;
--   - uma modalidade própria quando difere do padrão do fornecedor (ex.: só o
--     tomate é consignado), com a comissão;
--   - se é o fornecedor preferido daquele produto, e observações.
-- O "último preço pago" virá das entradas de mercadoria, não daqui.

-- +goose Up
CREATE TABLE supplier_products (
    supplier_id            BIGINT NOT NULL REFERENCES suppliers (party_id) ON DELETE CASCADE,
    product_id             BIGINT NOT NULL REFERENCES products (id),
    supplier_product_name  TEXT,
    months                 SMALLINT[] NOT NULL DEFAULT '{}'
                           CHECK (months <@ ARRAY[1,2,3,4,5,6,7,8,9,10,11,12]::smallint[]),
    purchase_mode          TEXT CHECK (purchase_mode IN ('direct', 'consignment')),
    consignment_commission percent_rate CHECK (consignment_commission BETWEEN 0 AND 100),
    preferred              BOOLEAN NOT NULL DEFAULT false,
    notes                  TEXT,
    version                INTEGER NOT NULL DEFAULT 1,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (supplier_id, product_id),
    -- Comissão só faz sentido com consignação própria.
    CHECK (purchase_mode = 'consignment' OR consignment_commission IS NULL)
);
-- "Quem fornece este produto?" (caminho inverso, muito usado na compra).
CREATE INDEX supplier_products_product_idx ON supplier_products (product_id);

-- Tempo real: quem está no fornecedor vê a aba mudar; quem está no produto vê
-- a lista de fornecedores dele mudar.
CREATE TRIGGER supplier_products_notify AFTER INSERT OR UPDATE OR DELETE ON supplier_products
    FOR EACH ROW EXECUTE FUNCTION notify_change('supplier_products', 'supplier_id');
CREATE TRIGGER product_suppliers_notify AFTER INSERT OR UPDATE OR DELETE ON supplier_products
    FOR EACH ROW EXECUTE FUNCTION notify_change('product_suppliers', 'product_id');

-- +goose Down
DROP TABLE supplier_products;
