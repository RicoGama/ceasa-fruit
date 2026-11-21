// Package access expõe o registro de acessos (migration 00009): quem entrou,
// quando, de onde, até quando ficou e como saiu, além das tentativas de login
// que falharam. Exige a permissão audit.view (por padrão, só o administrador).
package access

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// OnlineWindow: sem atividade há menos que isso, a pessoa conta como "online".
// (A última atividade é gravada a cada minuto enquanto a tela está aberta.)
const OnlineWindow = 3 * time.Minute

// AccessEntry é uma linha do registro.
type AccessEntry struct {
	ID       int64   `json:"id"`
	Kind     string  `json:"kind" enum:"session,failed_login"`
	UserID   *int64  `json:"userId"`
	UserName *string `json:"userName" doc:"null when the email does not belong to any user"`
	Email    string  `json:"email"`
	IP       *string `json:"ip"`
	// Navegador e sistema, como o aparelho informou (o front resume).
	UserAgent  *string    `json:"userAgent"`
	StartedAt  time.Time  `json:"startedAt"`
	LastSeenAt time.Time  `json:"lastSeenAt"`
	EndedAt    *time.Time `json:"endedAt"`
	// Situação da sessão:
	//   online   está usando agora
	//   idle     sessão aberta, mas sem atividade recente (fechou o navegador?)
	//   expired  a sessão venceu sem a pessoa sair
	//   ended    saiu (ver endReason)
	//   failed   tentativa de login que falhou (ver endReason)
	Status    string  `json:"status" enum:"online,idle,expired,ended,failed"`
	EndReason *string `json:"endReason" enum:"logout,password_changed,deactivated,wrong_password,unknown_email,inactive_user,too_many_attempts"`
}

type listInput struct {
	UserID int64  `query:"userId" minimum:"0" doc:"Only this user"`
	Kind   string `query:"kind" enum:"session,failed_login" doc:"Only sessions or only failed logins"`
	Days   int32  `query:"days" minimum:"1" maximum:"366" default:"7" doc:"How many days back"`
	Limit  int32  `query:"limit" minimum:"1" maximum:"200" default:"50"`
	Offset int32  `query:"offset" minimum:"0" default:"0"`
}

type listOutput struct {
	Body struct {
		Items  []AccessEntry `json:"items"`
		Total  int64         `json:"total"`
		Limit  int32         `json:"limit"`
		Offset int32         `json:"offset"`
	}
}

// Register adiciona a rota (api = grupo de usuários logados).
func Register(api huma.API, database postgres.DB) {
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "list-access-log", Method: http.MethodGet, Path: "/access-log",
		Summary: "Access log: logins, logouts and failed attempts", Tags: []string{"Users"},
	}, auth.AuditView), func(ctx context.Context, in *listInput) (*listOutput, error) {
		var userID *int64
		if in.UserID > 0 {
			userID = &in.UserID
		}
		var kind *string
		if in.Kind != "" {
			kind = &in.Kind
		}
		since := time.Now().AddDate(0, 0, -int(in.Days))

		out := &listOutput{}
		err := database.InReadTx(ctx, func(tx postgres.Tx) error {
			rows, err := tx.ListAccessLog(ctx, db.ListAccessLogParams{
				UserID: userID, Kind: kind, Since: since, LimitRows: in.Limit, OffsetRows: in.Offset,
			})
			if err != nil {
				return err
			}
			out.Body.Total, err = tx.CountAccessLog(ctx, db.CountAccessLogParams{UserID: userID, Kind: kind, Since: since})
			if err != nil {
				return err
			}
			out.Body.Items = make([]AccessEntry, 0, len(rows))
			for _, r := range rows {
				out.Body.Items = append(out.Body.Items, AccessEntry{
					ID: r.ID, Kind: r.Kind, UserID: r.UserID, UserName: r.UserName, Email: r.Email,
					IP: r.Ip, UserAgent: r.UserAgent, StartedAt: r.StartedAt, LastSeenAt: r.LastSeenAt,
					EndedAt: r.EndedAt, Status: status(r), EndReason: r.EndReason,
				})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		out.Body.Limit, out.Body.Offset = in.Limit, in.Offset
		return out, nil
	})
}

func status(r db.ListAccessLogRow) string {
	switch {
	case r.Kind == "failed_login":
		return "failed"
	case r.EndedAt != nil:
		return "ended"
	case !r.SessionOpen:
		return "expired"
	case time.Since(r.LastSeenAt) < OnlineWindow:
		return "online"
	default:
		return "idle"
	}
}
