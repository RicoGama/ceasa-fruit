package certificate

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
	"github.com/RicoGama/ceasa-fruit/api/internal/secrets"
)

// ExpiringDays: a partir de quantos dias antes do vencimento o sistema avisa.
const ExpiringDays = 30

// Rótulos usados na criptografia (internal/secrets): o arquivo não abre como
// senha e vice-versa.
const (
	labelPFX      = "certificate.pfx"
	labelPassword = "certificate.password"
)

// UserRef: quem enviou.
type UserRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// CertificateInfo é o que a API mostra do certificado. Nunca o arquivo nem a senha.
type CertificateInfo struct {
	ID           int64      `json:"id"`
	Active       bool       `json:"active"`
	HolderName   string     `json:"holderName" doc:"Certificate holder (legal name)"`
	Document     *string    `json:"document" doc:"Holder CNPJ"`
	Issuer       string     `json:"issuer" doc:"Certificate authority"`
	SerialNumber string     `json:"serialNumber"`
	Fingerprint  string     `json:"fingerprint" doc:"SHA-256 of the certificate"`
	NotBefore    time.Time  `json:"notBefore"`
	NotAfter     time.Time  `json:"notAfter" doc:"Expiration"`
	DaysLeft     int        `json:"daysLeft" doc:"Whole days until expiration (negative = expired)"`
	Status       string     `json:"status" enum:"valid,expiring,expired" doc:"expiring = less than 30 days left"`
	UploadedAt   time.Time  `json:"uploadedAt"`
	UploadedBy   *UserRef   `json:"uploadedBy"`
	ReplacedAt   *time.Time `json:"replacedAt"`
}

type certificatesOutput struct {
	Body struct {
		Current       *CertificateInfo  `json:"current" doc:"The certificate in use (null = none)"`
		History       []CertificateInfo `json:"history" doc:"Previous certificates (metadata only), newest first"`
		KeyConfigured bool              `json:"keyConfigured" doc:"false = the server has no SECRETS_KEY and cannot receive a certificate"`
	}
}

type uploadInput struct {
	RawBody huma.MultipartFormFiles[struct {
		File     huma.FormFile `form:"file" required:"true" doc:"The A1 certificate file (.pfx or .p12)"`
		Password string        `form:"password" required:"true" doc:"The certificate password"`
	}]
}

type handler struct {
	db  postgres.DB
	box *secrets.Box // nil: SECRETS_KEY não configurada
	now func() time.Time
}

// Register adiciona as rotas. Todas exigem certificate.manage.
func Register(api huma.API, database postgres.DB, box *secrets.Box) {
	h := &handler{db: database, box: box, now: time.Now}
	tags := []string{"Certificate"}

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "get-certificate", Method: http.MethodGet, Path: "/company/certificate",
		Summary: "Digital certificate in use and previous ones", Tags: tags,
	}, auth.CertificateManage), h.get)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "upload-certificate", Method: http.MethodPost, Path: "/company/certificate",
		Summary:       "Send (or replace) the A1 digital certificate",
		Description:   "The file is checked (password, private key, validity, CNPJ of the company) and stored encrypted. The previous certificate is kept only as history.",
		Tags:          tags,
		DefaultStatus: http.StatusCreated,
	}, auth.CertificateManage), h.upload)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "delete-certificate", Method: http.MethodDelete, Path: "/company/certificate",
		Summary: "Remove the certificate in use (e.g. it leaked)", Tags: tags,
		DefaultStatus: http.StatusNoContent,
	}, auth.CertificateManage), h.remove)
}

func (h *handler) get(ctx context.Context, _ *struct{}) (*certificatesOutput, error) {
	out := &certificatesOutput{}
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		return h.fill(ctx, tx, out)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *handler) upload(ctx context.Context, in *uploadInput) (*certificatesOutput, error) {
	if h.box == nil {
		return nil, huma.Error422UnprocessableEntity("secrets key not configured on the server (SECRETS_KEY)")
	}
	form := in.RawBody.Data()
	pfx, err := io.ReadAll(io.LimitReader(form.File, MaxFileSize+1))
	if err != nil {
		return nil, huma.Error400BadRequest("could not read the file")
	}
	parsed, err := Parse(pfx, form.Password, h.now())
	if err != nil {
		return nil, fileError(err)
	}

	var actor *int64
	if s, ok := auth.FromContext(ctx); ok {
		actor = &s.UserID
	}
	out := &certificatesOutput{}
	err = h.db.InTx(ctx, func(tx postgres.Tx) error {
		// Trava a empresa: duas trocas ao mesmo tempo viram uma depois da outra.
		company, err := tx.LockCompany(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("company not configured")
		}
		if err != nil {
			return err
		}
		// A SEFAZ exige que a raiz do CNPJ (8 primeiros dígitos) do certificado
		// seja a da empresa; a matriz pode assinar pelas filiais.
		if company.Document == nil || len(*company.Document) != 14 {
			return huma.Error422UnprocessableEntity("fill in the company CNPJ before sending the certificate")
		}
		if (*company.Document)[:8] != parsed.Document[:8] {
			return huma.Error422UnprocessableEntity("certificate CNPJ does not match the company CNPJ")
		}
		if _, err := tx.RetireActiveCertificate(ctx); err != nil {
			return err
		}
		doc := parsed.Document
		if _, err := tx.InsertCertificate(ctx, db.InsertCertificateParams{
			HolderName: parsed.HolderName, Document: &doc, Issuer: parsed.Issuer,
			SerialNumber: parsed.SerialNumber, Fingerprint: parsed.Fingerprint,
			NotBefore: parsed.NotBefore, NotAfter: parsed.NotAfter,
			EncryptedPfx:      h.box.Seal(pfx, labelPFX),
			EncryptedPassword: h.box.Seal([]byte(form.Password), labelPassword),
			KeyID:             ptr(h.box.KeyID()),
			UploadedBy:        actor,
		}); err != nil {
			return err
		}
		return h.fill(ctx, tx, out)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *handler) remove(ctx context.Context, _ *struct{}) (*struct{}, error) {
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		if _, err := tx.LockCompany(ctx); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		n, err := tx.RetireActiveCertificate(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			return huma.Error404NotFound("no certificate in use")
		}
		return nil
	})
	return nil, err
}

// fill monta a resposta: o certificado em uso e os anteriores.
func (h *handler) fill(ctx context.Context, tx postgres.Tx, out *certificatesOutput) error {
	rows, err := tx.ListCertificates(ctx)
	if err != nil {
		return err
	}
	now := h.now()
	out.Body.KeyConfigured = h.box != nil
	out.Body.History = []CertificateInfo{}
	for _, r := range rows {
		info := CertificateInfo{
			ID: r.ID, Active: r.Active, HolderName: r.HolderName, Document: r.Document,
			Issuer: r.Issuer, SerialNumber: r.SerialNumber, Fingerprint: r.Fingerprint,
			NotBefore: r.NotBefore, NotAfter: r.NotAfter, UploadedAt: r.CreatedAt, ReplacedAt: r.ReplacedAt,
		}
		info.DaysLeft, info.Status = Status(r.NotAfter, now)
		if r.UploadedBy != nil && r.UploadedByName != nil {
			info.UploadedBy = &UserRef{ID: *r.UploadedBy, Name: *r.UploadedByName}
		}
		if r.Active {
			out.Body.Current = &info
		} else {
			out.Body.History = append(out.Body.History, info)
		}
	}
	return nil
}

// Status diz quantos dias faltam (arredondado para baixo) e a situação.
func Status(notAfter, now time.Time) (int, string) {
	days := int(math.Floor(notAfter.Sub(now).Hours() / 24))
	switch {
	case !now.Before(notAfter):
		return days, "expired"
	case days < ExpiringDays:
		return days, "expiring"
	}
	return days, "valid"
}

// fileError traduz os erros de conferência do arquivo para respostas HTTP.
func fileError(err error) error {
	switch {
	case errors.Is(err, ErrWrongPassword):
		return huma.Error422UnprocessableEntity(err.Error())
	case errors.Is(err, ErrInvalidFile), errors.Is(err, ErrNoPrivateKey), errors.Is(err, ErrKeyMismatch),
		errors.Is(err, ErrExpired), errors.Is(err, ErrNotYetValid), errors.Is(err, ErrNoDocument):
		return huma.Error422UnprocessableEntity(err.Error())
	}
	return err
}

func ptr[T any](v T) *T { return &v }
