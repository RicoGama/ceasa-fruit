// Package city expõe a busca de municípios (lista oficial do IBGE), usada no
// campo "Cidade" dos cadastros.
package city

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/party"
)

// Store é o que a rota precisa do banco (*db.Queries satisfaz).
type Store interface {
	SearchCities(ctx context.Context, arg db.SearchCitiesParams) ([]db.City, error)
}

type searchInput struct {
	Search string `query:"search" minLength:"2" maxLength:"60" required:"true" doc:"Part of the city name (accent-insensitive)"`
	State  string `query:"state" pattern:"^[A-Za-z]{2}$" doc:"Optional state filter, e.g. RJ"`
	Limit  int32  `query:"limit" minimum:"1" maximum:"50" default:"20"`
}

type searchOutput struct {
	Body []party.City
}

// Register adiciona a rota. Municípios são dados públicos, mas a rota fica no
// grupo de usuários logados para não virar uma API aberta na internet.
func Register(api huma.API, store Store) {
	huma.Register(api, huma.Operation{
		OperationID: "search-cities",
		Method:      http.MethodGet,
		Path:        "/cities",
		Summary:     "Search cities (IBGE)",
		Tags:        []string{"Reference data"},
	}, func(ctx context.Context, in *searchInput) (*searchOutput, error) {
		var state *string
		if in.State != "" {
			s := strings.ToUpper(in.State)
			state = &s
		}
		rows, err := store.SearchCities(ctx, db.SearchCitiesParams{
			Search: strings.TrimSpace(in.Search), State: state, LimitRows: in.Limit,
		})
		if err != nil {
			return nil, err
		}
		out := &searchOutput{Body: make([]party.City, 0, len(rows))}
		for _, r := range rows {
			out.Body = append(out.Body, party.City{ID: r.ID, Name: r.Name, State: r.StateCode})
		}
		return out, nil
	})
}
