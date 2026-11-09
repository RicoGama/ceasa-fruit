// Package supplier exposes the supplier CRUD endpoints using Huma.
//
// How Huma works, in short: every endpoint is a plain Go function
//
//	func(ctx context.Context, in *SomeInput) (*SomeOutput, error)
//
// Huma reads path/query/header/body into SomeInput (using struct tags),
// validates it, calls the function, and serializes SomeOutput back to JSON.
// The same tags are used to generate the OpenAPI document at /openapi.json.
package supplier

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/document"
)

// Store is what the handlers need from the database.
// Declaring a small interface on the consumer side is idiomatic Go
// ("accept interfaces, return structs"): *db.Queries satisfies it in
// production, and tests can pass a fake implementation.
type Store interface {
	CreateSupplier(ctx context.Context, arg db.CreateSupplierParams) (db.Supplier, error)
	GetSupplier(ctx context.Context, id int64) (db.Supplier, error)
	ListSuppliers(ctx context.Context, arg db.ListSuppliersParams) ([]db.Supplier, error)
	CountSuppliers(ctx context.Context, arg db.CountSuppliersParams) (int64, error)
	UpdateSupplier(ctx context.Context, arg db.UpdateSupplierParams) (db.Supplier, error)
	DeactivateSupplier(ctx context.Context, id int64) (int64, error)
}

// ---------------------------------------------------------------------------
// API types (what goes over the wire). Kept separate from db.Supplier so the
// database schema and the public JSON contract can evolve independently.
// ---------------------------------------------------------------------------

// Supplier is the JSON representation returned by the API.
type Supplier struct {
	ID        int64     `json:"id" example:"1"`
	Name      string    `json:"name" example:"Sítio Boa Colheita"`
	Document  string    `json:"document" example:"11222333000181" doc:"CPF or CNPJ, digits/letters only"`
	Phone     *string   `json:"phone" example:"11999990000"`
	Email     *string   `json:"email" example:"contato@boacolheita.com.br"`
	City      *string   `json:"city" example:"Mogi das Cruzes"`
	State     *string   `json:"state" example:"SP"`
	Notes     *string   `json:"notes"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SupplierFields are the editable fields, shared by create and update.
//
// Tag cheat sheet:
//   - json:"x,omitempty"  -> field is optional in the request
//   - minLength/maxLength/pattern/format -> validated by Huma automatically
//   - doc/example         -> show up in the generated docs
type SupplierFields struct {
	Name     string  `json:"name" minLength:"2" maxLength:"120"`
	Document string  `json:"document" maxLength:"18" doc:"CPF or CNPJ (numeric or alphanumeric), with or without mask" example:"11.222.333/0001-81"`
	Phone    *string `json:"phone,omitempty" maxLength:"20"`
	Email    *string `json:"email,omitempty" format:"email" maxLength:"120"`
	City     *string `json:"city,omitempty" maxLength:"80"`
	State    *string `json:"state,omitempty" pattern:"^[A-Za-z]{2}$" doc:"Two-letter state code (UF)" example:"SP"`
	Notes    *string `json:"notes,omitempty" maxLength:"1000"`
}

// normalize cleans the input before it hits the database and returns
// validation errors that struct tags cannot express (CPF/CNPJ check digits).
func (f *SupplierFields) normalize() []error {
	f.Name = strings.TrimSpace(f.Name)
	f.Document = document.Normalize(f.Document)
	f.Phone = cleanOptional(f.Phone)
	f.Email = cleanOptional(f.Email)
	f.City = cleanOptional(f.City)
	f.Notes = cleanOptional(f.Notes)
	if s := cleanOptional(f.State); s != nil {
		upper := strings.ToUpper(*s)
		f.State = &upper
	} else {
		f.State = nil
	}

	if !document.IsValid(f.Document) {
		return []error{&huma.ErrorDetail{
			Location: "body.document",
			Message:  "invalid CPF or CNPJ",
			Value:    f.Document,
		}}
	}
	return nil
}

// cleanOptional trims spaces and turns "" into nil (stored as NULL).
func cleanOptional(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

// --- Inputs and outputs for each operation -----------------------------------

type listSuppliersInput struct {
	Search string `query:"search" maxLength:"100" doc:"Filter by name (contains, accent-insensitive) or document (prefix)"`
	Status string `query:"status" enum:"active,inactive,all" default:"active"`
	Limit  int32  `query:"limit" minimum:"1" maximum:"100" default:"20"`
	Offset int32  `query:"offset" minimum:"0" default:"0"`
}

type listSuppliersOutput struct {
	Body struct {
		Items  []Supplier `json:"items"`
		Total  int64      `json:"total"`
		Limit  int32      `json:"limit"`
		Offset int32      `json:"offset"`
	}
}

type idInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type supplierOutput struct {
	Body Supplier
}

type createInput struct {
	Body SupplierFields
}

// Resolve is called by Huma after tag validation (it implements huma.Resolver).
// The pointer receiver (*createInput) lets us modify the input in place.
func (in *createInput) Resolve(huma.Context) []error {
	return in.Body.normalize()
}

type updateInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		SupplierFields      // embedded struct: its fields are "promoted" (flattened in JSON)
		Active         bool `json:"active"`
	}
}

func (in *updateInput) Resolve(huma.Context) []error {
	return in.Body.normalize()
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

type handler struct {
	store Store
}

// Register adds all supplier routes to the Huma API.
func Register(api huma.API, store Store) {
	h := &handler{store: store}
	tags := []string{"Suppliers"}

	huma.Register(api, huma.Operation{
		OperationID: "list-suppliers",
		Method:      http.MethodGet,
		Path:        "/suppliers",
		Summary:     "List suppliers",
		Tags:        tags,
	}, h.list)

	huma.Register(api, huma.Operation{
		OperationID: "get-supplier",
		Method:      http.MethodGet,
		Path:        "/suppliers/{id}",
		Summary:     "Get a supplier",
		Tags:        tags,
	}, h.get)

	huma.Register(api, huma.Operation{
		OperationID:   "create-supplier",
		Method:        http.MethodPost,
		Path:          "/suppliers",
		Summary:       "Create a supplier",
		Tags:          tags,
		DefaultStatus: http.StatusCreated,
	}, h.create)

	huma.Register(api, huma.Operation{
		OperationID: "update-supplier",
		Method:      http.MethodPut,
		Path:        "/suppliers/{id}",
		Summary:     "Update a supplier",
		Tags:        tags,
	}, h.update)

	huma.Register(api, huma.Operation{
		OperationID:   "deactivate-supplier",
		Method:        http.MethodDelete,
		Path:          "/suppliers/{id}",
		Summary:       "Deactivate a supplier",
		Description:   "Soft delete: the supplier is marked inactive, never removed, to preserve history.",
		Tags:          tags,
		DefaultStatus: http.StatusNoContent,
	}, h.deactivate)
}

// list is a *method* on handler: (h *handler) is the receiver, like "this".
func (h *handler) list(ctx context.Context, in *listSuppliersInput) (*listSuppliersOutput, error) {
	var search *string
	if s := strings.TrimSpace(in.Search); s != "" {
		search = &s
	}

	var active *bool // nil = no filter ("all")
	switch in.Status {
	case "active":
		active = new(true) // Go 1.26+: new(expr) returns a pointer to the value
	case "inactive":
		active = new(false)
	}

	rows, err := h.store.ListSuppliers(ctx, db.ListSuppliersParams{
		Search: search, Active: active, Limit: in.Limit, Offset: in.Offset,
	})
	if err != nil {
		return nil, err
	}
	total, err := h.store.CountSuppliers(ctx, db.CountSuppliersParams{Search: search, Active: active})
	if err != nil {
		return nil, err
	}

	out := &listSuppliersOutput{}
	// make with length 0 so an empty result is encoded as [] instead of null.
	out.Body.Items = make([]Supplier, 0, len(rows))
	for _, r := range rows {
		out.Body.Items = append(out.Body.Items, toAPI(r))
	}
	out.Body.Total = total
	out.Body.Limit = in.Limit
	out.Body.Offset = in.Offset
	return out, nil
}

func (h *handler) get(ctx context.Context, in *idInput) (*supplierOutput, error) {
	s, err := h.store.GetSupplier(ctx, in.ID)
	if err != nil {
		return nil, mapError(err)
	}
	return &supplierOutput{Body: toAPI(s)}, nil
}

func (h *handler) create(ctx context.Context, in *createInput) (*supplierOutput, error) {
	f := in.Body
	s, err := h.store.CreateSupplier(ctx, db.CreateSupplierParams{
		Name: f.Name, Document: f.Document, Phone: f.Phone, Email: f.Email,
		City: f.City, State: f.State, Notes: f.Notes,
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &supplierOutput{Body: toAPI(s)}, nil
}

func (h *handler) update(ctx context.Context, in *updateInput) (*supplierOutput, error) {
	f := in.Body
	s, err := h.store.UpdateSupplier(ctx, db.UpdateSupplierParams{
		ID: in.ID, Name: f.Name, Document: f.Document, Phone: f.Phone, Email: f.Email,
		City: f.City, State: f.State, Notes: f.Notes, Active: f.Active,
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &supplierOutput{Body: toAPI(s)}, nil
}

// deactivate returns *struct{}: an empty output, i.e. no response body.
func (h *handler) deactivate(ctx context.Context, in *idInput) (*struct{}, error) {
	n, err := h.store.DeactivateSupplier(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, huma.Error404NotFound("supplier not found")
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// uniqueViolation is the PostgreSQL error code for a UNIQUE constraint failure.
const uniqueViolation = "23505"

// mapError translates database errors into HTTP errors.
// Anything unexpected is returned as-is and Huma answers 500.
func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return huma.Error404NotFound("supplier not found")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return huma.Error409Conflict("a supplier with this document already exists")
	}
	return err
}

func toAPI(s db.Supplier) Supplier {
	return Supplier{
		ID: s.ID, Name: s.Name, Document: s.Document, Phone: s.Phone, Email: s.Email,
		City: s.City, State: s.State, Notes: s.Notes, Active: s.Active,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}
