package party

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/document"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// ---------------------------------------------------------------------------
// Aproveitar uma pessoa que já existe
//
// Na CEASA o mesmo CNPJ costuma ser fornecedor E cliente. Ao cadastrar um
// cliente com o CNPJ de um fornecedor, a tela pergunta "usar este cadastro?"
// e manda o id da pessoa: o cliente é criado em cima dela, sem duplicar nome,
// endereço e contatos.
// ---------------------------------------------------------------------------

// Existing vai no corpo do cadastro de fornecedor ou de cliente.
type Existing struct {
	PartyID      *int64 `json:"partyId,omitempty" minimum:"1" doc:"Reuse this person (same CPF/CNPJ already registered as supplier or customer)"`
	PartyVersion int32  `json:"partyVersion,omitempty" minimum:"0" doc:"Version of the reused person, as read; 409 if someone saved in between"`
}

// Validate confere que a versão veio junto com o id.
func (e Existing) Validate() []error {
	if e.PartyID != nil && e.PartyVersion < 1 {
		return []error{&huma.ErrorDetail{Location: "body.partyVersion", Message: "version is required when reusing a person"}}
	}
	return nil
}

// Store é o que Save precisa do banco.
type Store interface {
	ContactStore
	CreateParty(ctx context.Context, arg db.CreatePartyParams) (int64, error)
	UpdateParty(ctx context.Context, arg db.UpdatePartyParams) (int64, error)
	ClaimPartyVersion(ctx context.Context, arg db.ClaimPartyVersionParams) (int64, error)
}

// Save grava a pessoa de um cadastro novo, com os contatos, e devolve o id:
//   - sem Existing.PartyID: cria uma pessoa nova;
//   - com Existing.PartyID: atualiza a pessoa que já existe, conferindo a
//     versão (se outra pessoa salvou no meio, 409).
//
// Deve rodar dentro da transação que cria o fornecedor ou o cliente.
func Save(ctx context.Context, s Store, f *Fields, e Existing) (int64, error) {
	var id int64
	if e.PartyID == nil {
		var err error
		if id, err = s.CreateParty(ctx, f.CreateParams()); err != nil {
			return 0, err
		}
	} else {
		id = *e.PartyID
		n, err := s.ClaimPartyVersion(ctx, db.ClaimPartyVersionParams{ID: id, Version: e.PartyVersion})
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, postgres.StaleError()
		}
		if _, err := s.UpdateParty(ctx, f.UpdateParams(id)); err != nil {
			return 0, err
		}
	}
	return id, SaveContacts(ctx, s, id, f.Contacts)
}

// ---------------------------------------------------------------------------
// GET /parties/by-document/{document}
// ---------------------------------------------------------------------------

// PartyMatch é a pessoa encontrada pelo documento, com os papéis que ela já tem.
type PartyMatch struct {
	ID int64 `json:"id"`
	Party
	IsSupplier bool `json:"isSupplier"`
	IsCustomer bool `json:"isCustomer"`
}

// LookupStore é o que a busca por documento precisa do banco.
type LookupStore interface {
	GetPartyByDocument(ctx context.Context, document *string) (db.GetPartyByDocumentRow, error)
	ListPartyContacts(ctx context.Context, partyID int64) ([]db.PartyContact, error)
}

type byDocumentInput struct {
	Document string `path:"document" maxLength:"18" doc:"CPF or CNPJ, with or without mask"`
}

type matchOutput struct {
	Body PartyMatch
}

// RegisterLookup adiciona GET /parties/by-document/{document}. Quem pode
// cadastrar fornecedor OU cliente pode consultar (é o mesmo cadastro).
func RegisterLookup(api huma.API, tx func(ctx context.Context, fn func(LookupStore) error) error) {
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "get-party-by-document", Method: http.MethodGet, Path: "/parties/by-document/{document}",
		Summary:     "Find a person by CPF/CNPJ",
		Description: "Used before registering a supplier or customer: if the person already exists, the screen offers to reuse it.",
		Tags:        []string{"Parties"},
	}, auth.SuppliersEdit, auth.CustomersEdit), func(ctx context.Context, in *byDocumentInput) (*matchOutput, error) {
		doc := document.Normalize(strings.TrimSpace(in.Document))
		if !document.IsValid(doc) {
			return nil, huma.Error422UnprocessableEntity("validation failed", &huma.ErrorDetail{
				Location: "path.document", Message: "invalid CPF or CNPJ", Value: in.Document,
			})
		}
		var out PartyMatch
		err := tx(ctx, func(s LookupStore) error {
			row, err := s.GetPartyByDocument(ctx, &doc)
			if err != nil {
				return err
			}
			contacts, err := s.ListPartyContacts(ctx, row.Party.ID)
			if err != nil {
				return err
			}
			out = PartyMatch{
				ID:         row.Party.ID,
				Party:      FromDB(row.Party, row.CityName, row.CityState, contacts),
				IsSupplier: row.IsSupplier,
				IsCustomer: row.IsCustomer,
			}
			return nil
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, huma.Error404NotFound("no registration with this document")
		}
		if err != nil {
			return nil, err
		}
		return &matchOutput{Body: out}, nil
	})
}
