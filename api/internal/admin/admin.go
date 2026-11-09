// Package admin expõe as rotas do painel do administrador da plataforma
// (o dono do SaaS): ver todas as organizações e suspender/reativar.
package admin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
)

// Store é o que estas rotas precisam do banco (*db.Queries satisfaz).
type Store interface {
	AdminListOrganizations(ctx context.Context, arg db.AdminListOrganizationsParams) ([]db.AdminListOrganizationsRow, error)
	AdminCountOrganizations(ctx context.Context) (int64, error)
	AdminSetOrganizationStatus(ctx context.Context, arg db.AdminSetOrganizationStatusParams) (db.Organization, error)
}

// AdminOrganization é a visão do admin sobre uma organização cliente.
type AdminOrganization struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status" enum:"active,suspended"`
	MemberCount int64     `json:"memberCount"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type listOrganizationsInput struct {
	Limit  int32 `query:"limit" minimum:"1" maximum:"100" default:"20"`
	Offset int32 `query:"offset" minimum:"0" default:"0"`
}

type listOrganizationsOutput struct {
	Body struct {
		Items  []AdminOrganization `json:"items"`
		Total  int64               `json:"total"`
		Limit  int32               `json:"limit"`
		Offset int32               `json:"offset"`
	}
}

type setStatusInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		Status string `json:"status" enum:"active,suspended"`
	}
}

type organizationOutput struct {
	Body AdminOrganization
}

type handler struct {
	store Store
}

// Register adiciona as rotas de admin. A api recebida deve ser um grupo
// protegido por auth.RequirePlatformAdmin.
func Register(api huma.API, store Store) {
	h := &handler{store: store}
	tags := []string{"Platform admin"}

	huma.Register(api, huma.Operation{
		OperationID: "admin-list-organizations",
		Method:      http.MethodGet,
		Path:        "/admin/organizations",
		Summary:     "List all organizations",
		Tags:        tags,
	}, h.list)

	huma.Register(api, huma.Operation{
		OperationID: "admin-set-organization-status",
		Method:      http.MethodPut,
		Path:        "/admin/organizations/{id}/status",
		Summary:     "Suspend or reactivate an organization",
		Tags:        tags,
	}, h.setStatus)
}

func (h *handler) list(ctx context.Context, in *listOrganizationsInput) (*listOrganizationsOutput, error) {
	rows, err := h.store.AdminListOrganizations(ctx, db.AdminListOrganizationsParams{
		LimitRows: in.Limit, OffsetRows: in.Offset,
	})
	if err != nil {
		return nil, err
	}
	total, err := h.store.AdminCountOrganizations(ctx)
	if err != nil {
		return nil, err
	}

	out := &listOrganizationsOutput{}
	out.Body.Items = make([]AdminOrganization, 0, len(rows))
	for _, r := range rows {
		out.Body.Items = append(out.Body.Items, AdminOrganization{
			ID: r.ID, Name: r.Name, Status: r.Status, MemberCount: r.MemberCount,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	out.Body.Total = total
	out.Body.Limit = in.Limit
	out.Body.Offset = in.Offset
	return out, nil
}

func (h *handler) setStatus(ctx context.Context, in *setStatusInput) (*organizationOutput, error) {
	o, err := h.store.AdminSetOrganizationStatus(ctx, db.AdminSetOrganizationStatusParams{
		ID: in.ID, Status: in.Body.Status,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, huma.Error404NotFound("organization not found")
	}
	if err != nil {
		return nil, err
	}
	return &organizationOutput{Body: AdminOrganization{
		ID: o.ID, Name: o.Name, Status: o.Status, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}}, nil
}
