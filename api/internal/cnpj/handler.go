package cnpj

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/document"
)

type lookupInput struct {
	CNPJ string `path:"cnpj" maxLength:"18" doc:"CNPJ, with or without mask"`
}

type lookupOutput struct{ Body CNPJData }

// Register adiciona GET /cnpj/{cnpj}. Quem cadastra fornecedor, cliente ou os
// dados da empresa pode consultar.
func Register(api huma.API, svc *Service) {
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "lookup-cnpj", Method: http.MethodGet, Path: "/cnpj/{cnpj}",
		Summary:     "Public data of a CNPJ at the Federal Revenue (Receita Federal)",
		Description: "Company name, status and address, to fill in registrations. Needs internet on the server.",
		Tags:        []string{"Parties"},
	}, auth.SuppliersEdit, auth.CustomersEdit, auth.CompanyEdit), func(ctx context.Context, in *lookupInput) (*lookupOutput, error) {
		doc := document.Normalize(strings.TrimSpace(in.CNPJ))
		if len(doc) != 14 || !document.IsValid(doc) {
			return nil, huma.Error422UnprocessableEntity("validation failed", &huma.ErrorDetail{
				Location: "path.cnpj", Message: "invalid CNPJ", Value: in.CNPJ,
			})
		}
		c, err := svc.Lookup(ctx, doc)
		switch {
		case errors.Is(err, ErrNotFound):
			return nil, huma.Error404NotFound("CNPJ not found at the Federal Revenue")
		case errors.Is(err, ErrUnavailable):
			return nil, huma.Error503ServiceUnavailable("could not reach the Federal Revenue lookup")
		case err != nil:
			return nil, err
		}
		return &lookupOutput{Body: c}, nil
	})
}
