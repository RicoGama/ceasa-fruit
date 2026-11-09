package mail

import (
	"strings"
	"testing"
)

func TestRenderTemplates(t *testing.T) {
	cases := []struct {
		name     string
		data     any
		subject  string
		contains []string
	}{
		{"welcome", map[string]any{"Name": "Rico", "OrganizationName": "Boa Safra & Cia", "AppURL": "http://app.test"},
			"Bem-vindo ao CEASA Fruit", []string{"Olá, Rico!", "http://app.test/app"}},
		{"password_reset", map[string]any{"Name": "Ana", "ResetURL": "http://app.test/reset-password?token=ABC", "ExpiresInMinutes": 60},
			"Redefinição de senha - CEASA Fruit", []string{"Olá, Ana!", "token=ABC", "60 minutos"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg, err := Render(c.name, "x@test.com", "X", c.data)
			if err != nil {
				t.Fatal(err)
			}
			if msg.Subject != c.subject {
				t.Errorf("subject = %q", msg.Subject)
			}
			for _, want := range c.contains {
				if !strings.Contains(msg.HTML, want) || !strings.Contains(msg.Text, want) {
					t.Errorf("%q missing in HTML or text", want)
				}
			}
		})
	}
}

// O HTML escapa os dados (proteção contra injeção de HTML pelo nome do usuário),
// e o texto puro mostra o valor original.
func TestRenderEscapesHTML(t *testing.T) {
	msg, err := Render("welcome", "x@test.com", "", map[string]any{
		"Name": "<script>alert(1)</script>", "OrganizationName": "Boa Safra & Cia", "AppURL": "http://app.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msg.HTML, "<script>") || !strings.Contains(msg.HTML, "&lt;script&gt;") {
		t.Error("name not escaped in HTML")
	}
	if !strings.Contains(msg.HTML, "Boa Safra &amp; Cia") || !strings.Contains(msg.Text, "Boa Safra & Cia") {
		t.Error("ampersand handling wrong")
	}
}
