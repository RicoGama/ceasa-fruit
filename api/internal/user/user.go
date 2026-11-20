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
	Role      string    `json:"role" enum:"admin,manager,seller,checker,finance,driver"`
	Active    bool      `json:"active"`
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
		Name  string `json:"name" minLength:"2" maxLength:"120"`
		Email string `json:"email" format:"email" maxLength:"254"`
		Role  string `json:"role" enum:"admin,manager,seller,checker,finance,driver"`
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
		Role   string `json:"role" enum:"admin,manager,seller,checker,finance,driver"`
		Active bool   `json:"active"`
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

	huma.Register(api, huma.Operation{
		OperationID: "list-users", Method: http.MethodGet, Path: "/users",
		Summary: "List users", Tags: tags,
	}, h.list)

	huma.Register(api, huma.Operation{
		OperationID: "create-user", Method: http.MethodPost, Path: "/users",
		Summary:     "Create a user",
		Description: "Without a password, an invitation email is sent so the person sets their own.",
		Tags:        tags, DefaultStatus: http.StatusCreated,
	}, h.create)

	huma.Register(api, huma.Operation{
		OperationID: "update-user", Method: http.MethodPut, Path: "/users/{id}",
		Summary: "Update a user's name, role or status", Tags: tags,
	}, h.update)

	huma.Register(api, huma.Operation{
		OperationID: "resend-user-invite", Method: http.MethodPost, Path: "/users/{id}/invite",
		Summary: "Send a new invitation (link to set the password)", Tags: tags,
		DefaultStatus: http.StatusAccepted,
	}, h.resendInvite)
}

func (h *handler) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	var rows []db.User
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
	for _, u := range rows {
		out.Body.Items = append(out.Body.Items, toAPI(u))
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

	var created db.User
	err = h.db.InTx(ctx, func(tx postgres.Tx) error {
		var err error
		created, err = tx.CreateUser(ctx, db.CreateUserParams{
			Name: b.Name, Email: b.Email, PasswordHash: hash, Role: b.Role,
		})
		if err != nil {
			return err
		}
		if b.Password == nil {
			return auth.SendPasswordLink(ctx, tx, h.emails, h.appURL, created, auth.PasswordLinkInvite)
		}
		return nil
	})
	if isUniqueViolation(err) {
		return nil, huma.Error409Conflict("email already registered")
	}
	if err != nil {
		return nil, err
	}
	return &userOutput{Body: toAPI(created)}, nil
}

func (h *handler) update(ctx context.Context, in *updateUserInput) (*userOutput, error) {
	me, _ := auth.FromContext(ctx)
	b := in.Body
	b.Name = strings.TrimSpace(b.Name)

	if in.ID == me.UserID && !b.Active {
		return nil, huma.Error409Conflict("you cannot deactivate your own user")
	}

	var updated db.User
	err := h.db.InTx(ctx, func(q postgres.Tx) error {
		current, err := q.GetUser(ctx, in.ID)
		if err != nil {
			return err
		}
		updated, err = q.UpdateUser(ctx, db.UpdateUserParams{ID: in.ID, Name: b.Name, Role: b.Role, Active: b.Active})
		if err != nil {
			return err
		}
		// Se era um admin ativo e deixou de ser, confere que sobrou pelo menos um.
		wasAdmin := current.Role == auth.RoleAdmin && current.Active
		isAdmin := updated.Role == auth.RoleAdmin && updated.Active
		if wasAdmin && !isAdmin {
			n, err := q.CountActiveAdmins(ctx)
			if err != nil {
				return err
			}
			if n == 0 {
				return errLastAdmin // desfaz a transação inteira
			}
		}
		// Desativado: encerra as sessões abertas na hora.
		if !updated.Active {
			return q.DeleteUserSessions(ctx, in.ID)
		}
		return nil
	})
	switch {
	case errors.Is(err, errLastAdmin):
		return nil, huma.Error409Conflict("the system must keep at least one active admin")
	case errors.Is(err, pgx.ErrNoRows):
		return nil, huma.Error404NotFound("user not found")
	case err != nil:
		return nil, err
	}
	return &userOutput{Body: toAPI(updated)}, nil
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
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, huma.Error404NotFound("user not found")
	case errors.Is(err, errInactive):
		return nil, huma.Error409Conflict("user is inactive")
	}
	return nil, err
}

var (
	errLastAdmin = errors.New("last admin")
	errInactive  = errors.New("inactive user")
)

func toAPI(u db.User) User {
	return User{
		ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role, Active: u.Active,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
