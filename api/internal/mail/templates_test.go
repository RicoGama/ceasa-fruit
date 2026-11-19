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
		{"invite", map[string]any{"Name": "Rico", "ResetURL": "http://app.test/reset-password?token=XYZ&invite=1", "ExpiresInDays": 3},
			"Seu acesso ao CEASA Fruit", []string{"Olá, Rico!", "token=XYZ", "3 dias"}},
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
	msg, err := Render("invite", "x@test.com", "", map[string]any{
		"Name": "<script>alert(1)</script> & Cia", "ResetURL": "http://app.test/reset-password?token=A", "ExpiresInDays": 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msg.HTML, "<script>") || !strings.Contains(msg.HTML, "&lt;script&gt;") {
		t.Error("name not escaped in HTML")
	}
	if !strings.Contains(msg.HTML, "&lt;/script&gt; &amp; Cia") || !strings.Contains(msg.Text, "</script> & Cia") {
		t.Error("ampersand handling wrong")
	}
}
