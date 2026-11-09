package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// Parâmetros do Argon2id (recomendação da RFC 9106 para servidores):
// 64 MiB de memória, 1 iteração, 4 threads. Quanto mais caro, mais difícil
// é testar senhas por força bruta se o banco vazar.
const (
	argonMemory  = 64 * 1024 // em KiB
	argonTime    = 1
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

var errInvalidHash = errors.New("invalid password hash format")

// HashPassword gera um hash no formato PHC, que guarda junto os parâmetros e o salt:
//
//	$argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
//
// Assim dá para aumentar os parâmetros no futuro sem invalidar senhas antigas.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword confere a senha contra um hash gerado por HashPassword.
func VerifyPassword(encoded, password string) (bool, error) {
	// strings.Split("$a$b", "$") -> ["", "a", "b"]: por isso esperamos 6 partes.
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errInvalidHash
	}

	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, errInvalidHash
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false, errInvalidHash
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil {
		return false, errInvalidHash
	}

	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))

	// Comparação em tempo constante: não revela, pelo tempo de resposta,
	// quantos bytes iniciais estavam certos.
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyHash é usado quando o e-mail não existe no login: calculamos um hash
// mesmo assim, para que "e-mail inexistente" e "senha errada" demorem o mesmo
// tempo e ninguém descubra quais e-mails estão cadastrados.
//
// sync.OnceValue roda a função uma única vez, na primeira chamada.
var dummyHash = sync.OnceValue(func() string {
	h, _ := HashPassword("dummy-password-for-timing")
	return h
})
