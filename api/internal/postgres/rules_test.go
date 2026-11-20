package postgres_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestEverythingGoesThroughTransactions garante a regra do projeto: todo
// acesso ao banco passa por postgres.DB (InTx / InReadTx).
//
// Ele lê o código-fonte (não executa nada) e reprova quem:
//   - abre transação por conta própria (pgx.BeginFunc, .Begin(...));
//   - cria queries soltas, fora de transação (db.New(...));
//   - recebe o pool de conexões (pgxpool) num pacote de regra de negócio.
//
// Arquivos de teste ficam de fora: eles podem montar cenários à vontade.
func TestEverythingGoesThroughTransactions(t *testing.T) {
	root := filepath.Join("..", "..") // a pasta api/, onde fica o go.mod

	// Quem pode lidar com o pool: a conexão, a montagem do servidor, a fila
	// (o River precisa do pool) e os executáveis em cmd/.
	poolAllowed := func(dir string) bool {
		return dir == "internal/postgres" || dir == "internal/server" ||
			dir == "internal/queue" || strings.HasPrefix(dir, "cmd/")
	}

	fset := token.NewFileSet()
	scanned := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			// internal/db é código gerado pelo sqlc; o helper vive em internal/postgres.
			// (rel != "." porque a própria raiz se chama ".." e seria pulada.)
			hidden := rel != "." && strings.HasPrefix(d.Name(), ".")
			if rel == "internal/db" || rel == "internal/postgres" || hidden {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}

		scanned++
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(rel))

		for _, imp := range file.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if p == "github.com/jackc/pgx/v5/pgxpool" && !poolAllowed(dir) {
				t.Errorf("%s: imports pgxpool; receive a postgres.DB instead", rel)
			}
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pos := fset.Position(call.Pos())
			name := sel.Sel.Name
			if pkg, ok := sel.X.(*ast.Ident); ok {
				switch {
				case pkg.Name == "pgx" && (name == "BeginFunc" || name == "BeginTxFunc"):
					t.Errorf("%s:%d: pgx.%s outside internal/postgres; use DB.InTx / DB.InReadTx", rel, pos.Line, name)
				case pkg.Name == "db" && name == "New":
					t.Errorf("%s:%d: db.New outside a transaction; use DB.InTx / DB.InReadTx", rel, pos.Line)
				}
			}
			if name == "Begin" || name == "BeginTx" {
				t.Errorf("%s:%d: .%s(...) opens a transaction by hand; use DB.InTx / DB.InReadTx", rel, pos.Line, name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Proteção contra o teste passar "em falso" sem ler nada.
	if scanned < 20 {
		t.Fatalf("only %d files scanned; is the root path right?", scanned)
	}
}
