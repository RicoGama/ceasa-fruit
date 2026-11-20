// Package fixed é o núcleo dos números decimais exatos do sistema (dinheiro,
// quantidade, percentual). Não use direto: use money.Money, money.Percent e
// quantity.Quantity, que são feitos em cima dele.
//
// Por que não float64? Porque float guarda os números em base 2, e a maioria
// dos decimais não tem representação exata: em Go, 0.1 + 0.2 dá
// 0.30000000000000004. Em nota fiscal isso vira rejeição na SEFAZ; no caixa,
// centavo sobrando ou faltando.
//
// A solução é "ponto fixo": guardar um inteiro e combinar quantas casas
// decimais ele tem (a escala). Com escala 2, R$ 12,34 é guardado como 1234
// (centavos). Soma e subtração de inteiros são sempre exatas; só a
// multiplicação precisa arredondar, e isso acontece num lugar só (MulDiv).
package fixed

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// Erros devolvidos pelas conversões. São variáveis (e não strings soltas)
// para quem chama poder testar com errors.Is.
var (
	ErrSyntax    = errors.New("invalid decimal number")
	ErrPrecision = errors.New("too many decimal places")
	ErrRange     = errors.New("number out of range")
)

// pow10 devolve 10^n para n pequeno (até 18 cabe em int64).
func pow10(n int) int64 {
	p := int64(1)
	for range n {
		p *= 10
	}
	return p
}

// Parse lê o formato canônico da API: dígitos, ponto como separador decimal,
// sem separador de milhar ("1234.56", "-0.5", "7"). Casas decimais além da
// escala são erro, não arredondamento: quem digitou 0,005 em reais errou, e
// é melhor avisar do que sumir com o valor.
//
// O formato brasileiro ("1.234,56") é convertido pelo front antes de enviar:
// "1.234" é ambíguo (mil e duzentos? um vírgula dois?), por isso a API não aceita.
func Parse(s string, scale int) (int64, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	intPart, fracPart, hasDot := strings.Cut(s, ".")
	if intPart == "" || (hasDot && fracPart == "") || !allDigits(intPart) || !allDigits(fracPart) {
		return 0, fmt.Errorf("%w: %q", ErrSyntax, s)
	}
	if len(fracPart) > scale {
		// Zeros sobrando no fim não mudam o valor ("1.500" com escala 2 é 1.50).
		extra := fracPart[scale:]
		if strings.Trim(extra, "0") != "" {
			return 0, fmt.Errorf("%w: %q has more than %d", ErrPrecision, s, scale)
		}
		fracPart = fracPart[:scale]
	}
	// Completa com zeros à direita até a escala: "1.5" com escala 2 -> "1" e "50".
	fracPart += strings.Repeat("0", scale-len(fracPart))

	n, ok := new(big.Int).SetString(intPart+fracPart, 10)
	if !ok || !n.IsInt64() {
		return 0, fmt.Errorf("%w: %q", ErrRange, s)
	}
	v := n.Int64()
	if neg {
		v = -v
	}
	return v, nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Format escreve v (com a escala dada) como texto.
//   - decimalSep: "." no formato da API, "," no brasileiro;
//   - thousandsSep: "" na API, "." no brasileiro;
//   - minDecimals: casas sempre mostradas (dinheiro = 2); as demais só
//     aparecem se não forem zero ("12.5" kg e não "12.500").
func Format(v int64, scale int, decimalSep, thousandsSep string, minDecimals int) string {
	neg := v < 0
	// uint64 evita o estouro de -math.MinInt64.
	u := uint64(v)
	if neg {
		u = -u
	}
	p := uint64(pow10(scale))
	intPart := fmt.Sprint(u / p)
	frac := fmt.Sprintf("%0*d", scale, u%p)

	frac = strings.TrimRight(frac, "0")
	if len(frac) < minDecimals {
		frac += strings.Repeat("0", minDecimals-len(frac))
	}

	if thousandsSep != "" {
		intPart = groupThousands(intPart, thousandsSep)
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	b.WriteString(intPart)
	if frac != "" {
		b.WriteString(decimalSep)
		b.WriteString(frac)
	}
	return b.String()
}

// groupThousands põe o separador a cada 3 dígitos: "1234567" -> "1.234.567".
func groupThousands(digits, sep string) string {
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	first := len(digits) % 3
	if first > 0 {
		b.WriteString(digits[:first])
	}
	for i := first; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteString(sep)
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// MulDiv calcula a * b / div arredondando "meio para cima" (0,5 vira 1;
// -0,5 vira -1), o arredondamento comercial. É a ÚNICA função que arredonda:
// todo cálculo com multiplicação (preço × quantidade, valor × percentual)
// passa por aqui. A conta intermediária usa big.Int, então não estoura.
func MulDiv(a, b, div int64) (int64, error) {
	if div == 0 {
		return 0, errors.New("fixed: division by zero")
	}
	num := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	d := big.NewInt(div)

	// QuoRem trunca em direção ao zero; depois ajustamos pelo resto.
	q, r := new(big.Int).QuoRem(num, d, new(big.Int))
	// |resto| * 2 >= |divisor| -> a parte cortada era 0,5 ou mais.
	twice := new(big.Int).Mul(new(big.Int).Abs(r), big.NewInt(2))
	if twice.Cmp(new(big.Int).Abs(d)) >= 0 {
		if (num.Sign() < 0) != (d.Sign() < 0) {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	if !q.IsInt64() {
		return 0, ErrRange
	}
	return q.Int64(), nil
}

// Add soma checando estouro (em dinheiro, um estouro silencioso seria um
// valor negativo absurdo; melhor falhar alto).
func Add(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, ErrRange
	}
	return a + b, nil
}

// ---------------------------------------------------------------------------
// Conversão de/para o NUMERIC do PostgreSQL
// ---------------------------------------------------------------------------

// FromNumeric converte o NUMERIC lido do banco para inteiro na escala dada.
// O pgx entrega o número como Int × 10^Exp (ex.: 12.50 = 1250 × 10^-2).
func FromNumeric(n pgtype.Numeric, scale int) (int64, error) {
	if !n.Valid {
		return 0, errors.New("fixed: cannot scan NULL; use a pointer for nullable columns")
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite {
		return 0, fmt.Errorf("%w: NaN or infinity", ErrRange)
	}
	v := new(big.Int).Set(n.Int)
	shift := int(n.Exp) + scale // quantas casas falta "andar" para chegar na escala
	switch {
	case shift > 0:
		v.Mul(v, big.NewInt(pow10(shift)))
	case shift < 0:
		// O banco mandou mais casas que a escala (ex.: 1.005 para dinheiro):
		// só aceitamos se as casas extras forem zero.
		q, r := new(big.Int).QuoRem(v, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-shift)), nil), new(big.Int))
		if r.Sign() != 0 {
			return 0, ErrPrecision
		}
		v = q
	}
	if !v.IsInt64() {
		return 0, ErrRange
	}
	return v.Int64(), nil
}

// ToNumeric converte o inteiro na escala dada para o NUMERIC do banco.
func ToNumeric(v int64, scale int) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(v), Exp: int32(-scale), Valid: true}
}
