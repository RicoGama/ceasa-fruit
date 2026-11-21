// Package supplierproduct liga fornecedores e produtos: "o que o Sítio do Zé
// fornece" e, pelo outro lado, "quem fornece tomate" (migration 00008).
//
// Rotas:
//
//	GET    /suppliers/{id}/products               produtos do fornecedor    (suppliers.view)
//	POST   /suppliers/{id}/products               ligar um produto          (suppliers.edit)
//	PUT    /suppliers/{id}/products/{productId}   alterar a ligação         (suppliers.edit)
//	DELETE /suppliers/{id}/products/{productId}   desligar                  (suppliers.edit)
//	GET    /products/{id}/suppliers               fornecedores do produto   (products.view E suppliers.view)
package supplierproduct

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/money"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
	"github.com/RicoGama/ceasa-fruit/api/internal/product"
)

// ---------------------------------------------------------------------------
// Tipos da API
// ---------------------------------------------------------------------------

// Fields são os dados editáveis da ligação.
type Fields struct {
	SupplierProductName   *string        `json:"supplierProductName,omitempty" maxLength:"80" doc:"How the supplier calls the product (on their invoice)"`
	Months                []int          `json:"months,omitempty" maxItems:"12" doc:"Months the supplier has it (1-12); empty = all year"`
	PurchaseMode          *string        `json:"purchaseMode,omitempty" enum:"direct,consignment" doc:"Overrides the supplier default for this product"`
	ConsignmentCommission *money.Percent `json:"consignmentCommission,omitempty" doc:"Required when purchaseMode is consignment"`
	Preferred             bool           `json:"preferred,omitempty" doc:"Preferred supplier for this product"`
	Notes                 *string        `json:"notes,omitempty" maxLength:"500"`
}

// normalize limpa e valida o que as tags não cobrem.
func (f *Fields) normalize() []error {
	var errs []error
	fail := func(loc, msg string) {
		errs = append(errs, &huma.ErrorDetail{Location: "body." + loc, Message: msg})
	}
	f.SupplierProductName = trimmed(f.SupplierProductName)
	f.Notes = trimmed(f.Notes)

	for _, m := range f.Months {
		if m < 1 || m > 12 {
			fail("months", "month must be between 1 and 12")
			break
		}
	}
	slices.Sort(f.Months)
	f.Months = slices.Compact(f.Months)
	if len(f.Months) == 12 {
		f.Months = nil // os 12 meses = o ano todo
	}

	consignment := f.PurchaseMode != nil && *f.PurchaseMode == "consignment"
	switch {
	case consignment && f.ConsignmentCommission == nil:
		fail("consignmentCommission", "commission is required for consignment")
	case !consignment:
		f.ConsignmentCommission = nil
	}
	if c := f.ConsignmentCommission; c != nil && !c.Between(0, money.PercentFromInt(100)) {
		fail("consignmentCommission", "commission must be between 0 and 100")
	}
	return errs
}

func trimmed(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

// SupplierProduct é um produto na aba do fornecedor.
type SupplierProduct struct {
	ProductID int64 `json:"productId"`
	Product   struct {
		DisplayName string `json:"displayName"`
		Category    string `json:"category" enum:"fruit,vegetable,leafy,egg,other"`
		StockUnit   string `json:"stockUnit" enum:"box,kg,unit,dozen,bag,bunch,tray,pack"`
		Active      bool   `json:"active"`
	} `json:"product"`
	SupplierProductName   *string        `json:"supplierProductName"`
	Months                []int          `json:"months" doc:"Empty = all year"`
	PurchaseMode          *string        `json:"purchaseMode" enum:"direct,consignment" doc:"null = the supplier default"`
	ConsignmentCommission *money.Percent `json:"consignmentCommission"`
	Preferred             bool           `json:"preferred"`
	Notes                 *string        `json:"notes"`
	Version               int32          `json:"version"`
	UpdatedAt             time.Time      `json:"updatedAt"`
}

// ProductSupplier é um fornecedor na lista do produto.
type ProductSupplier struct {
	SupplierID            int64          `json:"supplierId"`
	LegalName             string         `json:"legalName"`
	TradeName             *string        `json:"tradeName"`
	Phone                 *string        `json:"phone"`
	Whatsapp              *string        `json:"whatsapp"`
	Status                string         `json:"status" enum:"active,inactive,blocked"`
	SupplierProductName   *string        `json:"supplierProductName"`
	Months                []int          `json:"months"`
	PurchaseMode          string         `json:"purchaseMode" enum:"direct,consignment" doc:"Effective mode (own or the supplier default)"`
	ConsignmentCommission *money.Percent `json:"consignmentCommission"`
	Preferred             bool           `json:"preferred"`
}

type supplierInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type linkInput struct {
	ID        int64 `path:"id" minimum:"1"`
	ProductID int64 `path:"productId" minimum:"1"`
}

type createInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		ProductID int64 `json:"productId" minimum:"1"`
		Fields
	}
}

func (in *createInput) Resolve(huma.Context) []error { return in.Body.normalize() }

type updateInput struct {
	ID        int64 `path:"id" minimum:"1"`
	ProductID int64 `path:"productId" minimum:"1"`
	Body      struct {
		Fields
		Version int32 `json:"version" minimum:"1" doc:"The version read; 409 if someone saved in between"`
	}
}

func (in *updateInput) Resolve(huma.Context) []error { return in.Body.normalize() }

type listOutput struct {
	Body struct {
		Items []SupplierProduct `json:"items"`
	}
}

type linkOutput struct{ Body SupplierProduct }

type productSuppliersOutput struct {
	Body struct {
		Items []ProductSupplier `json:"items"`
	}
}

// ---------------------------------------------------------------------------
// Rotas
// ---------------------------------------------------------------------------

type handler struct{ db postgres.DB }

// Register adiciona as rotas (api = grupo de usuários logados).
func Register(api huma.API, database postgres.DB) {
	h := &handler{db: database}
	tags := []string{"Suppliers"}
	view := func(op huma.Operation) huma.Operation { return auth.Protect(api, op, auth.SuppliersView) }
	edit := func(op huma.Operation) huma.Operation { return auth.Protect(api, op, auth.SuppliersEdit) }

	huma.Register(api, view(huma.Operation{
		OperationID: "list-supplier-products", Method: http.MethodGet, Path: "/suppliers/{id}/products",
		Summary: "Products a supplier supplies", Tags: tags,
	}), h.list)

	huma.Register(api, edit(huma.Operation{
		OperationID: "add-supplier-product", Method: http.MethodPost, Path: "/suppliers/{id}/products",
		Summary: "Link a product to a supplier", Tags: tags, DefaultStatus: http.StatusCreated,
	}), h.create)

	huma.Register(api, edit(huma.Operation{
		OperationID: "update-supplier-product", Method: http.MethodPut, Path: "/suppliers/{id}/products/{productId}",
		Summary: "Update a supplier-product link", Tags: tags,
	}), h.update)

	huma.Register(api, edit(huma.Operation{
		OperationID: "remove-supplier-product", Method: http.MethodDelete, Path: "/suppliers/{id}/products/{productId}",
		Summary: "Unlink a product from a supplier", Tags: tags, DefaultStatus: http.StatusNoContent,
	}), h.remove)

	// Ver quem fornece um produto exige ver produtos E ver fornecedores.
	huma.Register(api, auth.Protect(api, auth.Protect(api, huma.Operation{
		OperationID: "list-product-suppliers", Method: http.MethodGet, Path: "/products/{id}/suppliers",
		Summary: "Suppliers of a product (preferred first)", Tags: []string{"Products"},
	}, auth.ProductsView), auth.SuppliersView), h.productSuppliers)
}

func (h *handler) list(ctx context.Context, in *supplierInput) (*listOutput, error) {
	out := &listOutput{}
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		// 404 se o fornecedor não existe (lista vazia seria enganosa).
		if _, err := tx.GetSupplier(ctx, in.ID); err != nil {
			return err
		}
		rows, err := tx.ListSupplierProducts(ctx, in.ID)
		if err != nil {
			return err
		}
		out.Body.Items = make([]SupplierProduct, 0, len(rows))
		for _, r := range rows {
			out.Body.Items = append(out.Body.Items, toAPI(db.GetSupplierProductRow(r)))
		}
		return nil
	})
	if err != nil {
		return nil, mapError(err)
	}
	return out, nil
}

func (h *handler) create(ctx context.Context, in *createInput) (*linkOutput, error) {
	f := in.Body.Fields
	var out SupplierProduct
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		if _, err := tx.GetSupplier(ctx, in.ID); err != nil {
			return err
		}
		p, err := tx.GetProduct(ctx, in.Body.ProductID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errProductNotFound
		}
		if err != nil {
			return err
		}
		if !p.Active {
			return errProductInactive
		}
		err = tx.CreateSupplierProduct(ctx, db.CreateSupplierProductParams{
			SupplierID: in.ID, ProductID: in.Body.ProductID,
			SupplierProductName: f.SupplierProductName, Months: toInt16(f.Months),
			PurchaseMode: f.PurchaseMode, ConsignmentCommission: f.ConsignmentCommission,
			Preferred: f.Preferred, Notes: f.Notes,
		})
		if err != nil {
			return err
		}
		out, err = load(ctx, tx, in.ID, in.Body.ProductID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &linkOutput{Body: out}, nil
}

func (h *handler) update(ctx context.Context, in *updateInput) (*linkOutput, error) {
	f := in.Body.Fields
	var out SupplierProduct
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		n, err := tx.UpdateSupplierProduct(ctx, db.UpdateSupplierProductParams{
			SupplierID: in.ID, ProductID: in.ProductID, Version: in.Body.Version,
			SupplierProductName: f.SupplierProductName, Months: toInt16(f.Months),
			PurchaseMode: f.PurchaseMode, ConsignmentCommission: f.ConsignmentCommission,
			Preferred: f.Preferred, Notes: f.Notes,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			if _, err := load(ctx, tx, in.ID, in.ProductID); err != nil {
				return err // não existe: 404
			}
			return postgres.StaleError()
		}
		out, err = load(ctx, tx, in.ID, in.ProductID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &linkOutput{Body: out}, nil
}

func (h *handler) remove(ctx context.Context, in *linkInput) (*struct{}, error) {
	var n int64
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		var err error
		n, err = tx.DeleteSupplierProduct(ctx, db.DeleteSupplierProductParams{SupplierID: in.ID, ProductID: in.ProductID})
		return err
	})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, huma.Error404NotFound("product is not linked to this supplier")
	}
	return nil, nil
}

func (h *handler) productSuppliers(ctx context.Context, in *supplierInput) (*productSuppliersOutput, error) {
	out := &productSuppliersOutput{}
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		if _, err := tx.GetProduct(ctx, in.ID); err != nil {
			return err
		}
		rows, err := tx.ListProductSuppliers(ctx, in.ID)
		if err != nil {
			return err
		}
		out.Body.Items = make([]ProductSupplier, 0, len(rows))
		for _, r := range rows {
			// Sem modalidade própria, vale a do fornecedor (com a comissão dele).
			mode, commission := r.DefaultPurchaseMode, r.DefaultCommission
			if r.PurchaseMode != nil {
				mode, commission = *r.PurchaseMode, r.ConsignmentCommission
			}
			if mode != "consignment" {
				commission = nil
			}
			out.Body.Items = append(out.Body.Items, ProductSupplier{
				SupplierID: r.SupplierID, LegalName: r.LegalName, TradeName: r.TradeName,
				Phone: r.Phone, Whatsapp: r.Whatsapp, Status: r.Status,
				SupplierProductName: r.SupplierProductName, Months: toInt(r.Months),
				PurchaseMode: mode, ConsignmentCommission: commission, Preferred: r.Preferred,
			})
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, huma.Error404NotFound("product not found")
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------------

func load(ctx context.Context, tx postgres.Tx, supplierID, productID int64) (SupplierProduct, error) {
	r, err := tx.GetSupplierProduct(ctx, db.GetSupplierProductParams{SupplierID: supplierID, ProductID: productID})
	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierProduct{}, errLinkNotFound
	}
	if err != nil {
		return SupplierProduct{}, err
	}
	return toAPI(r), nil
}

func toAPI(r db.GetSupplierProductRow) SupplierProduct {
	out := SupplierProduct{
		ProductID: r.ProductID, SupplierProductName: r.SupplierProductName, Months: toInt(r.Months),
		PurchaseMode: r.PurchaseMode, ConsignmentCommission: r.ConsignmentCommission,
		Preferred: r.Preferred, Notes: r.Notes, Version: r.Version, UpdatedAt: r.UpdatedAt,
	}
	out.Product.DisplayName = product.DisplayName(r.Name, r.Variety, r.Grade)
	out.Product.Category, out.Product.StockUnit, out.Product.Active = r.Category, r.StockUnit, r.ProductActive
	return out
}

func toInt16(ms []int) []int16 {
	out := make([]int16, len(ms))
	for i, m := range ms {
		out[i] = int16(m)
	}
	return out
}

func toInt(ms []int16) []int {
	out := make([]int, len(ms))
	for i, m := range ms {
		out[i] = int(m)
	}
	return out
}

var (
	errProductNotFound = errors.New("product not found")
	errProductInactive = errors.New("product inactive")
	errLinkNotFound    = errors.New("link not found")
)

func mapError(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errLinkNotFound):
		return huma.Error404NotFound("product is not linked to this supplier")
	case errors.Is(err, errProductNotFound):
		return huma.Error422UnprocessableEntity("validation failed",
			&huma.ErrorDetail{Location: "body.productId", Message: "product not found"})
	case errors.Is(err, errProductInactive):
		return huma.Error422UnprocessableEntity("validation failed",
			&huma.ErrorDetail{Location: "body.productId", Message: "product is inactive"})
	case errors.Is(err, pgx.ErrNoRows):
		return huma.Error404NotFound("supplier not found")
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return huma.Error409Conflict("this product is already linked to this supplier")
	}
	return err
}
