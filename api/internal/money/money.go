// Package money tem os tipos de valor monetário (Money) e percentual (Percent).
//
// Regras do projeto:
//   - todo valor em reais é money.Money; nunca float64;
//   - no banco, a coluna é do tipo money_amount (NUMERIC(14,2)) e o sqlc gera
//     money.Money sozinho; percentuais usam percent_rate (NUMERIC(7,4));
//   - na API (JSON), o valor vai como TEXTO com ponto decimal: "1234.56".
//     Texto porque o JavaScript também usa float: como número, 0.1 + 0.2 no
//     navegador daria a mesma conta errada. O front formata para "R$ 1.234,56";
//   - arredondamento só acontece em multiplicação (preço × quantidade, valor ×
//     percentual), sempre "meio para cima", em fixed.MulDiv.
package money

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/RicoGama/ceasa-fruit/api/internal/fixed"
	"github.com/RicoGama/ceasa-fruit/api/internal/quantity"
)

// Scale: dinheiro tem 2 casas (centavos).
const Scale = 2

// Money é um valor em reais guardado em centavos: R$ 12,34 = 1234.
// O valor zero (Money(0)) é R$ 0,00 e está pronto para uso.
type Money int64

// FromCents cria a partir de centavos: FromCents(1234) = R$ 12,34.
func FromCents(c int64) Money { return Money(c) }

// FromReais cria a partir de reais inteiros: FromReais(10) = R$ 10,00.
func FromReais(r int64) Money { return Money(r * 100) }

// Parse lê o formato da API ("1234.56", "10", "-0.5"). Mais de 2 casas é erro.
func Parse(s string) (Money, error) {
	v, err := fixed.Parse(s, Scale)
	return Money(v), err
}

// MustParse é para constantes e testes: entra em pânico se o texto for inválido.
func MustParse(s string) Money {
	m, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return m
}

// Cents devolve o valor em centavos.
func (m Money) Cents() int64 { return int64(m) }

// String é o formato da API e do XML da NF-e: sempre 2 casas, ponto decimal
// ("1234.50"). É também o que aparece no fmt.Println e nos logs.
func (m Money) String() string { return fixed.Format(int64(m), Scale, ".", "", Scale) }

// Format é o formato brasileiro para e-mails, PDFs e o DANFE: "R$ 1.234,50".
func (m Money) Format() string {
	return "R$ " + fixed.Format(int64(m), Scale, ",", ".", Scale)
}

func (m Money) Add(o Money) Money    { return m + o }
func (m Money) Sub(o Money) Money    { return m - o }
func (m Money) Neg() Money           { return -m }
func (m Money) IsZero() bool         { return m == 0 }
func (m Money) IsNegative() bool     { return m < 0 }
func (m Money) IsPositive() bool     { return m > 0 }
func (m Money) MulInt(n int64) Money { return m * Money(n) }

// Sum soma uma lista de valores (ex.: os itens de um pedido).
func Sum(values ...Money) Money {
	var total Money
	for _, v := range values {
		total += v
	}
	return total
}

// Times é preço × quantidade, arredondado para o centavo.
// Ex.: R$ 4,99/kg × 2,345 kg = 11,70155 -> R$ 11,70.
//
// Na NF-e, o valor do item é exatamente esta conta (vProd = qCom × vUnCom).
func (m Money) Times(q quantity.Quantity) (Money, error) {
	v, err := fixed.MulDiv(int64(m), q.Thousandths(), int64(quantity.One))
	return Money(v), err
}

// Percent aplica um percentual, arredondado para o centavo.
// Ex.: comissão de 12,5% sobre R$ 1.000,00 = R$ 125,00.
func (m Money) Percent(p Percent) (Money, error) {
	// p está em 1/10.000 de ponto percentual: 12,5% = 125000; dividir por
	// 100 (por ser %) e por 10.000 (a escala) = dividir por 1.000.000.
	v, err := fixed.MulDiv(int64(m), int64(p), 100*percentOne)
	return Money(v), err
}

// ErrAllocate é devolvido por Allocate quando não há parcelas.
var ErrAllocate = errors.New("money: allocate needs at least one part")

// Allocate divide o valor em n partes que somam EXATAMENTE o total; os
// centavos que sobram vão para as primeiras parcelas.
// Ex.: R$ 100,00 em 3 parcelas = 33,34 + 33,33 + 33,33 (e não 3 × 33,33 = 99,99).
func (m Money) Allocate(n int) ([]Money, error) {
	if n <= 0 {
		return nil, ErrAllocate
	}
	base := m / Money(n)
	rest := m % Money(n) // mesmo sinal de m
	unit := Money(1)
	if rest < 0 {
		unit, rest = -1, -rest
	}
	parts := make([]Money, n)
	for i := range parts {
		parts[i] = base
		if Money(i) < rest {
			parts[i] += unit
		}
	}
	return parts, nil
}

// ---------------------------------------------------------------------------
// JSON, OpenAPI e banco (o mesmo padrão em todos os tipos de ponto fixo)
// ---------------------------------------------------------------------------

func (m Money) MarshalJSON() ([]byte, error) { return json.Marshal(m.String()) }

func (m *Money) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("money must be a string like \"1234.56\": %w", err)
	}
	v, err := Parse(s)
	if err != nil {
		return err
	}
	*m = v
	return nil
}

// Schema descreve o tipo no OpenAPI: texto com até 2 casas decimais.
func (Money) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        huma.TypeString,
		Pattern:     `^-?\d{1,12}(\.\d{1,2})?$`,
		Description: "Amount in BRL, dot as decimal separator, up to 2 places",
		Examples:    []any{"1234.56"},
	}
}

func (m *Money) ScanNumeric(n pgtype.Numeric) error {
	v, err := fixed.FromNumeric(n, Scale)
	if err != nil {
		return fmt.Errorf("scanning money: %w", err)
	}
	*m = Money(v)
	return nil
}

func (m Money) NumericValue() (pgtype.Numeric, error) { return fixed.ToNumeric(int64(m), Scale), nil }
func (m Money) Value() (driver.Value, error)          { return m.String(), nil }

// ===========================================================================
// Percent
// ===========================================================================

// PercentScale: percentuais têm 4 casas (alíquotas de IBS/CBS, ex. 0,9000%).
const PercentScale = 4

const percentOne = 10_000 // 1% = 10.000

// Percent é um percentual com 4 casas: 12,5% = 125000.
// É o número "como se fala": Percent de 12,5 é 12,5% (e não 0,125).
type Percent int64

// PercentFromInt cria um percentual inteiro: PercentFromInt(18) = 18%.
func PercentFromInt(n int64) Percent { return Percent(n * percentOne) }

// ParsePercent lê o formato da API ("12.5" = 12,5%). Mais de 4 casas é erro.
func ParsePercent(s string) (Percent, error) {
	v, err := fixed.Parse(s, PercentScale)
	return Percent(v), err
}

// MustParsePercent é para constantes e testes.
func MustParsePercent(s string) Percent {
	p, err := ParsePercent(s)
	if err != nil {
		panic(err)
	}
	return p
}

// String é o formato da API: "12.5" (sem o símbolo %).
func (p Percent) String() string { return fixed.Format(int64(p), PercentScale, ".", "", 0) }

// Format é o formato brasileiro: "12,5%".
func (p Percent) Format() string { return fixed.Format(int64(p), PercentScale, ",", ".", 0) + "%" }

// Between diz se está no intervalo [min, max] (ex.: comissão entre 0% e 100%).
func (p Percent) Between(min, max Percent) bool { return p >= min && p <= max }

func (p Percent) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }

func (p *Percent) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("percent must be a string like \"12.5\": %w", err)
	}
	v, err := ParsePercent(s)
	if err != nil {
		return err
	}
	*p = v
	return nil
}

func (Percent) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        huma.TypeString,
		Pattern:     `^-?\d{1,3}(\.\d{1,4})?$`,
		Description: "Percentage (12.5 = 12.5%), dot as decimal separator, up to 4 places",
		Examples:    []any{"12.5"},
	}
}

func (p *Percent) ScanNumeric(n pgtype.Numeric) error {
	v, err := fixed.FromNumeric(n, PercentScale)
	if err != nil {
		return fmt.Errorf("scanning percent: %w", err)
	}
	*p = Percent(v)
	return nil
}

func (p Percent) NumericValue() (pgtype.Numeric, error) {
	return fixed.ToNumeric(int64(p), PercentScale), nil
}
func (p Percent) Value() (driver.Value, error) { return p.String(), nil }
