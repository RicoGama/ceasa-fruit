package money_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/RicoGama/ceasa-fruit/api/internal/money"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
	"github.com/RicoGama/ceasa-fruit/api/internal/quantity"
)

// Ida e volta no PostgreSQL de verdade, usando os domains da migration 00003.
// Só roda com TEST_DATABASE_URL (ver make test-integration).
func TestDatabaseRoundTrip(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	// Tudo numa transação desfeita no fim: o teste não deixa lixo no banco.
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `CREATE TEMP TABLE t (
			price money_amount NOT NULL, fee money_amount,
			qty quantity_amount NOT NULL, rate percent_rate NOT NULL)`)
		if err != nil {
			return err
		}
		price, qty, rate := money.MustParse("4.99"), quantity.MustParse("2.345"), money.MustParsePercent("0.9")
		for range 3 {
			if _, err := tx.Exec(ctx, "INSERT INTO t VALUES ($1, $2, $3, $4)", price, (*money.Money)(nil), qty, rate); err != nil {
				return err
			}
		}

		var (
			gotPrice money.Money
			gotFee   *money.Money
			gotQty   quantity.Quantity
			gotRate  money.Percent
			total    money.Money
			sumQty   quantity.Quantity
		)
		if err := tx.QueryRow(ctx, "SELECT price, fee, qty, rate FROM t LIMIT 1").Scan(&gotPrice, &gotFee, &gotQty, &gotRate); err != nil {
			return err
		}
		if gotPrice != price || gotFee != nil || gotQty != qty || gotRate != rate {
			t.Errorf("round trip: %s %v %s %s", gotPrice, gotFee, gotQty, gotRate)
		}

		// Conta feita no banco (preço × quantidade, arredondada pelo domain)
		// tem que bater com a conta feita em Go.
		if err := tx.QueryRow(ctx, "SELECT sum(price * qty)::money_amount, sum(qty)::quantity_amount FROM t").Scan(&total, &sumQty); err != nil {
			return err
		}
		item, _ := price.Times(qty) // 11,70155 -> 11,70
		// 3 × 11,70155 = 35,10465 -> 35,10 no banco; em Go, 3 × R$ 11,70 = 35,10.
		// (Atenção: somar e depois arredondar nem sempre dá o mesmo que
		// arredondar cada item e somar. A NF-e arredonda POR ITEM.)
		if total != money.FromCents(3510) || item.MulInt(3) != money.FromCents(3510) {
			t.Errorf("sum in db = %s, item×3 in Go = %s", total, item.MulInt(3))
		}
		if sumQty != quantity.MustParse("7.035") {
			t.Errorf("sum qty = %s", sumQty)
		}

		// Valor com casas demais vindo do banco é erro, não arredondamento escondido.
		var bad money.Money
		if err := tx.QueryRow(ctx, "SELECT 1.005::numeric").Scan(&bad); err == nil {
			t.Error("1.005 scanned into Money without error")
		}
		return pgx.ErrTxClosed // qualquer erro = ROLLBACK
	})
	if err != nil && err != pgx.ErrTxClosed {
		t.Fatal(err)
	}
}
