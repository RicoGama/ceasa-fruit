package money

import (
	"encoding/json"
	"testing"

	"github.com/RicoGama/ceasa-fruit/api/internal/quantity"
)

func TestFormatting(t *testing.T) {
	cases := []struct {
		m        Money
		api, brl string
	}{
		{FromCents(123456), "1234.56", "R$ 1.234,56"},
		{FromReais(10), "10.00", "R$ 10,00"},
		{FromCents(5), "0.05", "R$ 0,05"},
		{FromCents(-150), "-1.50", "R$ -1,50"},
		{0, "0.00", "R$ 0,00"},
	}
	for _, c := range cases {
		if c.m.String() != c.api || c.m.Format() != c.brl {
			t.Errorf("%d: %q %q, want %q %q", c.m, c.m.String(), c.m.Format(), c.api, c.brl)
		}
	}
}

// O motivo de existir este tipo: em float, 0.1 + 0.2 != 0.3.
func TestNoFloatErrors(t *testing.T) {
	if MustParse("0.1").Add(MustParse("0.2")) != MustParse("0.3") {
		t.Error("0.1 + 0.2 != 0.3")
	}
	var total Money
	for range 1000 {
		total = total.Add(MustParse("0.01"))
	}
	if total != FromReais(10) {
		t.Errorf("1000 × 0.01 = %s", total)
	}
}

func TestTimes(t *testing.T) {
	cases := []struct{ price, qty, want string }{
		{"4.99", "2.345", "11.70"}, // 11,70155
		{"4.99", "2.346", "11.71"}, // 11,70654
		{"85.00", "3", "255.00"},   // 3 caixas
		{"0.15", "0.5", "0.08"},    // 0,075 -> 0,08 (meio para cima)
		{"1250.00", "0.001", "1.25"},
	}
	for _, c := range cases {
		got, err := MustParse(c.price).Times(quantity.MustParse(c.qty))
		if err != nil || got != MustParse(c.want) {
			t.Errorf("%s × %s = %s, %v; want %s", c.price, c.qty, got, err, c.want)
		}
	}
}

func TestPercent(t *testing.T) {
	cases := []struct{ value, pct, want string }{
		{"1000.00", "12.5", "125.00"},
		{"333.33", "10", "33.33"},  // 33,333
		{"0.05", "50", "0.03"},     // 0,025 -> 0,03
		{"1000.00", "0.9", "9.00"}, // CBS em teste em 2026
		{"200.00", "18", "36.00"},
	}
	for _, c := range cases {
		got, err := MustParse(c.value).Percent(MustParsePercent(c.pct))
		if err != nil || got != MustParse(c.want) {
			t.Errorf("%s × %s%% = %s, %v; want %s", c.value, c.pct, got, err, c.want)
		}
	}
	p := MustParsePercent("12.5")
	if p.String() != "12.5" || p.Format() != "12,5%" || !p.Between(0, PercentFromInt(100)) {
		t.Errorf("percent: %q %q", p.String(), p.Format())
	}
	if _, err := ParsePercent("1.23456"); err == nil {
		t.Error("5 decimals should fail")
	}
}

func TestAllocateKeepsTheTotal(t *testing.T) {
	cases := []struct {
		total string
		n     int
		want  []string
	}{
		{"100.00", 3, []string{"33.34", "33.33", "33.33"}},
		{"0.05", 3, []string{"0.02", "0.02", "0.01"}},
		{"-100.00", 3, []string{"-33.34", "-33.33", "-33.33"}},
		{"90.00", 2, []string{"45.00", "45.00"}},
	}
	for _, c := range cases {
		parts, err := MustParse(c.total).Allocate(c.n)
		if err != nil {
			t.Fatal(err)
		}
		if Sum(parts...) != MustParse(c.total) {
			t.Errorf("%s / %d: sum = %s", c.total, c.n, Sum(parts...))
		}
		for i, p := range parts {
			if p != MustParse(c.want[i]) {
				t.Errorf("%s / %d: part %d = %s, want %s", c.total, c.n, i, p, c.want[i])
			}
		}
	}
	if _, err := FromReais(1).Allocate(0); err == nil {
		t.Error("allocate 0 should fail")
	}
}

func TestJSON(t *testing.T) {
	var v struct {
		Price Money    `json:"price"`
		Fee   *Money   `json:"fee"`
		Rate  *Percent `json:"rate"`
	}
	if err := json.Unmarshal([]byte(`{"price":"1234.5","fee":null,"rate":"12.5"}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.Price != FromCents(123450) || v.Fee != nil || *v.Rate != MustParsePercent("12.5") {
		t.Fatalf("%+v", v)
	}
	b, _ := json.Marshal(v)
	if string(b) != `{"price":"1234.50","fee":null,"rate":"12.5"}` {
		t.Errorf("marshal = %s", b)
	}
	for _, bad := range []string{`{"price":1234.5}`, `{"price":"1.234,50"}`, `{"price":"1.005"}`} {
		if err := json.Unmarshal([]byte(bad), &v); err == nil {
			t.Errorf("%s should fail", bad)
		}
	}
}
