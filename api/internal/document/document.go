// Package document normaliza e valida CPF (pessoa física) e CNPJ (pessoa
// jurídica), inclusive o CNPJ alfanumérico que a Receita Federal passou a
// emitir em julho de 2026.
//
// É um pacote "puro": não conhece HTTP nem banco, por isso é trivial de testar.
package document

import "strings"

// Normalize remove a máscara (pontos, traço, barra, espaços) e deixa em maiúsculas.
//
//	"529.982.247-25"     -> "52998224725"
//	"12.abc.345/01de-35" -> "12ABC34501DE35"
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if isDigit(r) || isLetter(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// IsValid diz se um documento já normalizado é um CPF ou CNPJ válido.
func IsValid(doc string) bool {
	switch len(doc) {
	case 11:
		return IsValidCPF(doc)
	case 14:
		return IsValidCNPJ(doc)
	default:
		return false
	}
}

// IsValidCPF confere os dois dígitos verificadores de um CPF de 11 dígitos.
func IsValidCPF(cpf string) bool {
	if len(cpf) != 11 || !allDigits(cpf) || allSame(cpf) {
		return false
	}
	// O CPF usa pesos decrescentes: 10..2 para o 1º dígito e 11..2 para o 2º.
	d1 := checkDigit(cpf[:9], []int{10, 9, 8, 7, 6, 5, 4, 3, 2})
	d2 := checkDigit(cpf[:10], []int{11, 10, 9, 8, 7, 6, 5, 4, 3, 2})
	return int(cpf[9]-'0') == d1 && int(cpf[10]-'0') == d2
}

// IsValidCNPJ confere um CNPJ numérico ou alfanumérico.
// Regra: as 12 primeiras posições podem ser 0-9 ou A-Z; as 2 últimas são sempre
// dígitos. Cada caractere vale (código ASCII - 48), então '0'..'9' valem 0..9 e
// 'A'..'Z' valem 17..42. Assim o cálculo antigo, só numérico, continua igual.
func IsValidCNPJ(cnpj string) bool {
	if len(cnpj) != 14 || allSame(cnpj) {
		return false
	}
	for i, r := range cnpj {
		if i < 12 && !isDigit(r) && !isLetter(r) {
			return false
		}
		if i >= 12 && !isDigit(r) {
			return false
		}
	}
	d1 := checkDigit(cnpj[:12], []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	d2 := checkDigit(cnpj[:13], []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	return int(cnpj[12]-'0') == d1 && int(cnpj[13]-'0') == d2
}

// checkDigit aplica o "módulo 11", usado tanto pelo CPF quanto pelo CNPJ.
func checkDigit(base string, weights []int) int {
	sum := 0
	for i := range len(base) { // range sobre um inteiro: 0, 1, ..., len-1 (Go 1.22+)
		sum += int(base[i]-'0') * weights[i]
	}
	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}

func isDigit(r rune) bool  { return r >= '0' && r <= '9' }
func isLetter(r rune) bool { return r >= 'A' && r <= 'Z' }

func allDigits(s string) bool {
	for _, r := range s {
		if !isDigit(r) {
			return false
		}
	}
	return true
}

// allSame barra entradas como "00000000000", que passariam no cálculo.
func allSame(s string) bool {
	return strings.Count(s, s[:1]) == len(s)
}
