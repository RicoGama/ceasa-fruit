// Package document normalizes and validates Brazilian tax ids: CPF (people)
// and CNPJ (companies), including the alphanumeric CNPJ that the Federal
// Revenue started issuing in July 2026.
//
// It is a "pure" package: no HTTP, no database, so it is trivial to test.
package document

import "strings"

// Normalize strips the mask (dots, dash, slash, spaces) and uppercases.
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

// IsValid reports whether a normalized document is a valid CPF or CNPJ.
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

// IsValidCPF checks the two check digits of an 11-digit CPF.
func IsValidCPF(cpf string) bool {
	if len(cpf) != 11 || !allDigits(cpf) || allSame(cpf) {
		return false
	}
	// CPF uses descending weights: 10..2 for the 1st digit, 11..2 for the 2nd.
	d1 := checkDigit(cpf[:9], []int{10, 9, 8, 7, 6, 5, 4, 3, 2})
	d2 := checkDigit(cpf[:10], []int{11, 10, 9, 8, 7, 6, 5, 4, 3, 2})
	return int(cpf[9]-'0') == d1 && int(cpf[10]-'0') == d2
}

// IsValidCNPJ checks a numeric or alphanumeric CNPJ.
// Rule: the first 12 positions may be 0-9 or A-Z; the last 2 are always digits.
// Each character is worth (ASCII code - 48), so '0'..'9' are 0..9 and 'A'..'Z'
// are 17..42. That keeps the old numeric calculation unchanged.
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

// checkDigit applies the "modulo 11" algorithm shared by CPF and CNPJ.
func checkDigit(base string, weights []int) int {
	sum := 0
	for i := range len(base) { // ranging over an int: 0, 1, ..., len-1 (Go 1.22+)
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

// allSame rejects inputs like "00000000000" that would pass the math.
func allSame(s string) bool {
	return strings.Count(s, s[:1]) == len(s)
}
