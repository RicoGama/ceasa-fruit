// Package supplier expõe os endpoints de CRUD de fornecedores usando o Huma.
//
// Como o Huma funciona, resumindo: cada endpoint é uma função Go comum
//
//	func(ctx context.Context, in *AlgumInput) (*AlgumOutput, error)
//
// O Huma lê path/query/header/body para dentro de AlgumInput (usando as struct
// tags), valida, chama a função e serializa AlgumOutput de volta para JSON.
// As mesmas tags geram o documento OpenAPI em /openapi.json.
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

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/document"
)

// Store é o que os handlers precisam do banco de dados.
// Declarar uma interface pequena do lado de quem USA é Go idiomático
// ("aceite interfaces, retorne structs"): *db.Queries satisfaz essa interface
// em produção, e os testes podem passar uma implementação falsa.
type Store interface {
	CreateSupplier(ctx context.Context, arg db.CreateSupplierParams) (db.Supplier, error)
	GetSupplier(ctx context.Context, arg db.GetSupplierParams) (db.Supplier, error)
	ListSuppliers(ctx context.Context, arg db.ListSuppliersParams) ([]db.Supplier, error)
	CountSuppliers(ctx context.Context, arg db.CountSuppliersParams) (int64, error)
	UpdateSupplier(ctx context.Context, arg db.UpdateSupplierParams) (db.Supplier, error)
	DeactivateSupplier(ctx context.Context, arg db.DeactivateSupplierParams) (int64, error)
}

// TxFunc abre uma transação presa à organização (com RLS) e entrega um Store
// para usar dentro dela. Em produção é postgres.InTenant; nos testes, um falso.
type TxFunc func(ctx context.Context, organizationID int64, fn func(Store) error) error

// ---------------------------------------------------------------------------
// Tipos da API (o que trafega no HTTP). Ficam separados de db.Supplier para
// que o schema do banco e o contrato JSON público possam evoluir independentes.
// ---------------------------------------------------------------------------

// Supplier é a representação JSON devolvida pela API.
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

// SupplierFields são os campos editáveis, compartilhados entre criar e atualizar.
//
// Colinha das tags:
//   - json:"x,omitempty"  -> campo opcional na requisição
//   - minLength/maxLength/pattern/format -> o Huma valida automaticamente
//   - doc/example         -> aparecem na documentação gerada
type SupplierFields struct {
	Name     string  `json:"name" minLength:"2" maxLength:"120"`
	Document string  `json:"document" maxLength:"18" doc:"CPF or CNPJ (numeric or alphanumeric), with or without mask" example:"11.222.333/0001-81"`
	Phone    *string `json:"phone,omitempty" maxLength:"20"`
	Email    *string `json:"email,omitempty" format:"email" maxLength:"120"`
	City     *string `json:"city,omitempty" maxLength:"80"`
	State    *string `json:"state,omitempty" pattern:"^[A-Za-z]{2}$" doc:"Two-letter state code (UF)" example:"SP"`
	Notes    *string `json:"notes,omitempty" maxLength:"1000"`
}

// normalize limpa a entrada antes de ir para o banco e devolve os erros de
// validação que as struct tags não conseguem expressar (dígitos do CPF/CNPJ).
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

// cleanOptional tira espaços das pontas e transforma "" em nil (gravado como NULL).
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

// --- Entradas e saídas de cada operação --------------------------------------

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

// Resolve é chamado pelo Huma depois da validação das tags (implementa huma.Resolver).
// O receiver ponteiro (*createInput) permite alterar a entrada no lugar.
func (in *createInput) Resolve(huma.Context) []error {
	return in.Body.normalize()
}

type updateInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		SupplierFields      // struct embutida: os campos dela são "promovidos" (achatados no JSON)
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
	tx TxFunc
}

// Register adiciona todas as rotas de fornecedor na API do Huma.
// A api recebida deve ser um grupo protegido por auth.RequireOrganization.
func Register(api huma.API, tx TxFunc) {
	h := &handler{tx: tx}
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

// list é um *método* de handler: (h *handler) é o receiver, parecido com o "this".
func (h *handler) list(ctx context.Context, in *listSuppliersInput) (*listSuppliersOutput, error) {
	var search *string
	if s := strings.TrimSpace(in.Search); s != "" {
		search = &s
	}

	var active *bool // nil = sem filtro ("all")
	switch in.Status {
	case "active":
		active = new(true) // Go 1.26+: new(expr) devolve um ponteiro para o valor
	case "inactive":
		active = new(false)
	}

	orgID := auth.OrganizationID(ctx)

	var rows []db.Supplier
	var total int64
	// A função anônima "enxerga" rows e total de fora (closure) e os preenche.
	err := h.tx(ctx, orgID, func(s Store) error {
		var err error
		rows, err = s.ListSuppliers(ctx, db.ListSuppliersParams{
			OrganizationID: orgID, Search: search, Active: active,
			LimitRows: in.Limit, OffsetRows: in.Offset,
		})
		if err != nil {
			return err
		}
		total, err = s.CountSuppliers(ctx, db.CountSuppliersParams{
			OrganizationID: orgID, Search: search, Active: active,
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	out := &listSuppliersOutput{}
	// make com tamanho 0 para que um resultado vazio vire [] no JSON, e não null.
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
	orgID := auth.OrganizationID(ctx)
	var row db.Supplier
	err := h.tx(ctx, orgID, func(s Store) error {
		var err error
		row, err = s.GetSupplier(ctx, db.GetSupplierParams{OrganizationID: orgID, ID: in.ID})
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &supplierOutput{Body: toAPI(row)}, nil
}

func (h *handler) create(ctx context.Context, in *createInput) (*supplierOutput, error) {
	orgID := auth.OrganizationID(ctx)
	f := in.Body
	var row db.Supplier
	err := h.tx(ctx, orgID, func(s Store) error {
		var err error
		row, err = s.CreateSupplier(ctx, db.CreateSupplierParams{
			OrganizationID: orgID, Name: f.Name, Document: f.Document, Phone: f.Phone,
			Email: f.Email, City: f.City, State: f.State, Notes: f.Notes,
		})
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &supplierOutput{Body: toAPI(row)}, nil
}

func (h *handler) update(ctx context.Context, in *updateInput) (*supplierOutput, error) {
	orgID := auth.OrganizationID(ctx)
	f := in.Body
	var row db.Supplier
	err := h.tx(ctx, orgID, func(s Store) error {
		var err error
		row, err = s.UpdateSupplier(ctx, db.UpdateSupplierParams{
			OrganizationID: orgID, ID: in.ID, Name: f.Name, Document: f.Document,
			Phone: f.Phone, Email: f.Email, City: f.City, State: f.State,
			Notes: f.Notes, Active: f.Active,
		})
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &supplierOutput{Body: toAPI(row)}, nil
}

// deactivate devolve *struct{}: uma saída vazia, ou seja, resposta sem corpo.
func (h *handler) deactivate(ctx context.Context, in *idInput) (*struct{}, error) {
	orgID := auth.OrganizationID(ctx)
	var n int64
	err := h.tx(ctx, orgID, func(s Store) error {
		var err error
		n, err = s.DeactivateSupplier(ctx, db.DeactivateSupplierParams{OrganizationID: orgID, ID: in.ID})
		return err
	})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, huma.Error404NotFound("supplier not found")
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Funções auxiliares
// ---------------------------------------------------------------------------

// uniqueViolation é o código de erro do PostgreSQL quando uma constraint UNIQUE falha.
const uniqueViolation = "23505"

// mapError traduz erros do banco em erros HTTP.
// Qualquer coisa inesperada volta como está e o Huma responde 500.
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
