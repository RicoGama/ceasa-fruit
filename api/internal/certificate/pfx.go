// Package certificate cuida do certificado digital A1 da empresa: recebe o
// arquivo .pfx/.p12 com a senha, confere, guarda criptografado no banco e
// entrega pronto para assinar (notas fiscais, SEFAZ).
//
// Por que A1 (arquivo) e não A3 (cartão/token): o servidor do box assina sem
// ninguém por perto e sem um token espetado na máquina.
package certificate

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"software.sslmate.com/src/go-pkcs12"

	"github.com/RicoGama/ceasa-fruit/api/internal/document"
)

// MaxFileSize: um .pfx tem poucos KB; isso sobra.
const MaxFileSize = 64 << 10

// Erros de conferência do arquivo. A tela traduz cada um.
var (
	ErrInvalidFile   = errors.New("not a valid A1 certificate file (.pfx or .p12)")
	ErrWrongPassword = errors.New("wrong certificate password")
	ErrNoPrivateKey  = errors.New("the file has no private key: export the certificate with the private key")
	ErrKeyMismatch   = errors.New("the private key does not belong to the certificate")
	ErrExpired       = errors.New("certificate has expired")
	ErrNotYetValid   = errors.New("certificate is not valid yet")
	ErrNoDocument    = errors.New("could not find a CNPJ in the certificate: it must be an e-CNPJ")
)

// Info é o que se mostra do certificado (nada secreto).
type Info struct {
	HolderName   string
	Document     string // CNPJ, 14 dígitos
	Issuer       string
	SerialNumber string
	Fingerprint  string // SHA-256 do certificado, em hexadecimal
	NotBefore    time.Time
	NotAfter     time.Time
}

// Parsed é o certificado aberto, pronto para assinar.
type Parsed struct {
	Info
	Certificate *x509.Certificate
	Chain       []*x509.Certificate // intermediárias que vieram no arquivo
	Key         crypto.Signer
}

// OIDs da ICP-Brasil no "nome alternativo" do certificado (DOC-ICP-04).
var (
	oidSubjectAltName = asn1.ObjectIdentifier{2, 5, 29, 17}
	oidICPCNPJ        = asn1.ObjectIdentifier{2, 16, 76, 1, 3, 3} // CNPJ da pessoa jurídica
)

// Parse abre o arquivo com a senha e confere: tem chave privada, a chave é do
// certificado, está dentro da validade e é de um CNPJ.
func Parse(pfx []byte, password string, now time.Time) (*Parsed, error) {
	if len(pfx) == 0 || len(pfx) > MaxFileSize {
		return nil, ErrInvalidFile
	}
	key, cert, chain, err := pkcs12.DecodeChain(pfx, password)
	switch {
	case errors.Is(err, pkcs12.ErrIncorrectPassword), errors.Is(err, pkcs12.ErrDecryption):
		return nil, ErrWrongPassword
	case err != nil:
		// Arquivo sem chave privada também cai aqui na biblioteca.
		if strings.Contains(err.Error(), "private key") {
			return nil, ErrNoPrivateKey
		}
		return nil, ErrInvalidFile
	case key == nil:
		return nil, ErrNoPrivateKey
	}
	signer, ok := key.(crypto.Signer)
	if !ok || !samePublicKey(signer.Public(), cert.PublicKey) {
		return nil, ErrKeyMismatch
	}
	if now.After(cert.NotAfter) {
		return nil, ErrExpired
	}
	if now.Before(cert.NotBefore) {
		return nil, ErrNotYetValid
	}
	doc := cnpjOf(cert)
	if doc == "" {
		return nil, ErrNoDocument
	}
	sum := sha256.Sum256(cert.Raw)
	return &Parsed{
		Info: Info{
			HolderName:   holderName(cert),
			Document:     doc,
			Issuer:       cert.Issuer.CommonName,
			SerialNumber: strings.ToUpper(cert.SerialNumber.Text(16)),
			Fingerprint:  hex.EncodeToString(sum[:]),
			NotBefore:    cert.NotBefore,
			NotAfter:     cert.NotAfter,
		},
		Certificate: cert,
		Chain:       chain,
		Key:         signer,
	}, nil
}

func samePublicKey(a, b crypto.PublicKey) bool {
	switch k := a.(type) {
	case *rsa.PublicKey:
		return k.Equal(b)
	case *ecdsa.PublicKey:
		return k.Equal(b)
	}
	return false
}

// holderName: no e-CNPJ o nome comum é "RAZÃO SOCIAL:CNPJ"; fica só a razão.
func holderName(c *x509.Certificate) string {
	name, _, _ := strings.Cut(c.Subject.CommonName, ":")
	return strings.TrimSpace(name)
}

var fourteenDigits = regexp.MustCompile(`\d{14}`)

// cnpjOf procura o CNPJ: primeiro no campo próprio da ICP-Brasil; se não
// houver, no fim do nome comum ("RAZÃO SOCIAL:12345678000199").
func cnpjOf(c *x509.Certificate) string {
	if d := otherName(c, oidICPCNPJ); len(d) == 14 && document.IsValidCNPJ(d) {
		return d
	}
	if _, after, ok := strings.Cut(c.Subject.CommonName, ":"); ok {
		if d := document.Normalize(after); len(d) == 14 && document.IsValidCNPJ(d) {
			return d
		}
	}
	if d := fourteenDigits.FindString(c.Subject.CommonName); d != "" && document.IsValidCNPJ(d) {
		return d
	}
	return ""
}

// otherName lê um campo "otherName" do nome alternativo e devolve só os
// dígitos. Formato (RFC 5280):
//
//	GeneralName ::= [0] otherName { type-id OID, value [0] EXPLICIT ANY }
func otherName(c *x509.Certificate, want asn1.ObjectIdentifier) string {
	for _, ext := range c.Extensions {
		if !ext.Id.Equal(oidSubjectAltName) {
			continue
		}
		var names []asn1.RawValue
		if _, err := asn1.Unmarshal(ext.Value, &names); err != nil {
			return ""
		}
		for _, n := range names {
			if n.Class != asn1.ClassContextSpecific || n.Tag != 0 {
				continue
			}
			var oid asn1.ObjectIdentifier
			rest, err := asn1.Unmarshal(n.Bytes, &oid)
			if err != nil || !oid.Equal(want) {
				continue
			}
			var explicit, inner asn1.RawValue
			if _, err := asn1.Unmarshal(rest, &explicit); err != nil {
				continue
			}
			if _, err := asn1.Unmarshal(explicit.Bytes, &inner); err != nil {
				continue
			}
			return document.Normalize(string(inner.Bytes))
		}
	}
	return ""
}
