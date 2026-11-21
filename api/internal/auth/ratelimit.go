package auth

import (
	"context"
	"time"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// Limite de tentativas de login (contra quem fica testando senhas).
//
//   - por e-mail: MaxEmailFailures falhas em LoginWindow bloqueiam aquele
//     e-mail (mesmo que ele não exista: a resposta não pode revelar quem tem
//     conta). Um login certo zera a contagem;
//   - por IP: MaxIPFailures falhas em LoginWindow bloqueiam aquele endereço,
//     com qualquer e-mail (quem testa uma senha em vários usuários).
//
// As tentativas feitas DURANTE o bloqueio são registradas
// (too_many_attempts), mas não renovam o bloqueio: senão alguém insistindo
// deixaria o dono da conta trancado para sempre.
//
// As falhas vêm do registro de acessos (access_log); não há tabela extra.
const (
	MaxEmailFailures = 5
	MaxIPFailures    = 20
	LoginWindow      = 15 * time.Minute
)

// loginLockedUntil devolve até quando este e-mail/IP está bloqueado (zero =
// liberado). Deve rodar dentro da transação que já pegou LockLoginEmail.
func loginLockedUntil(ctx context.Context, tx postgres.Tx, email string, ip *string, now time.Time) (time.Time, error) {
	since := now.Add(-LoginWindow)
	var until time.Time

	// O bloqueio vale até a falha mais antiga que conta (a N-ésima mais
	// recente) sair da janela de 15 minutos.
	byEmail, err := tx.RecentEmailFailures(ctx, db.RecentEmailFailuresParams{
		Email: email, Since: since, MaxRows: MaxEmailFailures,
	})
	if err != nil {
		return time.Time{}, err
	}
	if len(byEmail) >= MaxEmailFailures {
		until = byEmail[MaxEmailFailures-1].Add(LoginWindow)
	}

	if ip != nil {
		byIP, err := tx.RecentIPFailures(ctx, db.RecentIPFailuresParams{
			Ip: ip, Since: since, MaxRows: MaxIPFailures,
		})
		if err != nil {
			return time.Time{}, err
		}
		if len(byIP) >= MaxIPFailures {
			if t := byIP[MaxIPFailures-1].Add(LoginWindow); t.After(until) {
				until = t
			}
		}
	}
	if !until.After(now) {
		return time.Time{}, nil
	}
	return until, nil
}
