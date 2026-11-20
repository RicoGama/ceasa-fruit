// Package user expõe a gestão de usuários, feita pelo administrador.
//
// Não existe cadastro público: o administrador cria o usuário e o sistema
// manda um convite por e-mail com o link para a pessoa criar a própria senha.
// (Ou o administrador já define uma senha inicial, se preferir.)
package user

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// User é o usuário como a API devolve (nunca com a senha).
type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	RoleID    int64     `json:"roleId"`
	RoleName  string    `json:"roleName"`
	Active    bool      `json:"active"`
	Version   int32     `json:"version" doc:"Send it back on update (optimistic locking)"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type listOutput struct {
	Body struct {
		Items []User `json:"items"`
	}
}

type userOutput struct {
	Body User
}

type createUserInput struct {
	Body struct {
		Name   string `json:"name" minLength:"2" maxLength:"120"`
		Email  string `json:"email" format:"email" maxLength:"254"`
		RoleID int64  `json:"roleId" minimum:"1"`
		// Opcional: sem senha, a pessoa recebe um convite por e-mail para criar a dela.
		Password *string `json:"password,omitempty" minLength:"8" maxLength:"128"`
	}
}

func (in *createUserInput) Resolve(huma.Context) []error {
	in.Body.Name = strings.TrimSpace(in.Body.Name)
	in.Body.Email = auth.NormalizeEmail(in.Body.Email)
	return nil
}

type updateUserInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		Name   string `json:"name" minLength:"2" maxLength:"120"`
		RoleID int64  `json:"roleId" minimum:"1"`
		Active bool   `json:"active"`
		// Versão que a tela leu; se outra pessoa salvou depois, a API responde 409.
		Version int32 `json:"version" minimum:"1"`
	}
}

type idInput struct {
	ID int64 `path:"id" minimum:"1"`
}

// Options agrupa as dependências.
type Options struct {
	DB     postgres.DB
	Emails auth.EmailQueue
	AppURL string
}

type handler struct {
	db     postgres.DB
	emails auth.EmailQueue
	appURL string
}

// Register adiciona as rotas. A api recebida deve ser o grupo do administrador.
func Register(api huma.API, opts Options) {
	h := &handler{db: opts.DB, emails: opts.Emails, appURL: opts.AppURL}
	tags := []string{"Users"}
	manage := func(op huma.Operation) huma.Operation { return auth.Protect(api, op, auth.UsersManage) }

	huma.Register(api, manage(huma.Operation{
		OperationID: "list-users", Method: http.MethodGet, Path: "/users",
		Summary: "List users", Tags: tags,
	}), h.list)

	huma.Register(api, manage(huma.Operation{
		OperationID: "create-user", Method: http.MethodPost, Path: "/users",
		Summary:     "Create a user",
		Description: "Without a password, an invitation email is sent so the person sets their own.",
		Tags:        tags, DefaultStatus: http.StatusCreated,
	}), h.create)

	huma.Register(api, manage(huma.Operation{
		OperationID: "update-user", Method: http.MethodPut, Path: "/users/{id}",
		Summary: "Update a user's name, role or status", Tags: tags,
	}), h.update)

	huma.Register(api, manage(huma.Operation{
		OperationID: "resend-user-invite", Method: http.MethodPost, Path: "/users/{id}/invite",
		Summary: "Send a new invitation (link to set the password)", Tags: tags,
		DefaultStatus: http.StatusAccepted,
	}), h.resendInvite)
}

func (h *handler) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	var rows []db.ListUsersRow
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		var err error
		rows, err = tx.ListUsers(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := &listOutput{}
	out.Body.Items = make([]User, 0, len(rows))
	for _, r := range rows {
		out.Body.Items = append(out.Body.Items, toAPI(r.User, r.RoleName))
	}
	return out, nil
}

func (h *handler) create(ctx context.Context, in *createUserInput) (*userOutput, error) {
	b := in.Body

	// Sem senha informada, gravamos o hash de uma senha aleatória que ninguém
	// conhece: a pessoa só entra depois de criar a dela pelo link do convite.
	password := rand.Text()
	if b.Password != nil {
		password = *b.Password
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}

	var out User
	err = h.db.InTx(ctx, func(tx postgres.Tx) error {
		created, err := tx.CreateUser(ctx, db.CreateUserParams{
			Name: b.Name, Email: b.Email, PasswordHash: hash, RoleID: b.RoleID,
		})
		if err != nil {
			return err
		}
		if b.Password == nil {
			if err := auth.SendPasswordLink(ctx, tx, h.emails, h.appURL, created, auth.PasswordLinkInvite); err != nil {
				return err
			}
		}
		out, err = loadUser(ctx, tx, created.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &userOutput{Body: out}, nil
}

func (h *handler) update(ctx context.Context, in *updateUserInput) (*userOutput, error) {
	me, _ := auth.FromContext(ctx)
	b := in.Body
	b.Name = strings.TrimSpace(b.Name)

	if in.ID == me.UserID && !b.Active {
		return nil, huma.Error409Conflict("you cannot deactivate your own user")
	}

	var out User
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		if err := applyUpdate(ctx, tx, in.ID, b.Version, b.Name, b.RoleID, b.Active); err != nil {
			return err
		}
		var err error
		out, err = loadUser(ctx, tx, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &userOutput{Body: out}, nil
}

// applyUpdate altera o usuário dentro da transação tx. Fica separada do
// handler para o teste de concorrência controlar quando cada transação grava.
//
// Concorrência:
//   - version: a tela manda a versão que leu; se outra pessoa salvou no
//     meio, a versão não bate e devolvemos StaleError (409);
//   - último admin: dois admins rebaixando um ao outro AO MESMO TEMPO veriam,
//     cada um, "ainda existe outro admin" e o sistema ficaria sem nenhum.
//     Por isso travamos as linhas dos admins ativos ANTES de checar: a segunda
//     transação espera a primeira terminar e só então conta.
func applyUpdate(ctx context.Context, q postgres.Tx, id int64, version int32, name string, roleID int64, active bool) error {
	admins, err := q.LockActiveAdmins(ctx)
	if err != nil {
		return err
	}
	current, err := q.GetUserWithRole(ctx, id)
	if err != nil {
		return err
	}
	if current.User.Version != version {
		return postgres.StaleError()
	}
	_, err = q.UpdateUser(ctx, db.UpdateUserParams{ID: id, Version: version, Name: name, RoleID: roleID, Active: active})
	if errors.Is(err, pgx.ErrNoRows) {
		return postgres.StaleError() // existia, mas a versão mudou no meio
	}
	if err != nil {
		return err
	}
	updated, err := q.GetUserWithRole(ctx, id)
	if err != nil {
		return err
	}

	// Se era um admin ativo e deixou de ser, confere que sobrou pelo menos um.
	// (admins = os ativos no momento da trava, já com as alterações de quem
	// gravou antes de nós.)
	if isActiveAdmin(current) && !isActiveAdmin(updated) {
		remaining := 0
		for _, adminID := range admins {
			if adminID != id {
				remaining++
			}
		}
		if remaining == 0 {
			return errLastAdmin // desfaz a transação inteira
		}
	}
	// Desativado: encerra as sessões abertas na hora.
	if !updated.User.Active {
		return q.DeleteUserSessions(ctx, id)
	}
	return nil
}

func isActiveAdmin(u db.GetUserWithRoleRow) bool {
	return u.User.Active && u.RoleKey != nil && *u.RoleKey == auth.RoleKeyAdmin
}

func (h *handler) resendInvite(ctx context.Context, in *idInput) (*struct{}, error) {
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		u, err := tx.GetUser(ctx, in.ID)
		if err != nil {
			return err
		}
		if !u.Active {
			return errInactive
		}
		return auth.SendPasswordLink(ctx, tx, h.emails, h.appURL, u, auth.PasswordLinkInvite)
	})
	return nil, mapError(err)
}

var (
	errLastAdmin = errors.New("last admin")
	errInactive  = errors.New("inactive user")
)

func loadUser(ctx context.Context, tx postgres.Tx, id int64) (User, error) {
	r, err := tx.GetUserWithRole(ctx, id)
	if err != nil {
		return User{}, err
	}
	return toAPI(r.User, r.RoleName), nil
}

func toAPI(u db.User, roleName string) User {
	return User{
		ID: u.ID, Name: u.Name, Email: u.Email, RoleID: u.RoleID, RoleName: roleName,
		Active: u.Active, Version: u.Version, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}

// mapError traduz os erros em respostas HTTP.
func mapError(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errLastAdmin):
		return huma.Error409Conflict("the system must keep at least one active admin")
	case errors.Is(err, errInactive):
		return huma.Error409Conflict("user is inactive")
	case errors.Is(err, pgx.ErrNoRows):
		return huma.Error404NotFound("user not found")
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return huma.Error409Conflict("email already registered")
	case errors.As(err, &pgErr) && pgErr.Code == "23503":
		return huma.Error422UnprocessableEntity("validation failed",
			&huma.ErrorDetail{Location: "body.roleId", Message: "role not found"})
	}
	return err
}
