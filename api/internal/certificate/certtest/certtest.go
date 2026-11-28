// Package certtest gera certificados A1 de mentira, no formato da ICP-Brasil
// (CNPJ no nome alternativo, nome comum "RAZÃO SOCIAL:CNPJ"), para testes.
// Eles não valem para nada fora dos testes: a autoridade certificadora é
// inventada na hora.
package certtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

// Options do certificado gerado. Zeros viram valores padrão.
type Options struct {
	HolderName string    // padrão: "IRMAOS FAISAO FRUTICOLA LTDA"
	CNPJ       string    // 14 dígitos; padrão: 11222333000181
	NotBefore  time.Time // padrão: ontem
	NotAfter   time.Time // padrão: daqui a 1 ano
	Password   string    // padrão: "senha-teste"
	Legacy     bool      // criptografia antiga (3DES), comum em .pfx exportados no Windows
	NoCNPJ     bool      // certificado sem CNPJ (ex.: um e-CPF)
	WithoutKey bool      // .pfx sem a chave privada
	OtherKey   bool      // chave privada que não é a do certificado
}

// PFX devolve o arquivo .pfx pronto e a senha.
func PFX(o Options) ([]byte, string) {
	if o.HolderName == "" {
		o.HolderName = "IRMAOS FAISAO FRUTICOLA LTDA"
	}
	if o.CNPJ == "" {
		o.CNPJ = "11222333000181"
	}
	if o.NotBefore.IsZero() {
		o.NotBefore = time.Now().Add(-24 * time.Hour)
	}
	if o.NotAfter.IsZero() {
		o.NotAfter = time.Now().AddDate(1, 0, 0)
	}
	if o.Password == "" {
		o.Password = "senha-teste"
	}

	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "AC TESTE CEASA FRUIT", Organization: []string{"ICP-Brasil (teste)"}},
		NotBefore:    o.NotBefore.Add(-time.Hour), NotAfter: o.NotAfter.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	ca, _ := x509.ParseCertificate(caDER)

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	cn := o.HolderName
	var extra []pkix.Extension
	if !o.NoCNPJ {
		cn += ":" + o.CNPJ
		extra = append(extra, pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 17}, Value: sanWithCNPJ(o.CNPJ)})
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 64))
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"ICP-Brasil"}},
		NotBefore:    o.NotBefore, NotAfter: o.NotAfter,
		KeyUsage:        x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		ExtraExtensions: extra,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, ca, &key.PublicKey, caKey)
	cert, _ := x509.ParseCertificate(der)

	enc := pkcs12.Modern
	if o.Legacy {
		enc = pkcs12.LegacyDES
	}
	if o.WithoutKey {
		pfx, _ := enc.EncodeTrustStore([]*x509.Certificate{cert}, o.Password)
		return pfx, o.Password
	}
	var priv any = key
	if o.OtherKey {
		priv, _ = rsa.GenerateKey(rand.Reader, 2048)
	}
	pfx, err := enc.Encode(priv, cert, []*x509.Certificate{ca}, o.Password)
	if err != nil {
		panic(err)
	}
	return pfx, o.Password
}

// sanWithCNPJ monta o nome alternativo com o campo de CNPJ da ICP-Brasil
// (OID 2.16.76.1.3.3), como as autoridades certificadoras fazem.
func sanWithCNPJ(cnpj string) []byte {
	type otherName struct {
		TypeID asn1.ObjectIdentifier
		Value  asn1.RawValue `asn1:"tag:0,explicit"`
	}
	value, _ := asn1.Marshal(asn1.RawValue{Tag: asn1.TagOctetString, Bytes: []byte(cnpj)})
	on, _ := asn1.MarshalWithParams(otherName{
		TypeID: asn1.ObjectIdentifier{2, 16, 76, 1, 3, 3},
		Value:  asn1.RawValue{FullBytes: value},
	}, "tag:0")
	san, _ := asn1.Marshal([]asn1.RawValue{{FullBytes: on}})
	return san
}
