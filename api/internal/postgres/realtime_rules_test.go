package postgres_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// REGRA DO PROJETO: tempo real em tudo. Toda tabela tem um gatilho que avisa
// as telas (migration 00007) na criação, na alteração e na exclusão. A
// inativação (exclusão lógica) é um UPDATE, então já está coberta.
//
// Este teste lê o banco depois das migrations e reprova tabela nova sem
// gatilho. Se a tabela realmente não precisa (não aparece em tela nenhuma),
// ponha-a em notRealtime com o motivo.
var notRealtime = map[string]string{
	"cities":                "lista fixa do IBGE (migration 00001)",
	"states":                "lista fixa do IBGE (migration 00001)",
	"sessions":              "sessões de login; quem vê os acessos é o access_log",
	"password_reset_tokens": "links de troca de senha, nunca aparecem em tela",
	"goose_db_version":      "controle das migrations",
}

// Tabelas que avisam só em parte das operações, de propósito.
var partialRealtime = map[string]struct{ ops, why string }{
	"company":    {"IU", "registro único: nunca é apagado"},
	"access_log": {"IU", "histórico: nunca é apagado"},
	"fleet_sync": {"IU", "registro único da situação da sincronização"},
	"audit_log":  {"I", "o histórico só recebe linhas novas"},
	// A pessoa nasce junto com o papel (o INSERT de suppliers/customers avisa)
	// e, se apagada, a cascata apaga o papel, que avisa o DELETE.
	"parties": {"U", "avisa pelo papel (fornecedor/cliente)"},
}

func TestEveryTableNotifiesChanges(t *testing.T) {
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

	// Para cada tabela: quais operações (I, U, D) têm um gatilho que chama
	// uma função notify_* (notify_change, notify_party_change...).
	rows, err := pool.Query(ctx, `
		SELECT c.relname,
		       coalesce(bool_or(tg.tgtype & 4 > 0), false)  AS on_insert,
		       coalesce(bool_or(tg.tgtype & 16 > 0), false) AS on_update,
		       coalesce(bool_or(tg.tgtype & 8 > 0), false)  AS on_delete
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		LEFT JOIN pg_trigger tg ON tg.tgrelid = c.oid AND NOT tg.tgisinternal
		     AND tg.tgfoid IN (SELECT oid FROM pg_proc WHERE proname LIKE 'notify\_%')
		WHERE c.relkind = 'r'
		  AND c.relname NOT LIKE 'river\_%'   -- tabelas da fila (biblioteca River)
		  AND c.relname NOT LIKE 'tx\_test%'  -- tabela temporária de outro teste
		GROUP BY c.relname
		ORDER BY c.relname`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var seen []string
	for rows.Next() {
		var name string
		var ins, upd, del bool
		if err := rows.Scan(&name, &ins, &upd, &del); err != nil {
			t.Fatal(err)
		}
		seen = append(seen, name)
		var ops strings.Builder
		for _, o := range []struct {
			on bool
			c  string
		}{{ins, "I"}, {upd, "U"}, {del, "D"}} {
			if o.on {
				ops.WriteString(o.c)
			}
		}
		want := "IUD"
		if p, ok := partialRealtime[name]; ok {
			want = p.ops
		}
		if _, skip := notRealtime[name]; skip {
			want = ""
		}
		if ops.String() != want {
			t.Errorf("table %s notifies %q, want %q: add a trigger like "+
				"`CREATE TRIGGER %s_notify AFTER INSERT OR UPDATE OR DELETE ON %s FOR EACH ROW EXECUTE FUNCTION notify_change('<topic>', 'id')` "+
				"(or list it in notRealtime/partialRealtime with the reason)", name, ops.String(), want, name, name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	// As listas não podem citar tabelas que não existem mais.
	for name := range notRealtime {
		if !slices.Contains(seen, name) {
			t.Errorf("notRealtime lists %s, which does not exist", name)
		}
	}
	for name := range partialRealtime {
		if !slices.Contains(seen, name) {
			t.Errorf("partialRealtime lists %s, which does not exist", name)
		}
	}
}

// REGRA DO PROJETO: toda tabela de cadastro tem histórico de alterações
// (gatilho audit_row, migration 00013). Tabela nova sem histórico reprova,
// a não ser que esteja aqui com o motivo.
var notAudited = map[string]string{
	"cities":                "lista fixa do IBGE",
	"states":                "lista fixa do IBGE",
	"sessions":              "sessões de login; o registro de acessos cobre",
	"password_reset_tokens": "links de troca de senha",
	"goose_db_version":      "controle das migrations",
	"access_log":            "já é um registro (de acessos)",
	"vehicle_positions":     "posições do GPS, não são alterações de cadastro",
	"fleet_sync":            "situação da sincronização, muda a cada rodada",
	"audit_log":             "é o próprio histórico",
}

func TestEveryTableIsAudited(t *testing.T) {
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
	rows, err := pool.Query(ctx, `
		SELECT c.relname,
		       coalesce(bool_or(tg.tgtype & 4 > 0 AND tg.tgtype & 16 > 0 AND tg.tgtype & 8 > 0), false)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		LEFT JOIN pg_trigger tg ON tg.tgrelid = c.oid AND NOT tg.tgisinternal
		     AND tg.tgfoid = 'audit_row'::regproc
		WHERE c.relkind = 'r' AND c.relname NOT LIKE 'river\_%' AND c.relname NOT LIKE 'tx\_test%'
		GROUP BY c.relname ORDER BY c.relname`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var name string
		var audited bool
		if err := rows.Scan(&name, &audited); err != nil {
			t.Fatal(err)
		}
		seen[name] = true
		_, skip := notAudited[name]
		switch {
		case !skip && !audited:
			t.Errorf("table %s has no change history: add `CREATE TRIGGER %s_audit AFTER INSERT OR UPDATE OR DELETE ON %s "+
				"FOR EACH ROW EXECUTE FUNCTION audit_row('<entity>', '<entity id column>', '<ignored columns>')` "+
				"(or list it in notAudited with the reason)", name, name, name)
		case skip && audited:
			t.Errorf("table %s is audited but listed in notAudited", name)
		}
	}
	for name := range notAudited {
		if !seen[name] {
			t.Errorf("notAudited lists %s, which does not exist", name)
		}
	}
}
