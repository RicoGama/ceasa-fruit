package fixed

import (
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in    string
		scale int
		want  int64
		err   error
	}{
		{"1234.56", 2, 123456, nil},
		{"1234.5", 2, 123450, nil},
		{"7", 2, 700, nil},
		{"-0.05", 2, -5, nil},
		{" 3.10 ", 2, 310, nil},
		{"1.500", 2, 150, nil}, // zeros sobrando no fim não mudam o valor
		{"0.001", 3, 1, nil},
		{"1.005", 2, 0, ErrPrecision},
		{"1.234,56", 2, 0, ErrSyntax}, // formato brasileiro não é aceito na API
		{"1,5", 2, 0, ErrSyntax},
		{"", 2, 0, ErrSyntax},
		{"-", 2, 0, ErrSyntax},
		{"1.", 2, 0, ErrSyntax},
		{".5", 2, 0, ErrSyntax},
		{"1e3", 2, 0, ErrSyntax},
		{"+5", 2, 0, ErrSyntax},
		{"99999999999999999999", 2, 0, ErrRange},
	}
	for _, c := range cases {
		got, err := Parse(c.in, c.scale)
		if !errors.Is(err, c.err) || (c.err == nil && got != c.want) {
			t.Errorf("Parse(%q, %d) = %d, %v; want %d, %v", c.in, c.scale, got, err, c.want, c.err)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		v         int64
		scale     int
		dec, thou string
		minDec    int
		want      string
	}{
		{123456, 2, ".", "", 2, "1234.56"},
		{123450, 2, ".", "", 2, "1234.50"},
		{5, 2, ".", "", 2, "0.05"},
		{-5, 2, ".", "", 2, "-0.05"},
		{123456789, 2, ",", ".", 2, "1.234.567,89"},
		{100000, 2, ",", ".", 2, "1.000,00"},
		{12500, 3, ".", "", 0, "12.5"},
		{3000, 3, ".", "", 0, "3"},
		{0, 2, ".", "", 2, "0.00"},
		{math.MinInt64, 2, ".", "", 2, "-92233720368547758.08"},
	}
	for _, c := range cases {
		if got := Format(c.v, c.scale, c.dec, c.thou, c.minDec); got != c.want {
			t.Errorf("Format(%d) = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestMulDivRoundsHalfAwayFromZero(t *testing.T) {
	cases := []struct{ a, b, div, want int64 }{
		{499, 2345, 1000, 1170},          // 4,99 × 2,345 = 11,70155 -> 11,70
		{5, 1, 10, 1},                    // 0,5 -> 1
		{4, 1, 10, 0},                    // 0,4 -> 0
		{-5, 1, 10, -1},                  // -0,5 -> -1
		{-4, 1, 10, 0},                   // -0,4 -> 0
		{15, 1, 10, 2},                   // 1,5 -> 2
		{25, 1, 10, 3},                   // 2,5 -> 3 (não é o "arredondamento do banqueiro")
		{100000, 125000, 1000000, 12500}, // R$ 1.000,00 × 12,5%
	}
	for _, c := range cases {
		got, err := MulDiv(c.a, c.b, c.div)
		if err != nil || got != c.want {
			t.Errorf("MulDiv(%d, %d, %d) = %d, %v; want %d", c.a, c.b, c.div, got, err, c.want)
		}
	}
	if _, err := MulDiv(math.MaxInt64, 10, 1); !errors.Is(err, ErrRange) {
		t.Errorf("overflow: err = %v, want ErrRange", err)
	}
}

func TestAddOverflow(t *testing.T) {
	if _, err := Add(math.MaxInt64, 1); !errors.Is(err, ErrRange) {
		t.Errorf("err = %v, want ErrRange", err)
	}
	if v, _ := Add(2, -5); v != -3 {
		t.Errorf("Add(2, -5) = %d", v)
	}
}

func TestNumericRoundTrip(t *testing.T) {
	num := func(i int64, exp int32) pgtype.Numeric {
		return pgtype.Numeric{Int: big.NewInt(i), Exp: exp, Valid: true}
	}
	cases := []struct {
		in    pgtype.Numeric
		scale int
		want  int64
		err   error
	}{
		{num(1250, -2), 2, 1250, nil},
		{num(125, -1), 2, 1250, nil}, // 12.5 vindo com uma casa só
		{num(3, 2), 2, 30000, nil},   // 300 (expoente positivo)
		{num(12345, -3), 3, 12345, nil},
		{num(1005, -3), 2, 0, ErrPrecision}, // 1.005 não cabe em centavos
		{num(1000, -3), 2, 100, nil},        // 1.000 cabe
	}
	for _, c := range cases {
		got, err := FromNumeric(c.in, c.scale)
		if !errors.Is(err, c.err) || (c.err == nil && got != c.want) {
			t.Errorf("FromNumeric(%v e%d) = %d, %v; want %d, %v", c.in.Int, c.in.Exp, got, err, c.want, c.err)
		}
	}
	back, _ := FromNumeric(ToNumeric(-98765, 2), 2)
	if back != -98765 {
		t.Errorf("round trip = %d", back)
	}
	if _, err := FromNumeric(pgtype.Numeric{}, 2); err == nil {
		t.Error("NULL should fail")
	}
}
