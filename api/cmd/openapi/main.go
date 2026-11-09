// Command openapi imprime a especificação OpenAPI da API (sem banco).
//
// Uso: go run ./cmd/openapi > openapi.json   (ou: make openapi)
//
// O front (web/) gera os tipos TypeScript a partir desse arquivo, então
// qualquer mudança na API aparece como erro de tipo no front.
package main

import (
	"fmt"
	"os"

	"github.com/RicoGama/ceasa-fruit/api/internal/server"
)

func main() {
	spec, err := server.OpenAPISpec()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(string(spec))
}
