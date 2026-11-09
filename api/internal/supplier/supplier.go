// Package supplier expõe os endpoints de CRUD de fornecedores usando o Huma.
//
// Como o Huma funciona, resumindo: cada endpoint é uma função Go comum
//
//	func(ctx context.Context, in *AlgumInput) (*AlgumOutput, error)
//
// O Huma lê path/query/header/body para dentro de AlgumInput (usando as struct
// tags), valida, chama a função e serializa AlgumOutput de volta para JSON.
// As mesmas tags geram o documento OpenAPI em /openapi.json.
//
// Um fornecedor é uma pessoa (pacote party: identificação, endereço,
// contatos, pagamento) + os dados de fornecedor (tipo, consignação, prazo...).
package supplier

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/party"
)

// Store é o que os handlers precisam do banco de dados.
// Declarar uma interface pequena do lado de quem USA é Go idiomático
// ("aceite interfaces, retorne structs"): *db.Queries satisfaz essa interface
// em produção, e os testes podem passar uma implementação falsa.
type Store interface {
	party.ContactStore
	CreateParty(ctx context.Context, arg db.CreatePartyParams) (int64, error)
	UpdateParty(ctx context.Context, arg db.UpdatePartyParams) (int64, error)
	CreateSupplier(ctx context.Context, arg db.CreateSupplierParams) error
	UpdateSupplier(ctx context.Context, arg db.UpdateSupplierParams) (int64, error)
	GetSupplier(ctx context.Context, arg db.GetSupplierParams) (db.GetSupplierRow, error)
	ListSuppliers(ctx context.Context, arg db.ListSuppliersParams) ([]db.ListSuppliersRow, error)
	CountSuppliers(ctx context.Context, arg db.CountSuppliersParams) (int64, error)
	DeactivateSupplier(ctx context.Context, arg db.DeactivateSupplierParams) (int64, error)
	ListPartyContacts(ctx context.Context, arg db.ListPartyContactsParams) ([]db.PartyContact, error)
}

// TxFunc abre uma transação presa à organização (com RLS) e entrega um Store
// para usar dentro dela. Em produção é postgres.InTenant; nos testes, um falso.
type TxFunc func(ctx context.Context, organizationID int64, fn func(Store) error) error

// ---------------------------------------------------------------------------
// Tipos da API
// ---------------------------------------------------------------------------

// SupplierFields são os campos editáveis: os da pessoa (embutidos, aparecem "achatados"
// no JSON) + os de fornecedor.
type SupplierFields struct {
	party.Fields

	SupplierType          string   `json:"supplierType,omitempty" enum:"producer,wholesaler,cooperative,importer,other" default:"producer"`
	DefaultPurchaseMode   string   `json:"defaultPurchaseMode,omitempty" enum:"direct,consignment" default:"direct" doc:"direct = buys the goods; consignment = sells first and pays later minus commission"`
	ConsignmentCommission *float64 `json:"consignmentCommission,omitempty" minimum:"0" maximum:"100" doc:"Commission in % kept on consignment sales"`
	PaymentTermDays       int32    `json:"paymentTermDays,omitempty" minimum:"0" maximum:"365" default:"0" doc:"Default payment term in days (0 = cash)"`
	CeasaPavilion         *string  `json:"ceasaPavilion,omitempty" maxLength:"30"`
	CeasaBox              *string  `json:"ceasaBox,omitempty" maxLength:"30"`
	Status                string   `json:"status,omitempty" enum:"active,inactive,blocked" default:"active"`
	BlockedReason         *string  `json:"blockedReason,omitempty" maxLength:"500"`
}

// normalize valida a pessoa e as regras de fornecedor.
func (f *SupplierFields) normalize() []error {
	errs := f.Fields.Normalize()
	fail := func(location, message string) {
		errs = append(errs, &huma.ErrorDetail{Location: "body." + location, Message: message})
	}

	if f.SupplierType == "" {
		f.SupplierType = "producer"
	}
	if f.DefaultPurchaseMode == "" {
		f.DefaultPurchaseMode = "direct"
	}
	if f.Status == "" {
		f.Status = "active"
	}
	f.CeasaPavilion = trimmed(f.CeasaPavilion)
	f.CeasaBox = trimmed(f.CeasaBox)
	f.BlockedReason = trimmed(f.BlockedReason)

	if f.DefaultPurchaseMode == "consignment" && f.ConsignmentCommission == nil {
		fail("consignmentCommission", "commission is required for consignment")
	}
	if f.Status == "blocked" && f.BlockedReason == nil {
		fail("blockedReason", "reason is required when blocked")
	}
	if f.Status != "blocked" {
		f.BlockedReason = nil
	}
	return errs
}

// Supplier é o fornecedor completo, como a API devolve.
type Supplier struct {
	ID int64 `json:"id"`
	party.Party
	SupplierType          string   `json:"supplierType" enum:"producer,wholesaler,cooperative,importer,other"`
	DefaultPurchaseMode   string   `json:"defaultPurchaseMode" enum:"direct,consignment"`
	ConsignmentCommission *float64 `json:"consignmentCommission"`
	PaymentTermDays       int32    `json:"paymentTermDays"`
	CeasaPavilion         *string  `json:"ceasaPavilion"`
	CeasaBox              *string  `json:"ceasaBox"`
	Status                string   `json:"status" enum:"active,inactive,blocked"`
	BlockedReason         *string  `json:"blockedReason"`
	party.Timestamps
}

// SupplierSummary é a versão resumida, usada na listagem.
type SupplierSummary struct {
	ID                  int64       `json:"id"`
	PersonType          string      `json:"personType" enum:"individual,company"`
	Document            *string     `json:"document"`
	LegalName           string      `json:"legalName"`
	TradeName           *string     `json:"tradeName"`
	Phone               *string     `json:"phone"`
	Whatsapp            *string     `json:"whatsapp"`
	City                *party.City `json:"city"`
	SupplierType        string      `json:"supplierType" enum:"producer,wholesaler,cooperative,importer,other"`
	DefaultPurchaseMode string      `json:"defaultPurchaseMode" enum:"direct,consignment"`
	Status              string      `json:"status" enum:"active,inactive,blocked"`
}

// --- Entradas e saídas de cada operação --------------------------------------

type listSuppliersInput struct {
	Search string `query:"search" maxLength:"100" doc:"Name or nickname (contains, accent-insensitive) or document (prefix)"`
	Status string `query:"status" enum:"active,inactive,blocked,all" default:"active"`
	Limit  int32  `query:"limit" minimum:"1" maximum:"100" default:"20"`
	Offset int32  `query:"offset" minimum:"0" default:"0"`
}

type listSuppliersOutput struct {
	Body struct {
		Items  []SupplierSummary `json:"items"`
		Total  int64             `json:"total"`
		Limit  int32             `json:"limit"`
		Offset int32             `json:"offset"`
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
	Body SupplierFields
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
		OperationID: "list-suppliers", Method: http.MethodGet, Path: "/suppliers",
		Summary: "List suppliers", Tags: tags,
	}, h.list)

	huma.Register(api, huma.Operation{
		OperationID: "get-supplier", Method: http.MethodGet, Path: "/suppliers/{id}",
		Summary: "Get a supplier", Tags: tags,
	}, h.get)

	huma.Register(api, huma.Operation{
		OperationID: "create-supplier", Method: http.MethodPost, Path: "/suppliers",
		Summary: "Create a supplier", Tags: tags, DefaultStatus: http.StatusCreated,
	}, h.create)

	huma.Register(api, huma.Operation{
		OperationID: "update-supplier", Method: http.MethodPut, Path: "/suppliers/{id}",
		Summary: "Update a supplier", Tags: tags,
	}, h.update)

	huma.Register(api, huma.Operation{
		OperationID: "deactivate-supplier", Method: http.MethodDelete, Path: "/suppliers/{id}",
		Summary:     "Deactivate a supplier",
		Description: "Soft delete: the supplier is marked inactive, never removed, to preserve history.",
		Tags:        tags, DefaultStatus: http.StatusNoContent,
	}, h.deactivate)
}

// list é um *método* de handler: (h *handler) é o receiver, parecido com o "this".
func (h *handler) list(ctx context.Context, in *listSuppliersInput) (*listSuppliersOutput, error) {
	orgID := auth.OrganizationID(ctx)

	var search *string
	if s := strings.TrimSpace(in.Search); s != "" {
		search = &s
	}
	var status *string // nil = sem filtro ("all")
	if in.Status != "all" {
		status = &in.Status
	}

	var rows []db.ListSuppliersRow
	var total int64
	// A função anônima "enxerga" rows e total de fora (closure) e os preenche.
	err := h.tx(ctx, orgID, func(s Store) error {
		var err error
		rows, err = s.ListSuppliers(ctx, db.ListSuppliersParams{
			OrganizationID: orgID, Search: search, Status: status,
			LimitRows: in.Limit, OffsetRows: in.Offset,
		})
		if err != nil {
			return err
		}
		total, err = s.CountSuppliers(ctx, db.CountSuppliersParams{
			OrganizationID: orgID, Search: search, Status: status,
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	out := &listSuppliersOutput{}
	// make com tamanho 0 para que um resultado vazio vire [] no JSON, e não null.
	out.Body.Items = make([]SupplierSummary, 0, len(rows))
	for _, r := range rows {
		item := SupplierSummary{
			ID: r.PartyID, PersonType: r.PersonType, Document: r.Document, LegalName: r.LegalName,
			TradeName: r.TradeName, Phone: r.Phone, Whatsapp: r.Whatsapp,
			SupplierType: r.SupplierType, DefaultPurchaseMode: r.DefaultPurchaseMode, Status: r.Status,
		}
		if r.CityName != nil && r.CityState != nil {
			item.City = &party.City{Name: *r.CityName, State: *r.CityState}
		}
		out.Body.Items = append(out.Body.Items, item)
	}
	out.Body.Total = total
	out.Body.Limit = in.Limit
	out.Body.Offset = in.Offset
	return out, nil
}

func (h *handler) get(ctx context.Context, in *idInput) (*supplierOutput, error) {
	orgID := auth.OrganizationID(ctx)
	var out Supplier
	err := h.tx(ctx, orgID, func(s Store) error {
		var err error
		out, err = load(ctx, s, orgID, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &supplierOutput{Body: out}, nil
}

// create grava pessoa + fornecedor + contatos na mesma transação: ou tudo, ou nada.
func (h *handler) create(ctx context.Context, in *createInput) (*supplierOutput, error) {
	orgID := auth.OrganizationID(ctx)
	f := &in.Body
	var out Supplier
	err := h.tx(ctx, orgID, func(s Store) error {
		partyID, err := s.CreateParty(ctx, f.CreateParams(orgID))
		if err != nil {
			return err
		}
		err = s.CreateSupplier(ctx, db.CreateSupplierParams{
			PartyID: partyID, OrganizationID: orgID, SupplierType: f.SupplierType,
			DefaultPurchaseMode: f.DefaultPurchaseMode, ConsignmentCommission: f.ConsignmentCommission,
			PaymentTermDays: f.PaymentTermDays, CeasaPavilion: f.CeasaPavilion, CeasaBox: f.CeasaBox,
			Status: f.Status, BlockedReason: f.BlockedReason,
		})
		if err != nil {
			return err
		}
		if err := party.SaveContacts(ctx, s, orgID, partyID, f.Contacts); err != nil {
			return err
		}
		out, err = load(ctx, s, orgID, partyID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &supplierOutput{Body: out}, nil
}

func (h *handler) update(ctx context.Context, in *updateInput) (*supplierOutput, error) {
	orgID := auth.OrganizationID(ctx)
	f := &in.Body
	var out Supplier
	err := h.tx(ctx, orgID, func(s Store) error {
		// Primeiro o fornecedor: se ele não existir (ou for de outra organização),
		// nenhuma linha é alterada e respondemos 404 sem mexer na pessoa.
		n, err := s.UpdateSupplier(ctx, db.UpdateSupplierParams{
			PartyID: in.ID, OrganizationID: orgID, SupplierType: f.SupplierType,
			DefaultPurchaseMode: f.DefaultPurchaseMode, ConsignmentCommission: f.ConsignmentCommission,
			PaymentTermDays: f.PaymentTermDays, CeasaPavilion: f.CeasaPavilion, CeasaBox: f.CeasaBox,
			Status: f.Status, BlockedReason: f.BlockedReason,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return pgx.ErrNoRows
		}
		if _, err := s.UpdateParty(ctx, f.UpdateParams(orgID, in.ID)); err != nil {
			return err
		}
		if err := party.SaveContacts(ctx, s, orgID, in.ID, f.Contacts); err != nil {
			return err
		}
		out, err = load(ctx, s, orgID, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &supplierOutput{Body: out}, nil
}

// deactivate devolve *struct{}: uma saída vazia, ou seja, resposta sem corpo.
func (h *handler) deactivate(ctx context.Context, in *idInput) (*struct{}, error) {
	orgID := auth.OrganizationID(ctx)
	var n int64
	err := h.tx(ctx, orgID, func(s Store) error {
		var err error
		n, err = s.DeactivateSupplier(ctx, db.DeactivateSupplierParams{OrganizationID: orgID, PartyID: in.ID})
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

// load busca o fornecedor completo (com cidade e contatos).
func load(ctx context.Context, s Store, orgID, id int64) (Supplier, error) {
	row, err := s.GetSupplier(ctx, db.GetSupplierParams{OrganizationID: orgID, ID: id})
	if err != nil {
		return Supplier{}, err
	}
	contacts, err := s.ListPartyContacts(ctx, db.ListPartyContactsParams{OrganizationID: orgID, PartyID: id})
	if err != nil {
		return Supplier{}, err
	}
	sup := row.Supplier
	return Supplier{
		ID:                    sup.PartyID,
		Party:                 party.FromDB(row.Party, row.CityName, row.CityState, contacts),
		SupplierType:          sup.SupplierType,
		DefaultPurchaseMode:   sup.DefaultPurchaseMode,
		ConsignmentCommission: sup.ConsignmentCommission,
		PaymentTermDays:       sup.PaymentTermDays,
		CeasaPavilion:         sup.CeasaPavilion,
		CeasaBox:              sup.CeasaBox,
		Status:                sup.Status,
		BlockedReason:         sup.BlockedReason,
		Timestamps:            party.Timestamps{CreatedAt: sup.CreatedAt, UpdatedAt: sup.UpdatedAt},
	}, nil
}

// Códigos de erro do PostgreSQL que viram respostas específicas.
const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

// mapError traduz erros do banco em erros HTTP.
// Qualquer coisa inesperada volta como está e o Huma responde 500.
func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return huma.Error404NotFound("supplier not found")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == uniqueViolation:
			return huma.Error409Conflict("a registration with this document already exists")
		case pgErr.Code == foreignKeyViolation && pgErr.ConstraintName == "parties_city_id_fkey":
			return huma.Error422UnprocessableEntity("validation failed", &huma.ErrorDetail{
				Location: "body.address.cityId", Message: "city not found",
			})
		}
	}
	return err
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
