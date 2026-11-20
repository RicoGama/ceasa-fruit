// Package quantity tem o tipo Quantity: uma quantidade decimal exata com até
// 3 casas (milésimos). Serve para quilos (12,345 kg = 12 kg e 345 g), caixas,
// dúzias, pallets... A UNIDADE não fica aqui: ela é do produto ou do item do
// pedido. Quantity é só o número, sem os problemas de arredondamento do float.
//
// No banco: coluna do tipo quantity_amount (NUMERIC(14,3)); o sqlc gera
// quantity.Quantity sozinho para essas colunas.
// Na API (JSON): texto com ponto decimal, ex. "12.5" (ver money.Money).
package quantity

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/RicoGama/ceasa-fruit/api/internal/fixed"
)

// Scale é o número de casas decimais: 3 (milésimos).
const Scale = 3

// Quantity guarda a quantidade em milésimos: 1 kg = 1000, 0,5 cx = 500.
type Quantity int64

// One é uma unidade inteira (1 kg, 1 caixa).
const One Quantity = 1000

// FromInt cria uma quantidade inteira: FromInt(3) = 3 caixas.
func FromInt(n int64) Quantity { return Quantity(n) * One }

// FromThousandths cria a partir de milésimos: FromThousandths(1500) = 1,5.
func FromThousandths(n int64) Quantity { return Quantity(n) }

// Parse lê o formato da API ("12.5", "3"). Mais de 3 casas é erro.
func Parse(s string) (Quantity, error) {
	v, err := fixed.Parse(s, Scale)
	return Quantity(v), err
}

// MustParse é para constantes e testes: entra em pânico se o texto for inválido.
func MustParse(s string) Quantity {
	q, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return q
}

// Thousandths devolve o valor em milésimos (para cálculos de baixo nível).
func (q Quantity) Thousandths() int64 { return int64(q) }

// String é o formato da API: "12.5", "3", "-0.25".
func (q Quantity) String() string { return fixed.Format(int64(q), Scale, ".", "", 0) }

// Format é o formato brasileiro para telas e documentos: "1.234,5".
func (q Quantity) Format() string { return fixed.Format(int64(q), Scale, ",", ".", 0) }

func (q Quantity) Add(o Quantity) Quantity { return q + o }
func (q Quantity) Sub(o Quantity) Quantity { return q - o }
func (q Quantity) Neg() Quantity           { return -q }
func (q Quantity) IsZero() bool            { return q == 0 }
func (q Quantity) IsNegative() bool        { return q < 0 }
func (q Quantity) IsPositive() bool        { return q > 0 }

// IsWhole diz se não tem parte fracionária (útil para "caixa não se vende pela metade").
func (q Quantity) IsWhole() bool { return q%One == 0 }

// MulInt multiplica por um inteiro: 3 pallets × 50 caixas.
func (q Quantity) MulInt(n int64) Quantity { return q * Quantity(n) }

// Mul multiplica duas quantidades, arredondando para 3 casas
// (ex.: 2,5 caixas × 18,2 kg por caixa = 45,5 kg).
func (q Quantity) Mul(o Quantity) (Quantity, error) {
	v, err := fixed.MulDiv(int64(q), int64(o), int64(One))
	return Quantity(v), err
}

// ---------------------------------------------------------------------------
// JSON e OpenAPI
// ---------------------------------------------------------------------------

// MarshalJSON grava como texto ("12.5"), nunca como número: o JavaScript
// também usa float, e o texto chega exato até o front.
func (q Quantity) MarshalJSON() ([]byte, error) { return json.Marshal(q.String()) }

func (q *Quantity) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("quantity must be a string like \"12.5\": %w", err)
	}
	v, err := Parse(s)
	if err != nil {
		return err
	}
	*q = v
	return nil
}

// Schema diz ao Huma como o tipo aparece no OpenAPI (texto com até 3 casas);
// o Huma valida o formato antes de chegar no handler.
func (Quantity) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        huma.TypeString,
		Pattern:     `^-?\d{1,11}(\.\d{1,3})?$`,
		Description: "Decimal quantity with up to 3 places, dot as separator",
		Examples:    []any{"12.5"},
	}
}

// ---------------------------------------------------------------------------
// Banco de dados (pgx)
// ---------------------------------------------------------------------------

// ScanNumeric é chamado pelo pgx ao ler uma coluna NUMERIC.
func (q *Quantity) ScanNumeric(n pgtype.Numeric) error {
	v, err := fixed.FromNumeric(n, Scale)
	if err != nil {
		return fmt.Errorf("scanning quantity: %w", err)
	}
	*q = Quantity(v)
	return nil
}

// NumericValue é chamado pelo pgx ao gravar numa coluna NUMERIC.
func (q Quantity) NumericValue() (pgtype.Numeric, error) {
	return fixed.ToNumeric(int64(q), Scale), nil
}

// Value é a alternativa genérica (database/sql), usada quando o pgx não sabe
// o tipo do parâmetro: manda como texto e o PostgreSQL converte.
func (q Quantity) Value() (driver.Value, error) { return q.String(), nil }
