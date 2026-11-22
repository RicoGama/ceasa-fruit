// Package role expõe os papéis de acesso (Administrador, Vendedor, Financeiro...)
// e as permissões de cada um. Quem gerencia: permissão users.manage.
//
// Lembrete do desenho (ver internal/auth/permissions.go):
//   - o catálogo de permissões é fixo, no código;
//   - os papéis são criados e editados aqui, marcando permissões do catálogo;
//   - o papel "Administrador" (key = admin) é do sistema: tem tudo e não muda;
//   - mudar as permissões de um papel vale na hora para todos os usuários dele.
package role

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// Role é o papel como a API devolve.
type Role struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Description *string           `json:"description"`
	IsAdmin     bool              `json:"isAdmin" doc:"System role: every permission, cannot be changed or deleted"`
	Permissions []auth.Permission `json:"permissions" doc:"For the admin role, the whole catalog"`
	ActiveUsers int64             `json:"activeUsers"`
	Version     int32             `json:"version"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// RoleFields são os campos editáveis.
type RoleFields struct {
	Name        string            `json:"name" minLength:"2" maxLength:"60"`
	Description *string           `json:"description,omitempty" maxLength:"200"`
	Permissions []auth.Permission `json:"permissions" maxItems:"100"`
}

// normalize limpa os campos e completa as permissões implícitas (quem edita
// fornecedores também precisa vê-los). Não há erro possível aqui: permissão
// desconhecida já é recusada pelo enum do OpenAPI.
func (f *RoleFields) normalize() {
	f.Name = strings.TrimSpace(f.Name)
	if f.Description != nil {
		d := strings.TrimSpace(*f.Description)
		if d == "" {
			f.Description = nil
		} else {
			f.Description = &d
		}
	}
	perms := slices.Clone(f.Permissions)
	for _, p := range f.Permissions {
		if implied, ok := auth.Implies[p]; ok {
			perms = append(perms, implied)
		}
	}
	slices.Sort(perms)
	f.Permissions = slices.Compact(perms)
}

// RoleUpdate são os campos + a versão que a tela leu.
type RoleUpdate struct {
	RoleFields
	Version int32 `json:"version" minimum:"1" doc:"The version read; 409 if someone saved in between"`
}

type listOutput struct {
	Body struct {
		Items []Role `json:"items"`
	}
}

type idInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type createInput struct{ Body RoleFields }

func (in *createInput) Resolve(huma.Context) []error { in.Body.normalize(); return nil }

type updateInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body RoleUpdate
}

func (in *updateInput) Resolve(huma.Context) []error { in.Body.normalize(); return nil }

type roleOutput struct{ Body Role }

type handler struct{ db postgres.DB }

// Register adiciona as rotas (todas exigem users.manage).
func Register(api huma.API, database postgres.DB) {
	h := &handler{db: database}
	tags := []string{"Roles"}
	manage := func(op huma.Operation) huma.Operation { return auth.Protect(api, op, auth.UsersManage) }

	huma.Register(api, manage(huma.Operation{
		OperationID: "list-roles", Method: http.MethodGet, Path: "/roles",
		Summary: "List roles with their permissions", Tags: tags,
	}), h.list)

	huma.Register(api, manage(huma.Operation{
		OperationID: "get-role", Method: http.MethodGet, Path: "/roles/{id}",
		Summary: "Get a role", Tags: tags,
	}), h.get)

	huma.Register(api, manage(huma.Operation{
		OperationID: "create-role", Method: http.MethodPost, Path: "/roles",
		Summary: "Create a role", Tags: tags, DefaultStatus: http.StatusCreated,
	}), h.create)

	huma.Register(api, manage(huma.Operation{
		OperationID: "update-role", Method: http.MethodPut, Path: "/roles/{id}",
		Summary:     "Update a role's name, description and permissions",
		Description: "Takes effect immediately for every user with this role.",
		Tags:        tags,
	}), h.update)

	huma.Register(api, manage(huma.Operation{
		OperationID: "delete-role", Method: http.MethodDelete, Path: "/roles/{id}",
		Summary: "Delete a role that has no users", Tags: tags, DefaultStatus: http.StatusNoContent,
	}), h.delete)
}

func (h *handler) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	out := &listOutput{}
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		rows, err := tx.ListRoles(ctx)
		if err != nil {
			return err
		}
		out.Body.Items = make([]Role, 0, len(rows))
		for _, r := range rows {
			out.Body.Items = append(out.Body.Items, toAPI(db.GetRoleRow(r)))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *handler) get(ctx context.Context, in *idInput) (*roleOutput, error) {
	var out Role
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		var err error
		out, err = load(ctx, tx, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &roleOutput{Body: out}, nil
}

func (h *handler) create(ctx context.Context, in *createInput) (*roleOutput, error) {
	var out Role
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		id, err := tx.CreateRole(ctx, db.CreateRoleParams{Name: in.Body.Name, Description: in.Body.Description})
		if err != nil {
			return err
		}
		if err := savePermissions(ctx, tx, id, in.Body.Permissions); err != nil {
			return err
		}
		out, err = load(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &roleOutput{Body: out}, nil
}

func (h *handler) update(ctx context.Context, in *updateInput) (*roleOutput, error) {
	f := in.Body
	var out Role
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		n, err := tx.UpdateRole(ctx, db.UpdateRoleParams{
			ID: in.ID, Version: f.Version, Name: f.Name, Description: f.Description,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			// Não existe (404), é o administrador (409) ou a versão mudou (409)?
			current, err := tx.GetRole(ctx, in.ID)
			if err != nil {
				return err
			}
			if isAdmin(current.Key) {
				return errSystemRole
			}
			return postgres.StaleError()
		}
		if err := savePermissions(ctx, tx, in.ID, f.Permissions); err != nil {
			return err
		}
		out, err = load(ctx, tx, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &roleOutput{Body: out}, nil
}

func (h *handler) delete(ctx context.Context, in *idInput) (*struct{}, error) {
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		n, err := tx.DeleteRole(ctx, in.ID)
		if err != nil || n > 0 {
			return err
		}
		current, err := tx.GetRole(ctx, in.ID)
		if err != nil {
			return err
		}
		if isAdmin(current.Key) {
			return errSystemRole
		}
		return errRoleInUse
	})
	return nil, mapError(err)
}

// savePermissions deixa o papel com exatamente as permissões recebidas,
// mexendo só no que mudou (o histórico mostra o que entrou e o que saiu).
func savePermissions(ctx context.Context, tx postgres.Tx, roleID int64, perms []auth.Permission) error {
	codes := make([]string, len(perms))
	for i, p := range perms {
		codes[i] = string(p)
	}
	if err := tx.RemoveRolePermissionsNotIn(ctx, db.RemoveRolePermissionsNotInParams{RoleID: roleID, Permissions: codes}); err != nil {
		return err
	}
	if len(codes) == 0 {
		return nil
	}
	return tx.AddRolePermissions(ctx, db.AddRolePermissionsParams{RoleID: roleID, Permissions: codes})
}

func load(ctx context.Context, tx postgres.Tx, id int64) (Role, error) {
	r, err := tx.GetRole(ctx, id)
	if err != nil {
		return Role{}, err
	}
	return toAPI(r), nil
}

func toAPI(r db.GetRoleRow) Role {
	out := Role{
		ID: r.ID, Name: r.Name, Description: r.Description, IsAdmin: isAdmin(r.Key),
		ActiveUsers: r.ActiveUsers, Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if out.IsAdmin {
		out.Permissions = auth.All
		return out
	}
	out.Permissions = make([]auth.Permission, 0, len(r.Permissions))
	for _, p := range r.Permissions {
		// Permissão que saiu do catálogo (código antigo) não aparece mais.
		if perm := auth.Permission(p); perm.Valid() {
			out.Permissions = append(out.Permissions, perm)
		}
	}
	return out
}

func isAdmin(key *string) bool { return key != nil && *key == auth.RoleKeyAdmin }

var (
	errSystemRole = errors.New("system role")
	errRoleInUse  = errors.New("role in use")
)

func mapError(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return huma.Error404NotFound("role not found")
	case errors.Is(err, errSystemRole):
		return huma.Error409Conflict("the administrator role cannot be changed")
	case errors.Is(err, errRoleInUse):
		return huma.Error409Conflict("role has users; move them to another role first")
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return huma.Error409Conflict("a role with this name already exists")
	}
	return err
}
