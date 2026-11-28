// Comando dev-certificate gera um certificado A1 de TESTE (.pfx), no formato
// da ICP-Brasil, para experimentar a tela de certificado sem um certificado
// de verdade. Não vale para nada fora do desenvolvimento e não entra no pacote
// instalado no servidor (deploy/build-release.sh).
//
//	go run ./cmd/dev-certificate -cnpj 11222333000181 -days 365 -o teste.pfx
//
// A senha aparece na tela.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/RicoGama/ceasa-fruit/api/internal/certificate/certtest"
)

func main() {
	cnpj := flag.String("cnpj", "11222333000181", "CNPJ do titular (14 dígitos)")
	name := flag.String("name", "IRMAOS FAISAO FRUTICOLA LTDA", "razão social do titular")
	days := flag.Int("days", 365, "dias até vencer (negativo = já vencido)")
	password := flag.String("password", "senha-teste", "senha do arquivo")
	out := flag.String("o", "certificado-teste.pfx", "arquivo de saída")
	flag.Parse()

	opts := certtest.Options{HolderName: *name, CNPJ: *cnpj, Password: *password,
		NotAfter: time.Now().Add(time.Duration(*days)*24*time.Hour + time.Hour)}
	if *days < 0 {
		opts.NotBefore = opts.NotAfter.AddDate(-1, 0, 0)
	}
	pfx, pass := certtest.PFX(opts)
	if err := os.WriteFile(*out, pfx, 0o600); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s gerado (senha: %s)\n", *out, pass)
}
