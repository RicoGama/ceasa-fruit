package document

import "testing"

// A "table-driven" test: the most common pattern in Go. A slice of cases and a loop.
func TestIsValid(t *testing.T) {
	cases := []struct {
		name  string
		doc   string
		valid bool
	}{
		{"valid cpf", "529.982.247-25", true},
		{"cpf wrong check digit", "529.982.247-24", false},
		{"cpf all same digits", "111.111.111-11", false},
		{"valid numeric cnpj", "11.222.333/0001-81", true},
		{"numeric cnpj wrong check digit", "11.222.333/0001-80", false},
		{"valid alphanumeric cnpj", "12.ABC.345/01DE-35", true},
		{"alphanumeric cnpj lowercase", "12.abc.345/01de-35", true},
		{"alphanumeric cnpj wrong check digit", "12.ABC.345/01DE-36", false},
		{"cnpj with letter in check digit", "12ABC34501DE3A", false},
		{"zeroed cnpj", "00.000.000/0000-00", false},
		{"invalid length", "123", false},
		{"empty", "", false},
	}

	for _, c := range cases {
		// t.Run creates a named subtest, shown separately in `go test -v`.
		t.Run(c.name, func(t *testing.T) {
			got := IsValid(Normalize(c.doc))
			if got != c.valid {
				t.Errorf("IsValid(%q) = %v, want %v", c.doc, got, c.valid)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	if got := Normalize(" 12.abc.345/01de-35 "); got != "12ABC34501DE35" {
		t.Errorf("Normalize = %q", got)
	}
}
