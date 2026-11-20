// Package product expõe o cadastro de produtos (legumes, frutas, verduras e
// ovos) e as embalagens de cada um.
//
// Conceitos (ver migration 00005):
//   - um produto é nome + variedade + classificação: "Tomate Italiano Extra AA";
//   - unidade de estoque: o que se conta no box (caixa, saco, maço...);
//   - peso médio (kg) de uma unidade: usado na venda a quilo, já que o box não
//     tem balança (cada lote poderá ter o seu peso médio real);
//   - embalagens: pilha, pallet, caixa de 30... com quantas unidades de estoque
//     cabem; sem quantidade = varia, informada na hora da venda.
//
// Ver: permissão products.view. Cadastrar e editar: products.edit.
package product

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
	"github.com/RicoGama/ceasa-fruit/api/internal/quantity"
)

// ---------------------------------------------------------------------------
// Tipos da API
// ---------------------------------------------------------------------------

// Packaging é uma embalagem do produto (pilha, pallet...).
type Packaging struct {
	ID       int64              `json:"id"`
	Name     string             `json:"name"`
	Quantity *quantity.Quantity `json:"quantity" doc:"Stock units per packaging; null = varies, informed at sale"`
}

// PackagingInput é a embalagem como chega da tela. Sem id = nova.
type PackagingInput struct {
	ID       *int64             `json:"id,omitempty" doc:"Existing packaging id; omit for a new one"`
	Name     string             `json:"name" minLength:"1" maxLength:"40"`
	Quantity *quantity.Quantity `json:"quantity,omitempty" doc:"Stock units per packaging; omit if it varies"`
}

// ProductFields são os campos editáveis do produto.
type ProductFields struct {
	Code            *string            `json:"code,omitempty" maxLength:"20" doc:"Optional short code typed at the counter"`
	Name            string             `json:"name" minLength:"2" maxLength:"80" doc:"e.g. Tomate"`
	Variety         *string            `json:"variety,omitempty" maxLength:"60" doc:"e.g. Italiano"`
	Grade           *string            `json:"grade,omitempty" maxLength:"60" doc:"Classification, e.g. Extra AA"`
	Category        string             `json:"category" enum:"fruit,vegetable,leafy,egg,other"`
	StockUnit       string             `json:"stockUnit" enum:"box,kg,unit,dozen,bag,bunch,tray,pack" doc:"What is counted in stock"`
	NominalWeightKg *quantity.Quantity `json:"nominalWeightKg,omitempty" doc:"Average weight (kg) of one stock unit"`
	SellsByWeight   bool               `json:"sellsByWeight,omitempty" doc:"Can be sold per kg (uses the average weight)"`
	Ncm             *string            `json:"ncm,omitempty" maxLength:"10" doc:"NCM code, 8 digits (dots allowed)"`
	Origin          int16              `json:"origin,omitempty" minimum:"0" maximum:"8" doc:"NF-e goods origin (0 = national)"`
	Active          *bool              `json:"active,omitempty" doc:"Default true"`
	Notes           *string            `json:"notes,omitempty" maxLength:"1000"`
	Packagings      []PackagingInput   `json:"packagings,omitempty" maxItems:"10"`
}

// ProductUpdate são os campos + a versão que a tela leu.
type ProductUpdate struct {
	ProductFields
	Version int32 `json:"version" minimum:"1" doc:"The version read; 409 if someone saved in between"`
}

// Product é o produto completo, como a API devolve.
type Product struct {
	ID              int64              `json:"id"`
	Code            *string            `json:"code"`
	Name            string             `json:"name"`
	Variety         *string            `json:"variety"`
	Grade           *string            `json:"grade"`
	DisplayName     string             `json:"displayName" doc:"Name + variety + grade, e.g. Tomate Italiano Extra AA"`
	Category        string             `json:"category" enum:"fruit,vegetable,leafy,egg,other"`
	StockUnit       string             `json:"stockUnit" enum:"box,kg,unit,dozen,bag,bunch,tray,pack"`
	NominalWeightKg *quantity.Quantity `json:"nominalWeightKg"`
	SellsByWeight   bool               `json:"sellsByWeight"`
	Ncm             *string            `json:"ncm"`
	Origin          int16              `json:"origin"`
	Active          bool               `json:"active"`
	Notes           *string            `json:"notes"`
	Packagings      []Packaging        `json:"packagings"`
	Version         int32              `json:"version"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
}

// ProductSummary é a versão da listagem (sem embalagens).
type ProductSummary struct {
	ID              int64              `json:"id"`
	Code            *string            `json:"code"`
	DisplayName     string             `json:"displayName"`
	Category        string             `json:"category" enum:"fruit,vegetable,leafy,egg,other"`
	StockUnit       string             `json:"stockUnit" enum:"box,kg,unit,dozen,bag,bunch,tray,pack"`
	NominalWeightKg *quantity.Quantity `json:"nominalWeightKg"`
	SellsByWeight   bool               `json:"sellsByWeight"`
	Active          bool               `json:"active"`
}

// --- Entradas e saídas -------------------------------------------------------

type listInput struct {
	Search   string `query:"search" maxLength:"100" doc:"Name, variety or grade (contains, accent-insensitive) or code (prefix)"`
	Category string `query:"category" enum:"fruit,vegetable,leafy,egg,other"`
	Status   string `query:"status" enum:"active,inactive,all" default:"active"`
	Limit    int32  `query:"limit" minimum:"1" maximum:"100" default:"20"`
	Offset   int32  `query:"offset" minimum:"0" default:"0"`
}

type listOutput struct {
	Body struct {
		Items  []ProductSummary `json:"items"`
		Total  int64            `json:"total"`
		Limit  int32            `json:"limit"`
		Offset int32            `json:"offset"`
	}
}

type idInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type createInput struct {
	Body ProductFields
}

func (in *createInput) Resolve(huma.Context) []error { return in.Body.normalize() }

type updateInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body ProductUpdate
}

func (in *updateInput) Resolve(huma.Context) []error { return in.Body.normalize() }

type productOutput struct {
	Body Product
}

// ---------------------------------------------------------------------------
// Validação
// ---------------------------------------------------------------------------

var nonDigits = regexp.MustCompile(`\D`)

// normalize limpa os campos (espaços, NCM só com dígitos) e confere as regras
// que as struct tags não expressam. Devolve os erros no formato do Huma.
func (f *ProductFields) normalize() []error {
	var errs []error
	fail := func(location, message string) {
		errs = append(errs, &huma.ErrorDetail{Location: "body." + location, Message: message})
	}

	f.Name = strings.TrimSpace(f.Name)
	f.Code = upperTrimmed(f.Code)
	f.Variety = trimmed(f.Variety)
	f.Grade = trimmed(f.Grade)
	f.Notes = trimmed(f.Notes)
	if f.Active == nil {
		yes := true
		f.Active = &yes
	}

	if n := trimmed(f.Ncm); n != nil {
		digits := nonDigits.ReplaceAllString(*n, "")
		f.Ncm = &digits
		if len(digits) != 8 {
			fail("ncm", "NCM must have 8 digits")
		}
	} else {
		f.Ncm = nil
	}

	if w := f.NominalWeightKg; w != nil && !w.IsPositive() {
		fail("nominalWeightKg", "weight must be greater than zero")
	}
	if f.StockUnit == "kg" {
		f.NominalWeightKg = nil // a própria unidade já é o quilo
	}
	if f.SellsByWeight && f.StockUnit != "kg" && f.NominalWeightKg == nil {
		fail("nominalWeightKg", "average weight is required to sell by weight")
	}

	seen := map[string]bool{}
	for i := range f.Packagings {
		p := &f.Packagings[i]
		p.Name = strings.TrimSpace(p.Name)
		key := strings.ToLower(p.Name)
		switch {
		case p.Name == "":
			fail(fmt.Sprintf("packagings[%d].name", i), "packaging name is required")
		case seen[key]:
			fail(fmt.Sprintf("packagings[%d].name", i), "duplicate packaging name")
		}
		seen[key] = true
		if p.Quantity != nil && !p.Quantity.IsPositive() {
			fail(fmt.Sprintf("packagings[%d].quantity", i), "quantity must be greater than zero")
		}
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

func upperTrimmed(s *string) *string {
	if v := trimmed(s); v != nil {
		u := strings.ToUpper(*v)
		return &u
	}
	return nil
}

// DisplayName junta nome, variedade e classificação: "Tomate Italiano Extra AA".
func DisplayName(name string, variety, grade *string) string {
	parts := []string{name}
	for _, p := range []*string{variety, grade} {
		if p != nil && *p != "" {
			parts = append(parts, *p)
		}
	}
	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Rotas
// ---------------------------------------------------------------------------

type handler struct{ db postgres.DB }

// Register adiciona as rotas: ver exige products.view; cadastrar, editar e
// inativar exigem products.edit.
func Register(api huma.API, database postgres.DB) {
	h := &handler{db: database}
	tags := []string{"Products"}

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "list-products", Method: http.MethodGet, Path: "/products",
		Summary: "List products", Tags: tags,
	}, auth.ProductsView), h.list)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "get-product", Method: http.MethodGet, Path: "/products/{id}",
		Summary: "Get a product with its packagings", Tags: tags,
	}, auth.ProductsView), h.get)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "create-product", Method: http.MethodPost, Path: "/products",
		Summary: "Create a product", Tags: tags, DefaultStatus: http.StatusCreated,
	}, auth.ProductsEdit), h.create)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "update-product", Method: http.MethodPut, Path: "/products/{id}",
		Summary:     "Update a product",
		Description: "Packagings: items with id are updated, items without id are created, missing ones are removed.",
		Tags:        tags,
	}, auth.ProductsEdit), h.update)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "deactivate-product", Method: http.MethodDelete, Path: "/products/{id}",
		Summary: "Deactivate a product (it is never deleted)", Tags: tags,
		DefaultStatus: http.StatusNoContent,
	}, auth.ProductsEdit), h.deactivate)
}

func (h *handler) list(ctx context.Context, in *listInput) (*listOutput, error) {
	var search, category *string
	if s := strings.TrimSpace(in.Search); s != "" {
		search = &s
	}
	if in.Category != "" {
		category = &in.Category
	}
	var active *bool
	switch in.Status {
	case "active":
		v := true
		active = &v
	case "inactive":
		v := false
		active = &v
	}

	out := &listOutput{}
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		rows, err := tx.ListProducts(ctx, db.ListProductsParams{
			Search: search, Category: category, Active: active,
			LimitRows: in.Limit, OffsetRows: in.Offset,
		})
		if err != nil {
			return err
		}
		out.Body.Total, err = tx.CountProducts(ctx, db.CountProductsParams{
			Search: search, Category: category, Active: active,
		})
		if err != nil {
			return err
		}
		out.Body.Items = make([]ProductSummary, 0, len(rows))
		for _, p := range rows {
			out.Body.Items = append(out.Body.Items, ProductSummary{
				ID: p.ID, Code: p.Code, DisplayName: DisplayName(p.Name, p.Variety, p.Grade),
				Category: p.Category, StockUnit: p.StockUnit, NominalWeightKg: p.NominalWeightKg,
				SellsByWeight: p.SellsByWeight, Active: p.Active,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out.Body.Limit, out.Body.Offset = in.Limit, in.Offset
	return out, nil
}

func (h *handler) get(ctx context.Context, in *idInput) (*productOutput, error) {
	var out Product
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		var err error
		out, err = load(ctx, tx, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &productOutput{Body: out}, nil
}

// create grava o produto e as embalagens na mesma transação.
func (h *handler) create(ctx context.Context, in *createInput) (*productOutput, error) {
	f := &in.Body
	var out Product
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		id, err := tx.CreateProduct(ctx, db.CreateProductParams{
			Code: f.Code, Name: f.Name, Variety: f.Variety, Grade: f.Grade,
			Category: f.Category, StockUnit: f.StockUnit, NominalWeightKg: f.NominalWeightKg,
			SellsByWeight: f.SellsByWeight, Ncm: f.Ncm, Origin: f.Origin,
			Active: *f.Active, Notes: f.Notes,
		})
		if err != nil {
			return err
		}
		if err := savePackagings(ctx, tx, id, f.Packagings); err != nil {
			return err
		}
		out, err = load(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &productOutput{Body: out}, nil
}

func (h *handler) update(ctx context.Context, in *updateInput) (*productOutput, error) {
	f := &in.Body
	var out Product
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		n, err := tx.UpdateProduct(ctx, db.UpdateProductParams{
			ID: in.ID, Version: f.Version,
			Code: f.Code, Name: f.Name, Variety: f.Variety, Grade: f.Grade,
			Category: f.Category, StockUnit: f.StockUnit, NominalWeightKg: f.NominalWeightKg,
			SellsByWeight: f.SellsByWeight, Ncm: f.Ncm, Origin: f.Origin,
			Active: *f.Active, Notes: f.Notes,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			// Não existe (404) ou outra pessoa salvou no meio (409)?
			if _, err := tx.GetProduct(ctx, in.ID); err != nil {
				return err
			}
			return postgres.StaleError()
		}
		if err := savePackagings(ctx, tx, in.ID, f.Packagings); err != nil {
			return err
		}
		out, err = load(ctx, tx, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &productOutput{Body: out}, nil
}

func (h *handler) deactivate(ctx context.Context, in *idInput) (*struct{}, error) {
	var n int64
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		var err error
		n, err = tx.DeactivateProduct(ctx, in.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, huma.Error404NotFound("product not found")
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Funções auxiliares
// ---------------------------------------------------------------------------

// savePackagings deixa as embalagens do produto iguais à lista recebida:
// com id = atualiza, sem id = cria, as que sumiram da lista = apaga.
// (Atualizar em vez de apagar e recriar mantém o id: estoque e vendas vão
// apontar para a embalagem.)
func savePackagings(ctx context.Context, tx postgres.Tx, productID int64, items []PackagingInput) error {
	keep := make([]int64, 0, len(items))
	for _, p := range items {
		if p.ID != nil {
			keep = append(keep, *p.ID)
		}
	}
	// Primeiro apaga as que saíram: assim um nome pode passar de uma
	// embalagem removida para uma nova sem bater no índice único.
	if err := tx.DeleteProductPackagingsExcept(ctx, db.DeleteProductPackagingsExceptParams{
		ProductID: productID, KeepIds: keep,
	}); err != nil {
		return err
	}
	for i, p := range items {
		pos := int16(i)
		if p.ID == nil {
			err := tx.CreateProductPackaging(ctx, db.CreateProductPackagingParams{
				ProductID: productID, Name: p.Name, Quantity: p.Quantity, Position: pos,
			})
			if err != nil {
				return err
			}
			continue
		}
		n, err := tx.UpdateProductPackaging(ctx, db.UpdateProductPackagingParams{
			ID: *p.ID, ProductID: productID, Name: p.Name, Quantity: p.Quantity, Position: pos,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return errUnknownPackaging
		}
	}
	return nil
}

var errUnknownPackaging = errors.New("unknown packaging")

// load busca o produto completo, com as embalagens.
func load(ctx context.Context, tx postgres.Tx, id int64) (Product, error) {
	p, err := tx.GetProduct(ctx, id)
	if err != nil {
		return Product{}, err
	}
	rows, err := tx.ListProductPackagings(ctx, id)
	if err != nil {
		return Product{}, err
	}
	packs := make([]Packaging, 0, len(rows))
	for _, r := range rows {
		packs = append(packs, Packaging{ID: r.ID, Name: r.Name, Quantity: r.Quantity})
	}
	return Product{
		ID: p.ID, Code: p.Code, Name: p.Name, Variety: p.Variety, Grade: p.Grade,
		DisplayName: DisplayName(p.Name, p.Variety, p.Grade),
		Category:    p.Category, StockUnit: p.StockUnit, NominalWeightKg: p.NominalWeightKg,
		SellsByWeight: p.SellsByWeight, Ncm: p.Ncm, Origin: p.Origin, Active: p.Active,
		Notes: p.Notes, Packagings: packs, Version: p.Version,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}, nil
}

// mapError traduz erros do banco em respostas HTTP.
func mapError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return huma.Error404NotFound("product not found")
	case errors.Is(err, errUnknownPackaging):
		return huma.Error422UnprocessableEntity("validation failed",
			&huma.ErrorDetail{Location: "body.packagings", Message: "packaging does not belong to this product"})
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "products_code_key":
			return huma.Error409Conflict("a product with this code already exists")
		case "products_identity_key":
			return huma.Error409Conflict("a product with this name, variety and grade already exists")
		}
	}
	return err
}
