// Package party tem o que é comum a qualquer cadastro de pessoa (física ou
// jurídica): identificação, endereço, contatos e dados de pagamento.
//
// Fornecedor (e, no futuro, cliente) é um "papel" de uma pessoa: o pacote
// supplier embute os tipos daqui e acrescenta só o que é de fornecedor.
package party

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/document"
)

// ---------------------------------------------------------------------------
// Entrada (corpo das requisições)
// ---------------------------------------------------------------------------

// Fields são os campos editáveis de uma pessoa.
// Ponteiro (*string) = campo opcional: nil vira NULL no banco.
type Fields struct {
	PersonType        string  `json:"personType" enum:"individual,company" doc:"individual = CPF, company = CNPJ"`
	Document          *string `json:"document,omitempty" maxLength:"18" doc:"CPF or CNPJ, with or without mask"`
	LegalName         string  `json:"legalName" minLength:"2" maxLength:"150" doc:"Company name (razão social) or full name"`
	TradeName         *string `json:"tradeName,omitempty" maxLength:"150" doc:"Trade name or nickname"`
	StateRegistration *string `json:"stateRegistration,omitempty" maxLength:"20"`
	TaxpayerType      string  `json:"taxpayerType,omitempty" enum:"taxpayer,exempt,non_taxpayer" default:"non_taxpayer"`
	Email             *string `json:"email,omitempty" format:"email" maxLength:"150"`
	Phone             *string `json:"phone,omitempty" maxLength:"20"`
	Whatsapp          *string `json:"whatsapp,omitempty" maxLength:"20"`

	Address  AddressInput   `json:"address,omitempty"`
	Payment  PaymentInput   `json:"payment,omitempty"`
	Contacts []ContactInput `json:"contacts,omitempty" maxItems:"20"`
	Notes    *string        `json:"notes,omitempty" maxLength:"2000"`
}

type AddressInput struct {
	PostalCode *string `json:"postalCode,omitempty" maxLength:"9" doc:"CEP, with or without dash"`
	Street     *string `json:"street,omitempty" maxLength:"150"`
	Number     *string `json:"number,omitempty" maxLength:"20"`
	Complement *string `json:"complement,omitempty" maxLength:"100"`
	District   *string `json:"district,omitempty" maxLength:"100"`
	CityID     *int32  `json:"cityId,omitempty" minimum:"1000000" maximum:"9999999" doc:"IBGE city code"`
}

type PaymentInput struct {
	PixKeyType                *string `json:"pixKeyType,omitempty" enum:"cpf,cnpj,email,phone,random"`
	PixKey                    *string `json:"pixKey,omitempty" maxLength:"100"`
	Bank                      *string `json:"bank,omitempty" maxLength:"100"`
	BankBranch                *string `json:"bankBranch,omitempty" maxLength:"20"`
	BankAccount               *string `json:"bankAccount,omitempty" maxLength:"30"`
	BankAccountType           *string `json:"bankAccountType,omitempty" enum:"checking,savings,payment"`
	BankAccountHolder         *string `json:"bankAccountHolder,omitempty" maxLength:"150"`
	BankAccountHolderDocument *string `json:"bankAccountHolderDocument,omitempty" maxLength:"18"`
}

type ContactInput struct {
	Name       string  `json:"name" minLength:"1" maxLength:"100"`
	Role       *string `json:"role,omitempty" maxLength:"60" doc:"e.g. owner, manager, driver"`
	Phone      *string `json:"phone,omitempty" maxLength:"20"`
	IsWhatsapp bool    `json:"isWhatsapp,omitempty"`
	Email      *string `json:"email,omitempty" format:"email" maxLength:"150"`
}

// ---------------------------------------------------------------------------
// Saída (respostas)
// ---------------------------------------------------------------------------

type City struct {
	ID    int32  `json:"id" example:"3304557"`
	Name  string `json:"name" example:"Rio de Janeiro"`
	State string `json:"state" example:"RJ"`
}

// Party é a pessoa como a API devolve.
type Party struct {
	PersonType        string    `json:"personType" enum:"individual,company"`
	Document          *string   `json:"document"`
	LegalName         string    `json:"legalName"`
	TradeName         *string   `json:"tradeName"`
	StateRegistration *string   `json:"stateRegistration"`
	TaxpayerType      string    `json:"taxpayerType" enum:"taxpayer,exempt,non_taxpayer"`
	Email             *string   `json:"email"`
	Phone             *string   `json:"phone"`
	Whatsapp          *string   `json:"whatsapp"`
	Address           Address   `json:"address"`
	Payment           Payment   `json:"payment"`
	Contacts          []Contact `json:"contacts"`
	Notes             *string   `json:"notes"`
}

type Address struct {
	PostalCode *string `json:"postalCode"`
	Street     *string `json:"street"`
	Number     *string `json:"number"`
	Complement *string `json:"complement"`
	District   *string `json:"district"`
	City       *City   `json:"city"`
}

type Payment struct {
	PixKeyType                *string `json:"pixKeyType" enum:"cpf,cnpj,email,phone,random"`
	PixKey                    *string `json:"pixKey"`
	Bank                      *string `json:"bank"`
	BankBranch                *string `json:"bankBranch"`
	BankAccount               *string `json:"bankAccount"`
	BankAccountType           *string `json:"bankAccountType" enum:"checking,savings,payment"`
	BankAccountHolder         *string `json:"bankAccountHolder"`
	BankAccountHolderDocument *string `json:"bankAccountHolderDocument"`
}

type Contact struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Role       *string `json:"role"`
	Phone      *string `json:"phone"`
	IsWhatsapp bool    `json:"isWhatsapp"`
	Email      *string `json:"email"`
}

// ---------------------------------------------------------------------------
// Normalização e validação
// ---------------------------------------------------------------------------

var (
	nonDigits = regexp.MustCompile(`\D`)
	uuidRe    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// Normalize limpa os campos (espaços, máscaras, "" -> nil) e devolve os erros
// que as struct tags não conseguem expressar. Os erros apontam o campo com o
// mesmo caminho do JSON ("body.address.postalCode"), para o front destacar.
func (f *Fields) Normalize() []error {
	var errs []error
	fail := func(location, message string, value any) {
		errs = append(errs, &huma.ErrorDetail{Location: "body." + location, Message: message, Value: value})
	}

	f.LegalName = strings.TrimSpace(f.LegalName)
	f.TradeName = clean(f.TradeName)
	f.StateRegistration = upper(clean(f.StateRegistration))
	f.Email = lower(clean(f.Email))
	f.Notes = clean(f.Notes)
	if f.TaxpayerType == "" {
		f.TaxpayerType = "non_taxpayer"
	}

	if len(f.LegalName) < 2 {
		fail("legalName", "expected length >= 2", f.LegalName)
	}

	// Documento: opcional, mas se vier tem que ser válido e combinar com o tipo.
	if d := clean(f.Document); d != nil {
		n := document.Normalize(*d)
		f.Document = &n
		switch {
		case !document.IsValid(n):
			fail("document", "invalid CPF or CNPJ", n)
		case f.PersonType == "individual" && len(n) != 11:
			fail("document", "an individual must have a CPF", n)
		case f.PersonType == "company" && len(n) != 14:
			fail("document", "a company must have a CNPJ", n)
		}
	} else {
		f.Document = nil
	}

	var bad string
	if f.Phone, bad = phone(f.Phone); bad != "" {
		fail("phone", "invalid phone", bad)
	}
	if f.Whatsapp, bad = phone(f.Whatsapp); bad != "" {
		fail("whatsapp", "invalid phone", bad)
	}

	// Endereço
	a := &f.Address
	if pc := clean(a.PostalCode); pc != nil {
		digits := nonDigits.ReplaceAllString(*pc, "")
		if len(digits) != 8 {
			fail("address.postalCode", "invalid postal code", *pc)
		}
		a.PostalCode = &digits
	} else {
		a.PostalCode = nil
	}
	a.Street = clean(a.Street)
	a.Number = clean(a.Number)
	a.Complement = clean(a.Complement)
	a.District = clean(a.District)

	// Pagamento
	p := &f.Payment
	p.PixKey = clean(p.PixKey)
	switch {
	case p.PixKeyType == nil && p.PixKey != nil:
		fail("payment.pixKeyType", "pix key type is required", nil)
	case p.PixKeyType != nil && p.PixKey == nil:
		fail("payment.pixKey", "pix key is required", nil)
	case p.PixKeyType != nil:
		if msg := normalizePixKey(*p.PixKeyType, p.PixKey); msg != "" {
			fail("payment.pixKey", msg, *p.PixKey)
		}
	}
	p.Bank = clean(p.Bank)
	p.BankBranch = clean(p.BankBranch)
	p.BankAccount = clean(p.BankAccount)
	p.BankAccountHolder = clean(p.BankAccountHolder)
	if d := clean(p.BankAccountHolderDocument); d != nil {
		n := document.Normalize(*d)
		p.BankAccountHolderDocument = &n
		if !document.IsValid(n) {
			fail("payment.bankAccountHolderDocument", "invalid CPF or CNPJ", n)
		}
	} else {
		p.BankAccountHolderDocument = nil
	}

	// Contatos
	for i := range f.Contacts {
		c := &f.Contacts[i] // ponteiro para alterar o item da lista, e não uma cópia
		c.Name = strings.TrimSpace(c.Name)
		c.Role = clean(c.Role)
		c.Email = lower(clean(c.Email))
		if c.Name == "" {
			fail(contactField(i, "name"), "expected length >= 1", c.Name)
		}
		if c.Phone, bad = phone(c.Phone); bad != "" {
			fail(contactField(i, "phone"), "invalid phone", bad)
		}
	}
	return errs
}

func contactField(i int, field string) string {
	return "contacts[" + strconv.Itoa(i) + "]." + field
}

// normalizePixKey ajusta a chave conforme o tipo e devolve uma mensagem de erro (ou "").
func normalizePixKey(kind string, key *string) string {
	switch kind {
	case "cpf", "cnpj":
		n := document.Normalize(*key)
		*key = n
		if (kind == "cpf" && len(n) != 11) || (kind == "cnpj" && len(n) != 14) || !document.IsValid(n) {
			return "invalid pix key"
		}
	case "email":
		*key = strings.ToLower(*key)
		if !strings.Contains(*key, "@") {
			return "invalid pix key"
		}
	case "phone":
		digits := nonDigits.ReplaceAllString(*key, "")
		*key = digits
		if len(digits) < 10 || len(digits) > 13 {
			return "invalid pix key"
		}
	case "random":
		*key = strings.ToLower(*key)
		if !uuidRe.MatchString(*key) {
			return "invalid pix key"
		}
	}
	return ""
}

// phone guarda só os dígitos. Devolve o valor original em bad quando inválido.
func phone(s *string) (normalized *string, bad string) {
	s = clean(s)
	if s == nil {
		return nil, ""
	}
	digits := nonDigits.ReplaceAllString(*s, "")
	if len(digits) < 8 || len(digits) > 13 {
		return s, *s
	}
	return &digits, ""
}

func clean(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

func upper(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.ToUpper(*s)
	return &v
}

func lower(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.ToLower(*s)
	return &v
}

// ---------------------------------------------------------------------------
// Conversões banco <-> API
// ---------------------------------------------------------------------------

// CreateParams monta os parâmetros do INSERT a partir dos campos já normalizados.
func (f *Fields) CreateParams(organizationID int64) db.CreatePartyParams {
	a, p := f.Address, f.Payment
	return db.CreatePartyParams{
		OrganizationID: organizationID, PersonType: f.PersonType, Document: f.Document,
		LegalName: f.LegalName, TradeName: f.TradeName, StateRegistration: f.StateRegistration,
		TaxpayerType: f.TaxpayerType, Email: f.Email, Phone: f.Phone, Whatsapp: f.Whatsapp,
		PostalCode: a.PostalCode, Street: a.Street, Number: a.Number, Complement: a.Complement,
		District: a.District, CityID: a.CityID,
		PixKeyType: p.PixKeyType, PixKey: p.PixKey, Bank: p.Bank, BankBranch: p.BankBranch,
		BankAccount: p.BankAccount, BankAccountType: p.BankAccountType,
		BankAccountHolder: p.BankAccountHolder, BankAccountHolderDocument: p.BankAccountHolderDocument,
		Notes: f.Notes,
	}
}

// UpdateParams monta os parâmetros do UPDATE.
func (f *Fields) UpdateParams(organizationID, id int64) db.UpdatePartyParams {
	c := f.CreateParams(organizationID)
	// As duas structs têm os mesmos campos (fora o ID); a conversão direta só
	// funciona se forem idênticas, então copiamos campo a campo.
	return db.UpdatePartyParams{
		OrganizationID: organizationID, ID: id, PersonType: c.PersonType, Document: c.Document,
		LegalName: c.LegalName, TradeName: c.TradeName, StateRegistration: c.StateRegistration,
		TaxpayerType: c.TaxpayerType, Email: c.Email, Phone: c.Phone, Whatsapp: c.Whatsapp,
		PostalCode: c.PostalCode, Street: c.Street, Number: c.Number, Complement: c.Complement,
		District: c.District, CityID: c.CityID,
		PixKeyType: c.PixKeyType, PixKey: c.PixKey, Bank: c.Bank, BankBranch: c.BankBranch,
		BankAccount: c.BankAccount, BankAccountType: c.BankAccountType,
		BankAccountHolder: c.BankAccountHolder, BankAccountHolderDocument: c.BankAccountHolderDocument,
		Notes: c.Notes,
	}
}

// FromDB converte a linha do banco (com a cidade e os contatos) para a API.
func FromDB(p db.Party, cityName, cityState *string, contacts []db.PartyContact) Party {
	out := Party{
		PersonType: p.PersonType, Document: p.Document, LegalName: p.LegalName,
		TradeName: p.TradeName, StateRegistration: p.StateRegistration,
		TaxpayerType: p.TaxpayerType, Email: p.Email, Phone: p.Phone, Whatsapp: p.Whatsapp,
		Address: Address{
			PostalCode: p.PostalCode, Street: p.Street, Number: p.Number,
			Complement: p.Complement, District: p.District,
		},
		Payment: Payment{
			PixKeyType: p.PixKeyType, PixKey: p.PixKey, Bank: p.Bank, BankBranch: p.BankBranch,
			BankAccount: p.BankAccount, BankAccountType: p.BankAccountType,
			BankAccountHolder: p.BankAccountHolder, BankAccountHolderDocument: p.BankAccountHolderDocument,
		},
		Contacts: make([]Contact, 0, len(contacts)),
		Notes:    p.Notes,
	}
	if p.CityID != nil && cityName != nil && cityState != nil {
		out.Address.City = &City{ID: *p.CityID, Name: *cityName, State: *cityState}
	}
	for _, c := range contacts {
		out.Contacts = append(out.Contacts, Contact{
			ID: c.ID, Name: c.Name, Role: c.Role, Phone: c.Phone, IsWhatsapp: c.IsWhatsapp, Email: c.Email,
		})
	}
	return out
}

// ContactStore é o que SaveContacts precisa do banco.
type ContactStore interface {
	DeletePartyContacts(ctx context.Context, arg db.DeletePartyContactsParams) error
	CreatePartyContact(ctx context.Context, arg db.CreatePartyContactParams) error
}

// SaveContacts substitui todos os contatos da pessoa pelos recebidos.
// Apagar e inserir de novo é mais simples que comparar item a item, e a
// lista é pequena (no máximo 20).
func SaveContacts(ctx context.Context, s ContactStore, organizationID, partyID int64, contacts []ContactInput) error {
	if err := s.DeletePartyContacts(ctx, db.DeletePartyContactsParams{
		OrganizationID: organizationID, PartyID: partyID,
	}); err != nil {
		return err
	}
	for i, c := range contacts {
		if err := s.CreatePartyContact(ctx, db.CreatePartyContactParams{
			OrganizationID: organizationID, PartyID: partyID, Name: c.Name, Role: c.Role,
			Phone: c.Phone, IsWhatsapp: c.IsWhatsapp, Email: c.Email, Position: int32(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

// Timestamps aparece nas respostas de qualquer cadastro.
type Timestamps struct {
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
