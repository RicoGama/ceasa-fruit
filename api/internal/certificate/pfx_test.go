package certificate_test

import (
	"errors"
	"testing"
	"time"

	"github.com/RicoGama/ceasa-fruit/api/internal/certificate"
	"github.com/RicoGama/ceasa-fruit/api/internal/certificate/certtest"
)

func TestParseICPBrasilCertificate(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		pfx, pass := certtest.PFX(certtest.Options{Legacy: legacy})
		p, err := certificate.Parse(pfx, pass, time.Now())
		if err != nil {
			t.Fatalf("legacy=%v: %v", legacy, err)
		}
		if p.HolderName != "IRMAOS FAISAO FRUTICOLA LTDA" || p.Document != "11222333000181" {
			t.Fatalf("legacy=%v: info %+v", legacy, p.Info)
		}
		if p.Issuer != "AC TESTE CEASA FRUIT" || len(p.Fingerprint) != 64 || p.Key == nil || len(p.Chain) != 1 {
			t.Fatalf("legacy=%v: details %+v", legacy, p.Info)
		}
	}
}

func TestParseRejects(t *testing.T) {
	now := time.Now()
	good, pass := certtest.PFX(certtest.Options{})
	cases := []struct {
		name string
		pfx  []byte
		pass string
		want error
	}{
		{"wrong password", good, "errada", certificate.ErrWrongPassword},
		{"not a pfx", []byte("isto não é um certificado"), pass, certificate.ErrInvalidFile},
		{"empty", nil, pass, certificate.ErrInvalidFile},
		{"expired", first(certtest.PFX(certtest.Options{NotBefore: now.AddDate(-2, 0, 0), NotAfter: now.AddDate(0, 0, -1)})), "senha-teste", certificate.ErrExpired},
		{"not yet valid", first(certtest.PFX(certtest.Options{NotBefore: now.Add(48 * time.Hour)})), "senha-teste", certificate.ErrNotYetValid},
		{"without CNPJ (e-CPF)", first(certtest.PFX(certtest.Options{NoCNPJ: true})), "senha-teste", certificate.ErrNoDocument},
		{"without private key", first(certtest.PFX(certtest.Options{WithoutKey: true})), "senha-teste", certificate.ErrNoPrivateKey},
		{"key of another certificate", first(certtest.PFX(certtest.Options{OtherKey: true})), "senha-teste", certificate.ErrKeyMismatch},
	}
	for _, c := range cases {
		if _, err := certificate.Parse(c.pfx, c.pass, now); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func first(b []byte, _ string) []byte { return b }
