package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=1,p=4$") {
		t.Errorf("unexpected hash format: %s", hash)
	}

	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil || !ok {
		t.Errorf("correct password rejected: ok=%v err=%v", ok, err)
	}

	ok, err = VerifyPassword(hash, "wrong password")
	if err != nil || ok {
		t.Errorf("wrong password accepted: ok=%v err=%v", ok, err)
	}
}

func TestHashPasswordUsesRandomSalt(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Error("two hashes of the same password must differ (random salt)")
	}
}

func TestVerifyPasswordInvalidHash(t *testing.T) {
	for _, h := range []string{"", "plain", "$argon2id$v=19$bad$salt$hash", "$bcrypt$x$y$z$w"} {
		if _, err := VerifyPassword(h, "x"); err == nil {
			t.Errorf("VerifyPassword(%q) should fail", h)
		}
	}
}
