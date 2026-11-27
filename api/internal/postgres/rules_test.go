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

// TestSQLIsNeverBuiltAtRuntime protege contra SQL injection: o texto de toda
// consulta passada a Exec/Query/QueryRow tem que ser fixo no código (texto
// entre aspas ou constante). Valores digitados pelo usuário vão sempre como
// parâmetro ($1, @nome), nunca colados no texto.
//
// Reprova, por exemplo:
//
//	tx.Exec(ctx, "DELETE FROM x WHERE name = '"+name+"'")
//	tx.Query(ctx, fmt.Sprintf("SELECT * FROM %s", table))
//
// As consultas geradas pelo sqlc (internal/db) também passam por aqui: são
// constantes, então passam.
func TestSQLIsNeverBuiltAtRuntime(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()

	// 1ª passada: lê os arquivos e anota as constantes de cada pacote.
	type parsed struct {
		rel  string
		dir  string
		file *ast.File
	}
	var files []parsed
	consts := map[string]map[string]bool{} // pasta -> nomes das constantes
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		if consts[dir] == nil {
			consts[dir] = map[string]bool{}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if gd, ok := n.(*ast.GenDecl); ok && gd.Tok == token.CONST {
				for _, spec := range gd.Specs {
					for _, name := range spec.(*ast.ValueSpec).Names {
						consts[dir][name.Name] = true
					}
				}
			}
			return true
		})
		files = append(files, parsed{rel, dir, file})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// fixed diz se a expressão é texto fixo: "...", uma constante do pacote,
	// ou a soma (+) de coisas fixas.
	var fixed func(dir string, e ast.Expr) bool
	fixed = func(dir string, e ast.Expr) bool {
		switch v := e.(type) {
		case *ast.BasicLit:
			return v.Kind == token.STRING
		case *ast.Ident:
			return consts[dir][v.Name]
		case *ast.ParenExpr:
			return fixed(dir, v.X)
		case *ast.BinaryExpr:
			return v.Op == token.ADD && fixed(dir, v.X) && fixed(dir, v.Y)
		}
		return false
	}

	// 2ª passada: confere cada chamada.
	checked := 0
	for _, f := range files {
		ast.Inspect(f.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "Exec", "Query", "QueryRow":
			default:
				return true
			}
			checked++
			if !fixed(f.dir, call.Args[1]) {
				pos := fset.Position(call.Args[1].Pos())
				t.Errorf("%s:%d: SQL built at runtime in .%s(...); write a fixed query (sqlc) and pass values as parameters", f.rel, pos.Line, sel.Sel.Name)
			}
			return true
		})
	}
	// Proteção contra o teste passar "em falso" sem conferir nada (o sqlc gera dezenas).
	if checked < 50 {
		t.Fatalf("only %d query calls checked; is the root path right?", checked)
	}
}
