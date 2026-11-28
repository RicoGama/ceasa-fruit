// Package secrets criptografa segredos guardados no banco (hoje: o
// certificado digital e a senha dele).
//
// Por quê: o banco vai para os backups, para dumps de teste e pode ser lido por
// quem tiver acesso ao Postgres. O arquivo do certificado + a senha permitem
// assinar notas fiscais em nome da empresa. Criptografado, quem só tem o banco
// não tem o certificado: falta a chave, que fica em SECRETS_KEY no arquivo de
// configuração do servidor (/etc/ceasa-fruit/ceasa.env, só o sistema lê).
//
// Como: AES-256-GCM (criptografa e detecta qualquer alteração). Cada valor
// leva um "rótulo" (ex.: "certificate.pfx"): um valor copiado para outro
// campo não abre.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// ErrNotConfigured: SECRETS_KEY não foi definida.
var ErrNotConfigured = errors.New("secrets key not configured (SECRETS_KEY)")

// ErrWrongKey: o valor foi criptografado com outra chave (ex.: banco
// restaurado num servidor novo sem a chave antiga) ou foi alterado.
var ErrWrongKey = errors.New("secret was encrypted with another key or was tampered with")

// MinKeyLength: a chave é um texto aleatório; abaixo disso é fraca demais.
const MinKeyLength = 32

// Box guarda a chave pronta para uso. Pode ser usado por várias goroutines.
type Box struct {
	aead cipher.AEAD
	id   string
}

// New prepara a chave. Texto vazio devolve ErrNotConfigured.
func New(key string) (*Box, error) {
	if key == "" {
		return nil, ErrNotConfigured
	}
	if len(key) < MinKeyLength {
		return nil, errors.New("SECRETS_KEY too short: use at least 32 random characters")
	}
	// O texto da configuração vira uma chave de 32 bytes (AES-256).
	sum := sha256.Sum256([]byte("ceasa-fruit/secrets/v1\x00" + key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	idSum := sha256.Sum256(sum[:])
	return &Box{aead: aead, id: hex.EncodeToString(idSum[:4])}, nil
}

// KeyID identifica a chave sem revelá-la. Fica gravado junto do segredo para
// dar uma mensagem clara quando a chave do servidor não é a mesma.
func (b *Box) KeyID() string { return b.id }

// Seal criptografa. O resultado é: nonce aleatório + texto criptografado.
func (b *Box) Seal(plaintext []byte, label string) []byte {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err) // crypto/rand não falha em sistemas suportados
	}
	return b.aead.Seal(nonce, nonce, plaintext, []byte(label))
}

// Open desfaz o Seal. Chave errada, rótulo errado ou dado alterado: ErrWrongKey.
func (b *Box) Open(sealed []byte, label string) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return nil, ErrWrongKey
	}
	out, err := b.aead.Open(nil, sealed[:n], sealed[n:], []byte(label))
	if err != nil {
		return nil, ErrWrongKey
	}
	return out, nil
}
