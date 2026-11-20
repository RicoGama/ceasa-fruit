// Package company expõe o perfil da empresa desta instalação (um registro só).
// Ver: GET /company (qualquer usuário logado) e PUT /company (administrador).
package company

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/document"
	"github.com/RicoGama/ceasa-fruit/api/internal/party"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// Company é a empresa como a API devolve.
type Company struct {
	Name              string      `json:"name"`
	TradeName         *string     `json:"tradeName"`
	Document          *string     `json:"document" doc:"CNPJ"`
	StateRegistration *string     `json:"stateRegistration"`
	TaxRegime         *string     `json:"taxRegime" enum:"simples,presumido,real"`
	Phone             *string     `json:"phone"`
	Email             *string     `json:"email"`
	PostalCode        *string     `json:"postalCode"`
	Street            *string     `json:"street"`
	Number            *string     `json:"number"`
	Complement        *string     `json:"complement"`
	District          *string     `json:"district"`
	City              *party.City `json:"city"`
	Version           int32       `json:"version" doc:"Send it back on update (optimistic locking)"`
	UpdatedAt         time.Time   `json:"updatedAt"`
}

type companyOutput struct {
	Body Company
}

type updateCompanyInput struct {
	Body struct {
		Version           int32   `json:"version" minimum:"1" doc:"The version read; 409 if someone saved in between"`
		Name              string  `json:"name" minLength:"2" maxLength:"150" doc:"Legal name (razão social)"`
		TradeName         *string `json:"tradeName,omitempty" maxLength:"150"`
		Document          *string `json:"document,omitempty" maxLength:"18" doc:"CNPJ, with or without mask"`
		StateRegistration *string `json:"stateRegistration,omitempty" maxLength:"20"`
		TaxRegime         *string `json:"taxRegime,omitempty" enum:"simples,presumido,real"`
		Phone             *string `json:"phone,omitempty" maxLength:"20"`
		Email             *string `json:"email,omitempty" format:"email" maxLength:"150"`
		PostalCode        *string `json:"postalCode,omitempty" maxLength:"9"`
		Street            *string `json:"street,omitempty" maxLength:"150"`
		Number            *string `json:"number,omitempty" maxLength:"20"`
		Complement        *string `json:"complement,omitempty" maxLength:"100"`
		District          *string `json:"district,omitempty" maxLength:"100"`
		CityID            *int32  `json:"cityId,omitempty" minimum:"1000000" maximum:"9999999"`
	}
}

var nonDigits = regexp.MustCompile(`\D`)

func (in *updateCompanyInput) Resolve(huma.Context) []error {
	b := &in.Body
	var errs []error
	b.Name = strings.TrimSpace(b.Name)
	for _, p := range []**string{&b.TradeName, &b.StateRegistration, &b.Phone, &b.Email, &b.Street, &b.Number, &b.Complement, &b.District} {
		*p = clean(*p)
	}
	if d := clean(b.Document); d != nil {
		n := document.Normalize(*d)
		b.Document = &n
		if len(n) != 14 || !document.IsValidCNPJ(n) {
			errs = append(errs, &huma.ErrorDetail{Location: "body.document", Message: "invalid CNPJ", Value: n})
		}
	} else {
		b.Document = nil
	}
	if pc := clean(b.PostalCode); pc != nil {
		digits := nonDigits.ReplaceAllString(*pc, "")
		b.PostalCode = &digits
		if len(digits) != 8 {
			errs = append(errs, &huma.ErrorDetail{Location: "body.postalCode", Message: "invalid postal code", Value: *pc})
		}
	} else {
		b.PostalCode = nil
	}
	return errs
}

type handler struct{ db postgres.DB }

// errNotConfigured: a empresa ainda não foi criada (make seed).
var errNotConfigured = huma.Error404NotFound("company not configured")

// Register adiciona as rotas: qualquer usuário logado vê os dados da empresa;
// alterar exige a permissão company.edit.
func Register(api huma.API, database postgres.DB) {
	h := &handler{db: database}
	tags := []string{"Company"}

	huma.Register(api, huma.Operation{
		OperationID: "get-company", Method: http.MethodGet, Path: "/company",
		Summary: "Company of this installation", Tags: tags,
	}, h.get)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "update-company", Method: http.MethodPut, Path: "/company",
		Summary: "Update the company profile", Tags: tags,
	}, auth.CompanyEdit), h.update)
}

func (h *handler) get(ctx context.Context, _ *struct{}) (*companyOutput, error) {
	var out Company
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		var err error
		out, err = load(ctx, tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &companyOutput{Body: out}, nil
}

// update grava e lê de volta na MESMA transação: a resposta é exatamente o
// que ficou gravado.
func (h *handler) update(ctx context.Context, in *updateCompanyInput) (*companyOutput, error) {
	b := in.Body
	var out Company
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		n, err := tx.UpdateCompany(ctx, db.UpdateCompanyParams{
			Version: b.Version, Name: b.Name, TradeName: b.TradeName, Document: b.Document, StateRegistration: b.StateRegistration,
			TaxRegime: b.TaxRegime, Phone: b.Phone, Email: b.Email, PostalCode: b.PostalCode,
			Street: b.Street, Number: b.Number, Complement: b.Complement, District: b.District, CityID: b.CityID,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			// Ou a empresa não existe (404), ou outra pessoa salvou no meio (409).
			if _, err := load(ctx, tx); err != nil {
				return err
			}
			return postgres.StaleError()
		}
		out, err = load(ctx, tx)
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return nil, huma.Error422UnprocessableEntity("validation failed",
			&huma.ErrorDetail{Location: "body.cityId", Message: "city not found"})
	}
	if err != nil {
		return nil, err
	}
	return &companyOutput{Body: out}, nil
}

// load lê a empresa e converte para o formato da API.
func load(ctx context.Context, tx postgres.Tx) (Company, error) {
	c, err := tx.GetCompany(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return Company{}, errNotConfigured
	}
	if err != nil {
		return Company{}, err
	}
	out := Company{
		Name: c.Name, TradeName: c.TradeName, Document: c.Document, StateRegistration: c.StateRegistration,
		TaxRegime: c.TaxRegime, Phone: c.Phone, Email: c.Email, PostalCode: c.PostalCode,
		Street: c.Street, Number: c.Number, Complement: c.Complement, District: c.District,
		Version: c.Version, UpdatedAt: c.UpdatedAt,
	}
	if c.CityID != nil && c.CityName != nil && c.CityState != nil {
		out.City = &party.City{ID: *c.CityID, Name: *c.CityName, State: *c.CityState}
	}
	return out, nil
}

func clean(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}
