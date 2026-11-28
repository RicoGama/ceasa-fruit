package secrets

import (
	"bytes"
	"errors"
	"testing"
)

const key = "chave-de-teste-com-mais-de-32-caracteres"

func TestSealOpen(t *testing.T) {
	b, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed := b.Seal([]byte("segredo"), "certificate.pfx")
	if bytes.Contains(sealed, []byte("segredo")) {
		t.Fatal("plaintext visible in the sealed value")
	}
	// Duas vezes o mesmo valor: resultados diferentes (nonce aleatório).
	if bytes.Equal(sealed, b.Seal([]byte("segredo"), "certificate.pfx")) {
		t.Fatal("same output twice: nonce is not random")
	}
	got, err := b.Open(sealed, "certificate.pfx")
	if err != nil || string(got) != "segredo" {
		t.Fatalf("open: %q %v", got, err)
	}

	// Rótulo errado, dado alterado, outra chave: nada abre.
	if _, err := b.Open(sealed, "certificate.password"); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("wrong label: %v", err)
	}
	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	if _, err := b.Open(tampered, "certificate.pfx"); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("tampered: %v", err)
	}
	other, _ := New(key + "-outra")
	if _, err := other.Open(sealed, "certificate.pfx"); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("other key: %v", err)
	}
	if other.KeyID() == b.KeyID() {
		t.Fatal("different keys with the same id")
	}
}

func TestNewRejectsMissingOrShortKey(t *testing.T) {
	if _, err := New(""); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := New("curta"); err == nil {
		t.Fatal("short key accepted")
	}
}
