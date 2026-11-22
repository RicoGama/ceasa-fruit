// Package customer expõe os endpoints de CRUD de clientes.
//
// Segue o mesmo desenho do pacote supplier (vale ler os comentários de lá):
// um cliente é uma pessoa (pacote party: identificação, endereço, contatos,
// pagamento) + os dados de cliente (tipo, box na CEASA, situação).
//
// O mesmo CNPJ pode ser fornecedor e cliente. Nesse caso os dois papéis
// apontam para a MESMA pessoa: o cadastro de cliente pode nascer em cima de um
// fornecedor (party.Existing) e editar o nome aqui muda lá também.
package customer

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
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// Store é o que os handlers precisam do banco.
type Store interface {
	party.Store
	BumpPartyVersion(ctx context.Context, id int64) error
	CreateCustomer(ctx context.Context, arg db.CreateCustomerParams) error
	UpdateCustomer(ctx context.Context, arg db.UpdateCustomerParams) (int64, error)
	GetCustomer(ctx context.Context, id int64) (db.GetCustomerRow, error)
	ListCustomers(ctx context.Context, arg db.ListCustomersParams) ([]db.ListCustomersRow, error)
	CountCustomers(ctx context.Context, arg db.CountCustomersParams) (int64, error)
	DeactivateCustomer(ctx context.Context, partyID int64) (int64, error)
	ListPartyContacts(ctx context.Context, partyID int64) ([]db.PartyContact, error)
}

// TxFunc abre uma transação e entrega um Store para usar dentro dela.
type TxFunc func(ctx context.Context, fn func(Store) error) error

// ---------------------------------------------------------------------------
// Tipos da API
// ---------------------------------------------------------------------------

// CustomerFields são os campos editáveis: os da pessoa + os de cliente.
type CustomerFields struct {
	party.Fields

	CustomerType  string  `json:"customerType,omitempty" enum:"retailer,supermarket,restaurant,street_vendor,wholesaler,institutional,consumer,other" default:"retailer"`
	CeasaPavilion *string `json:"ceasaPavilion,omitempty" maxLength:"30"`
	CeasaBox      *string `json:"ceasaBox,omitempty" maxLength:"30"`
	Status        string  `json:"status,omitempty" enum:"active,inactive,blocked" default:"active"`
	BlockedReason *string `json:"blockedReason,omitempty" maxLength:"500"`
}

func (f *CustomerFields) normalize() []error {
	errs := f.Fields.Normalize()
	if f.CustomerType == "" {
		f.CustomerType = "retailer"
	}
	if f.Status == "" {
		f.Status = "active"
	}
	f.CeasaPavilion = trimmed(f.CeasaPavilion)
	f.CeasaBox = trimmed(f.CeasaBox)
	f.BlockedReason = trimmed(f.BlockedReason)
	if f.Status == "blocked" && f.BlockedReason == nil {
		errs = append(errs, &huma.ErrorDetail{Location: "body.blockedReason", Message: "reason is required when blocked"})
	}
	if f.Status != "blocked" {
		f.BlockedReason = nil
	}
	return errs
}

// Customer é o cliente completo, como a API devolve.
type Customer struct {
	ID int64 `json:"id"`
	party.Party
	CustomerType  string  `json:"customerType" enum:"retailer,supermarket,restaurant,street_vendor,wholesaler,institutional,consumer,other"`
	CeasaPavilion *string `json:"ceasaPavilion"`
	CeasaBox      *string `json:"ceasaBox"`
	Status        string  `json:"status" enum:"active,inactive,blocked"`
	BlockedReason *string `json:"blockedReason"`
	// O mesmo cadastro também é fornecedor.
	AlsoSupplier bool `json:"alsoSupplier"`
	party.Timestamps
}

// CustomerSummary é a versão resumida, usada na listagem.
type CustomerSummary struct {
	ID            int64       `json:"id"`
	PersonType    string      `json:"personType" enum:"individual,company"`
	Document      *string     `json:"document"`
	LegalName     string      `json:"legalName"`
	TradeName     *string     `json:"tradeName"`
	Phone         *string     `json:"phone"`
	Whatsapp      *string     `json:"whatsapp"`
	City          *party.City `json:"city"`
	CustomerType  string      `json:"customerType" enum:"retailer,supermarket,restaurant,street_vendor,wholesaler,institutional,consumer,other"`
	CeasaPavilion *string     `json:"ceasaPavilion"`
	CeasaBox      *string     `json:"ceasaBox"`
	Status        string      `json:"status" enum:"active,inactive,blocked"`
	AlsoSupplier  bool        `json:"alsoSupplier"`
}

type listCustomersInput struct {
	Search       string `query:"search" maxLength:"100" doc:"Name or nickname (contains, accent-insensitive) or document (prefix)"`
	Status       string `query:"status" enum:"active,inactive,blocked,all" default:"active"`
	CustomerType string `query:"customerType" enum:"retailer,supermarket,restaurant,street_vendor,wholesaler,institutional,consumer,other,all" default:"all"`
	Limit        int32  `query:"limit" minimum:"1" maximum:"100" default:"20"`
	Offset       int32  `query:"offset" minimum:"0" default:"0"`
}

type listCustomersOutput struct {
	Body struct {
		Items  []CustomerSummary `json:"items"`
		Total  int64             `json:"total"`
		Limit  int32             `json:"limit"`
		Offset int32             `json:"offset"`
	}
}

type idInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type customerOutput struct {
	Body Customer
}

// CustomerCreate são os campos + (opcional) a pessoa já cadastrada que vai
// virar cliente (ex.: um fornecedor com o mesmo CNPJ).
type CustomerCreate struct {
	CustomerFields
	party.Existing
}

type createCustomerInput struct {
	Body CustomerCreate
}

func (in *createCustomerInput) Resolve(huma.Context) []error {
	return append(in.Body.normalize(), in.Body.Existing.Validate()...)
}

// CustomerUpdate são os campos editáveis + a versão que a tela leu.
type CustomerUpdate struct {
	CustomerFields
	Version int32 `json:"version" minimum:"1" doc:"The version read; 409 if someone saved in between"`
}

type updateCustomerInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body CustomerUpdate
}

func (in *updateCustomerInput) Resolve(huma.Context) []error {
	return in.Body.normalize()
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

type handler struct {
	tx TxFunc
}

// Register adiciona as rotas de cliente. api deve ser um grupo que exige login.
func Register(api huma.API, tx TxFunc) {
	h := &handler{tx: tx}
	tags := []string{"Customers"}

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "list-customers", Method: http.MethodGet, Path: "/customers",
		Summary: "List customers", Tags: tags,
	}, auth.CustomersView), h.list)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "get-customer", Method: http.MethodGet, Path: "/customers/{id}",
		Summary: "Get a customer", Tags: tags,
	}, auth.CustomersView), h.get)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "create-customer", Method: http.MethodPost, Path: "/customers",
		Summary:     "Create a customer",
		Description: "Send partyId + partyVersion to turn an existing person (e.g. a supplier with the same CNPJ) into a customer.",
		Tags:        tags, DefaultStatus: http.StatusCreated,
	}, auth.CustomersEdit), h.create)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "update-customer", Method: http.MethodPut, Path: "/customers/{id}",
		Summary: "Update a customer", Tags: tags,
	}, auth.CustomersEdit), h.update)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "deactivate-customer", Method: http.MethodDelete, Path: "/customers/{id}",
		Summary:     "Deactivate a customer",
		Description: "Soft delete: the customer is marked inactive, never removed, to preserve history.",
		Tags:        tags, DefaultStatus: http.StatusNoContent,
	}, auth.CustomersEdit), h.deactivate)
}

func (h *handler) list(ctx context.Context, in *listCustomersInput) (*listCustomersOutput, error) {
	var search, status, customerType *string
	if s := strings.TrimSpace(in.Search); s != "" {
		search = &s
	}
	if in.Status != "all" {
		status = &in.Status
	}
	if in.CustomerType != "" && in.CustomerType != "all" {
		customerType = &in.CustomerType
	}

	var rows []db.ListCustomersRow
	var total int64
	err := h.tx(ctx, func(s Store) error {
		var err error
		rows, err = s.ListCustomers(ctx, db.ListCustomersParams{
			Search: search, Status: status, CustomerType: customerType,
			LimitRows: in.Limit, OffsetRows: in.Offset,
		})
		if err != nil {
			return err
		}
		total, err = s.CountCustomers(ctx, db.CountCustomersParams{Search: search, Status: status, CustomerType: customerType})
		return err
	})
	if err != nil {
		return nil, err
	}

	out := &listCustomersOutput{}
	out.Body.Items = make([]CustomerSummary, 0, len(rows))
	for _, r := range rows {
		item := CustomerSummary{
			ID: r.PartyID, PersonType: r.PersonType, Document: r.Document, LegalName: r.LegalName,
			TradeName: r.TradeName, Phone: r.Phone, Whatsapp: r.Whatsapp,
			CustomerType: r.CustomerType, CeasaPavilion: r.CeasaPavilion, CeasaBox: r.CeasaBox,
			Status: r.Status, AlsoSupplier: r.IsSupplier,
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

func (h *handler) get(ctx context.Context, in *idInput) (*customerOutput, error) {
	var out Customer
	err := h.tx(ctx, func(s Store) error {
		var err error
		out, err = load(ctx, s, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &customerOutput{Body: out}, nil
}

// create grava pessoa (nova ou existente) + cliente + contatos na mesma transação.
func (h *handler) create(ctx context.Context, in *createCustomerInput) (*customerOutput, error) {
	f := &in.Body
	var out Customer
	err := h.tx(ctx, func(s Store) error {
		partyID, err := party.Save(ctx, s, &f.Fields, f.Existing)
		if err != nil {
			return err
		}
		err = s.CreateCustomer(ctx, db.CreateCustomerParams{
			PartyID: partyID, CustomerType: f.CustomerType,
			CeasaPavilion: f.CeasaPavilion, CeasaBox: f.CeasaBox,
			Status: f.Status, BlockedReason: f.BlockedReason,
		})
		if err != nil {
			return err
		}
		out, err = load(ctx, s, partyID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &customerOutput{Body: out}, nil
}

func (h *handler) update(ctx context.Context, in *updateCustomerInput) (*customerOutput, error) {
	f := &in.Body
	var out Customer
	err := h.tx(ctx, func(s Store) error {
		// 1. Reserva a edição (versão igual à que a tela leu), como no fornecedor.
		n, err := s.ClaimPartyVersion(ctx, db.ClaimPartyVersionParams{ID: in.ID, Version: f.Version})
		if err != nil {
			return err
		}
		if n == 0 {
			if _, err := s.GetCustomer(ctx, in.ID); err != nil {
				return err // 404
			}
			return postgres.StaleError()
		}
		// 2. Se a pessoa existe mas não é cliente, 404.
		n, err = s.UpdateCustomer(ctx, db.UpdateCustomerParams{
			PartyID: in.ID, CustomerType: f.CustomerType,
			CeasaPavilion: f.CeasaPavilion, CeasaBox: f.CeasaBox,
			Status: f.Status, BlockedReason: f.BlockedReason,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return pgx.ErrNoRows
		}
		if _, err := s.UpdateParty(ctx, f.UpdateParams(in.ID)); err != nil {
			return err
		}
		if err := party.SaveContacts(ctx, s, in.ID, f.Contacts); err != nil {
			return err
		}
		out, err = load(ctx, s, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &customerOutput{Body: out}, nil
}

func (h *handler) deactivate(ctx context.Context, in *idInput) (*struct{}, error) {
	var n int64
	err := h.tx(ctx, func(s Store) error {
		var err error
		n, err = s.DeactivateCustomer(ctx, in.ID)
		if err != nil || n == 0 {
			return err
		}
		// Quem estiver com o cadastro aberto recebe 409 ao salvar.
		return s.BumpPartyVersion(ctx, in.ID)
	})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, huma.Error404NotFound("customer not found")
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Funções auxiliares
// ---------------------------------------------------------------------------

func load(ctx context.Context, s Store, id int64) (Customer, error) {
	row, err := s.GetCustomer(ctx, id)
	if err != nil {
		return Customer{}, err
	}
	contacts, err := s.ListPartyContacts(ctx, id)
	if err != nil {
		return Customer{}, err
	}
	c := row.Customer
	return Customer{
		ID:            c.PartyID,
		Party:         party.FromDB(row.Party, row.CityName, row.CityState, contacts),
		CustomerType:  c.CustomerType,
		CeasaPavilion: c.CeasaPavilion,
		CeasaBox:      c.CeasaBox,
		Status:        c.Status,
		BlockedReason: c.BlockedReason,
		AlsoSupplier:  row.IsSupplier,
		Timestamps:    party.Timestamps{CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt},
	}, nil
}

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return huma.Error404NotFound("customer not found")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == uniqueViolation && pgErr.ConstraintName == "customers_pkey":
			return huma.Error409Conflict("this person is already a customer")
		case pgErr.Code == uniqueViolation:
			return huma.Error409Conflict("a registration with this document already exists")
		case pgErr.Code == foreignKeyViolation && pgErr.ConstraintName == "parties_city_id_fkey":
			return huma.Error422UnprocessableEntity("validation failed", &huma.ErrorDetail{
				Location: "body.address.cityId", Message: "city not found",
			})
		case pgErr.Code == foreignKeyViolation && pgErr.ConstraintName == "customers_party_id_fkey":
			return huma.Error404NotFound("person not found")
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
