package party

import (
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

// locations devolve os campos com erro, para comparar nos testes.
func locations(errs []error) []string {
	var out []string
	for _, e := range errs {
		if d, ok := e.(*huma.ErrorDetail); ok {
			out = append(out, d.Location+": "+d.Message)
		}
	}
	return out
}

func TestNormalizeCleansAndFormats(t *testing.T) {
	f := Fields{
		PersonType:        "individual",
		Document:          new(" 529.982.247-25 "),
		LegalName:         "  José da Silva ",
		TradeName:         new("   "), // vazio vira nil
		Email:             new("ZE@Sitio.COM"),
		Phone:             new("(21) 99999-0000"),
		StateRegistration: new("isento"),
		Address: AddressInput{
			PostalCode: new("25960-000"),
			Street:     new(" Estrada da Barragem "),
		},
		Payment:  PaymentInput{PixKeyType: new("cpf"), PixKey: new("529.982.247-25")},
		Contacts: []ContactInput{{Name: " Maria ", Phone: new("21 98888-7777"), Email: new("Maria@X.com")}},
	}

	if errs := f.Normalize(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", locations(errs))
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"document", *f.Document, "52998224725"},
		{"legalName", f.LegalName, "José da Silva"},
		{"tradeName", f.TradeName == nil, true},
		{"email", *f.Email, "ze@sitio.com"},
		{"phone", *f.Phone, "21999990000"},
		{"stateRegistration", *f.StateRegistration, "ISENTO"},
		{"taxpayerType default", f.TaxpayerType, "non_taxpayer"},
		{"postalCode", *f.Address.PostalCode, "25960000"},
		{"street", *f.Address.Street, "Estrada da Barragem"},
		{"pixKey", *f.Payment.PixKey, "52998224725"},
		{"contact name", f.Contacts[0].Name, "Maria"},
		{"contact phone", *f.Contacts[0].Phone, "21988887777"},
		{"contact email", *f.Contacts[0].Email, "maria@x.com"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestNormalizeErrors(t *testing.T) {
	base := func() Fields { return Fields{PersonType: "company", LegalName: "Box 21 Ltda"} }

	cases := []struct {
		name   string
		modify func(f *Fields)
		want   string // trecho esperado em "local: mensagem"
	}{
		{"invalid document", func(f *Fields) { f.Document = new("11.222.333/0001-80") }, "body.document: invalid CPF or CNPJ"},
		{"company with CPF", func(f *Fields) { f.Document = new("529.982.247-25") }, "body.document: a company must have a CNPJ"},
		{"individual with CNPJ", func(f *Fields) {
			f.PersonType = "individual"
			f.Document = new("11.222.333/0001-81")
		}, "body.document: an individual must have a CPF"},
		{"short name after trim", func(f *Fields) { f.LegalName = "  A  " }, "body.legalName"},
		{"bad postal code", func(f *Fields) { f.Address.PostalCode = new("2596-000") }, "body.address.postalCode: invalid postal code"},
		{"bad phone", func(f *Fields) { f.Phone = new("123") }, "body.phone: invalid phone"},
		{"pix key without type", func(f *Fields) { f.Payment.PixKey = new("x@y.com") }, "body.payment.pixKeyType"},
		{"pix type without key", func(f *Fields) { f.Payment.PixKeyType = new("email") }, "body.payment.pixKey: pix key is required"},
		{"pix cpf invalid", func(f *Fields) {
			f.Payment.PixKeyType = new("cpf")
			f.Payment.PixKey = new("11122233344")
		}, "body.payment.pixKey: invalid pix key"},
		{"pix random not uuid", func(f *Fields) {
			f.Payment.PixKeyType = new("random")
			f.Payment.PixKey = new("abc")
		}, "body.payment.pixKey: invalid pix key"},
		{"holder document invalid", func(f *Fields) { f.Payment.BankAccountHolderDocument = new("123") }, "body.payment.bankAccountHolderDocument"},
		{"contact without name", func(f *Fields) { f.Contacts = []ContactInput{{Name: "ok"}, {Name: "   "}} }, "body.contacts[1].name"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := base()
			c.modify(&f)
			got := strings.Join(locations(f.Normalize()), "\n")
			if !strings.Contains(got, c.want) {
				t.Errorf("errors = %q, want something containing %q", got, c.want)
			}
		})
	}
}

func TestNormalizeValidPixKeys(t *testing.T) {
	cases := []struct{ kind, key, want string }{
		{"cnpj", "11.222.333/0001-81", "11222333000181"},
		{"email", "Pix@Empresa.com", "pix@empresa.com"},
		{"phone", "+55 (21) 99999-0000", "5521999990000"},
		{"random", "123E4567-E89B-12D3-A456-426614174000", "123e4567-e89b-12d3-a456-426614174000"},
	}
	for _, c := range cases {
		f := Fields{PersonType: "company", LegalName: "Teste", Payment: PaymentInput{PixKeyType: &c.kind, PixKey: new(c.key)}}
		if errs := f.Normalize(); len(errs) > 0 {
			t.Errorf("%s: unexpected errors %v", c.kind, locations(errs))
			continue
		}
		if *f.Payment.PixKey != c.want {
			t.Errorf("%s: key = %q, want %q", c.kind, *f.Payment.PixKey, c.want)
		}
	}
}
