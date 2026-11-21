-- Fornecedor × produto (ver migration 00008).

-- name: ListSupplierProducts :many
SELECT sp.*, p.name, p.variety, p.grade, p.category, p.stock_unit, p.active AS product_active
FROM supplier_products sp
JOIN products p ON p.id = sp.product_id
WHERE sp.supplier_id = @supplier_id
ORDER BY sp.preferred DESC, lower(p.name), lower(coalesce(p.variety, '')), lower(coalesce(p.grade, ''));

-- name: GetSupplierProduct :one
SELECT sp.*, p.name, p.variety, p.grade, p.category, p.stock_unit, p.active AS product_active
FROM supplier_products sp
JOIN products p ON p.id = sp.product_id
WHERE sp.supplier_id = @supplier_id AND sp.product_id = @product_id;

-- name: CreateSupplierProduct :exec
INSERT INTO supplier_products (
    supplier_id, product_id, supplier_product_name, months,
    purchase_mode, consignment_commission, preferred, notes
) VALUES (
    @supplier_id, @product_id, @supplier_product_name, @months,
    @purchase_mode, @consignment_commission, @preferred, @notes
);

-- Só altera se a versão ainda for a que a tela leu (concorrência otimista).
-- name: UpdateSupplierProduct :execrows
UPDATE supplier_products
SET supplier_product_name = @supplier_product_name, months = @months,
    purchase_mode = @purchase_mode, consignment_commission = @consignment_commission,
    preferred = @preferred, notes = @notes,
    version = version + 1, updated_at = now()
WHERE supplier_id = @supplier_id AND product_id = @product_id AND version = @version;

-- name: DeleteSupplierProduct :execrows
DELETE FROM supplier_products
WHERE supplier_id = @supplier_id AND product_id = @product_id;

-- Quem fornece este produto (preferidos primeiro).
-- name: ListProductSuppliers :many
SELECT sp.supplier_id, sp.supplier_product_name, sp.months, sp.purchase_mode,
       sp.consignment_commission, sp.preferred,
       pa.legal_name, pa.trade_name, pa.phone, pa.whatsapp,
       s.status, s.default_purchase_mode,
       s.consignment_commission AS default_commission
FROM supplier_products sp
JOIN suppliers s ON s.party_id = sp.supplier_id
JOIN parties pa ON pa.id = sp.supplier_id
WHERE sp.product_id = @product_id
ORDER BY sp.preferred DESC, s.status = 'active' DESC, lower(coalesce(pa.trade_name, pa.legal_name));
