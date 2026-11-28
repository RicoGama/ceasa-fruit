package certificate

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
	"github.com/RicoGama/ceasa-fruit/api/internal/secrets"
)

// ErrNoCertificate: nenhum certificado foi enviado (ou foi removido).
var ErrNoCertificate = errors.New("no digital certificate configured")

// ErrOtherKey: o certificado foi gravado com outra SECRETS_KEY (ex.: banco
// restaurado num servidor novo sem a chave antiga). Solução: enviar de novo.
var ErrOtherKey = errors.New("the certificate was stored with another SECRETS_KEY: send it again")

// Load abre o certificado em uso, pronto para assinar. É o que a emissão de
// notas vai usar. Confere de novo a validade: um certificado que venceu
// depois de enviado não assina nada.
func Load(ctx context.Context, tx postgres.Tx, box *secrets.Box, now time.Time) (*Parsed, error) {
	if box == nil {
		return nil, secrets.ErrNotConfigured
	}
	row, err := tx.GetActiveCertificateSecret(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoCertificate
	}
	if err != nil {
		return nil, err
	}
	if row.KeyID == nil || *row.KeyID != box.KeyID() {
		return nil, ErrOtherKey
	}
	pfx, err := box.Open(row.EncryptedPfx, labelPFX)
	if err != nil {
		return nil, ErrOtherKey
	}
	password, err := box.Open(row.EncryptedPassword, labelPassword)
	if err != nil {
		return nil, ErrOtherKey
	}
	return Parse(pfx, string(password), now)
}
