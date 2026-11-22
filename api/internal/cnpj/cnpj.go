// Package cnpj consulta os dados públicos de um CNPJ na Receita Federal
// (razão social, nome fantasia, situação, endereço), para preencher os
// cadastros de fornecedor, cliente e da empresa.
//
// A Receita não tem uma API aberta; usamos serviços gratuitos que publicam a
// base dela, um de reserva do outro:
//  1. BrasilAPI  — https://brasilapi.com.br/api/cnpj/v1/{cnpj}
//  2. CNPJá      — https://open.cnpja.com/office/{cnpj}  (aberta, 5 por minuto)
//
// A consulta é feita pela API do box (e não pelo navegador): assim guardamos o
// resultado por 24 h (os serviços gratuitos limitam consultas) e, sem internet,
// a tela recebe uma mensagem clara para preencher à mão.
package cnpj

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/RicoGama/ceasa-fruit/api/internal/party"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CNPJData é o que a Receita informa de um CNPJ, já no formato do sistema.
type CNPJData struct {
	CNPJ         string      `json:"cnpj"`
	LegalName    string      `json:"legalName" doc:"Razão social"`
	TradeName    *string     `json:"tradeName" doc:"Nome fantasia"`
	Status       string      `json:"status" doc:"Situação cadastral como a Receita escreve: ATIVA, BAIXADA, INAPTA, SUSPENSA, NULA"`
	Active       bool        `json:"active" doc:"Situação ATIVA"`
	OpenedOn     *string     `json:"openedOn" doc:"Início da atividade (AAAA-MM-DD)"`
	MainActivity *string     `json:"mainActivity" doc:"Atividade principal (CNAE)"`
	Simples      *bool       `json:"simples" doc:"Optante pelo Simples Nacional"`
	Address      CNPJAddress `json:"address"`
	Phone        *string     `json:"phone" doc:"Só os números, com DDD"`
	Email        *string     `json:"email"`
	Source       string      `json:"source" doc:"Serviço que respondeu"`

	// Preenchidos pelos provedores; o município de verdade é resolvido no banco.
	cityIBGE  int32
	cityName  string
	cityState string
}

// CNPJAddress é o endereço do CNPJ. City vem do cadastro de municípios do IBGE.
type CNPJAddress struct {
	PostalCode *string     `json:"postalCode"`
	Street     *string     `json:"street"`
	Number     *string     `json:"number"`
	Complement *string     `json:"complement"`
	District   *string     `json:"district"`
	City       *party.City `json:"city"`
}

var (
	// ErrNotFound: o CNPJ não existe na Receita.
	ErrNotFound = errors.New("cnpj not found")
	// ErrUnavailable: nenhum serviço respondeu (sem internet, fora do ar,
	// limite de consultas).
	ErrUnavailable = errors.New("cnpj lookup unavailable")
)

// Provider é um serviço que sabe consultar um CNPJ.
type Provider interface {
	Name() string
	Lookup(ctx context.Context, cnpj string) (CNPJData, error)
}

// CityResolver acha o município no cadastro do IBGE (pelo código ou pelo nome).
type CityResolver func(ctx context.Context, ibge int32, name, state string) (*party.City, error)

// Service consulta os provedores em ordem, com cache.
type Service struct {
	Providers []Provider
	Cities    CityResolver
	Logger    *slog.Logger
	TTL       time.Duration // padrão 24 h

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	at      time.Time
	company CNPJData
	err     error // ErrNotFound também fica guardado (não pergunta de novo)
}

// Lookup devolve os dados do CNPJ (só dígitos e letras, 14 caracteres).
func (s *Service) Lookup(ctx context.Context, cnpj string) (CNPJData, error) {
	ttl := s.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	s.mu.Lock()
	if c, ok := s.cache[cnpj]; ok && time.Since(c.at) < ttl {
		s.mu.Unlock()
		return c.company, c.err
	}
	s.mu.Unlock()

	var lastErr error
	for _, p := range s.Providers {
		c, err := p.Lookup(ctx, cnpj)
		if err == nil || errors.Is(err, ErrNotFound) {
			if err == nil {
				c.Source = p.Name()
				if s.Cities != nil && (c.cityIBGE != 0 || c.cityName != "") {
					city, cerr := s.Cities(ctx, c.cityIBGE, c.cityName, c.cityState)
					if cerr != nil {
						return CNPJData{}, cerr
					}
					c.Address.City = city
				}
			}
			s.remember(cnpj, c, err)
			return c, err
		}
		if s.Logger != nil {
			s.Logger.Warn("cnpj: provider failed", "provider", p.Name(), "error", err)
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return CNPJData{}, fmt.Errorf("%w: %v", ErrUnavailable, lastErr)
}

func (s *Service) remember(cnpj string, c CNPJData, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil {
		s.cache = map[string]cached{}
	}
	s.cache[cnpj] = cached{at: time.Now(), company: c, err: err}
}

// ---------------------------------------------------------------------------
// BrasilAPI
// ---------------------------------------------------------------------------

// BrasilAPI consulta https://brasilapi.com.br/api/cnpj/v1/{cnpj}.
type BrasilAPI struct {
	BaseURL string // padrão https://brasilapi.com.br/api/cnpj/v1
	Client  *http.Client
}

func (BrasilAPI) Name() string { return "BrasilAPI" }

func (b BrasilAPI) Lookup(ctx context.Context, cnpj string) (CNPJData, error) {
	base := b.BaseURL
	if base == "" {
		base = "https://brasilapi.com.br/api/cnpj/v1"
	}
	var r struct {
		CNPJ                string      `json:"cnpj"`
		RazaoSocial         string      `json:"razao_social"`
		NomeFantasia        string      `json:"nome_fantasia"`
		DescricaoSituacao   string      `json:"descricao_situacao_cadastral"`
		DataInicioAtividade string      `json:"data_inicio_atividade"`
		CNAEDescricao       string      `json:"cnae_fiscal_descricao"`
		OpcaoPeloSimples    *bool       `json:"opcao_pelo_simples"`
		CEP                 string      `json:"cep"`
		TipoLogradouro      string      `json:"descricao_tipo_de_logradouro"`
		Logradouro          string      `json:"logradouro"`
		Numero              string      `json:"numero"`
		Complemento         string      `json:"complemento"`
		Bairro              string      `json:"bairro"`
		Municipio           string      `json:"municipio"`
		UF                  string      `json:"uf"`
		CodigoMunicipioIBGE json.Number `json:"codigo_municipio_ibge"`
		DDDTelefone1        string      `json:"ddd_telefone_1"`
		Email               *string     `json:"email"`
	}
	if err := getJSON(ctx, b.Client, base+"/"+cnpj, &r); err != nil {
		return CNPJData{}, err
	}
	street := strings.TrimSpace(strings.TrimSpace(r.TipoLogradouro) + " " + r.Logradouro)
	ibge, _ := strconv.Atoi(r.CodigoMunicipioIBGE.String())
	return CNPJData{
		CNPJ:         firstNonEmpty(onlyAlnum(r.CNPJ), cnpj),
		LegalName:    strings.TrimSpace(r.RazaoSocial),
		TradeName:    opt(r.NomeFantasia),
		Status:       strings.ToUpper(strings.TrimSpace(r.DescricaoSituacao)),
		Active:       strings.EqualFold(strings.TrimSpace(r.DescricaoSituacao), "ATIVA"),
		OpenedOn:     opt(r.DataInicioAtividade),
		MainActivity: opt(r.CNAEDescricao),
		Simples:      r.OpcaoPeloSimples,
		Address: CNPJAddress{
			PostalCode: opt(onlyDigits(r.CEP)), Street: opt(titleCase(street)), Number: opt(r.Numero),
			Complement: opt(titleCase(r.Complemento)), District: opt(titleCase(r.Bairro)),
		},
		Phone:     opt(onlyDigits(r.DDDTelefone1)),
		Email:     lowerOpt(r.Email),
		cityIBGE:  int32(ibge),
		cityName:  r.Municipio,
		cityState: r.UF,
	}, nil
}

// ---------------------------------------------------------------------------
// CNPJá (API aberta)
// ---------------------------------------------------------------------------

// CNPJa consulta https://open.cnpja.com/office/{cnpj}.
type CNPJa struct {
	BaseURL string // padrão https://open.cnpja.com/office
	Client  *http.Client
}

func (CNPJa) Name() string { return "CNPJá" }

func (c CNPJa) Lookup(ctx context.Context, cnpj string) (CNPJData, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://open.cnpja.com/office"
	}
	var r struct {
		TaxID    string `json:"taxId"`
		Alias    string `json:"alias"`
		Founded  string `json:"founded"`
		Company struct {
			Name    string `json:"name"`
			Simples *struct {
				Optant *bool `json:"optant"`
			} `json:"simples"`
		} `json:"company"`
		Status struct {
			Text string `json:"text"`
		} `json:"status"`
		Address struct {
			Municipality json.Number `json:"municipality"`
			Street       string      `json:"street"`
			Number       string      `json:"number"`
			Details      string      `json:"details"`
			District     string      `json:"district"`
			City         string      `json:"city"`
			State        string      `json:"state"`
			Zip          string      `json:"zip"`
		} `json:"address"`
		Phones []struct {
			Area   string `json:"area"`
			Number string `json:"number"`
		} `json:"phones"`
		Emails []struct {
			Address string `json:"address"`
		} `json:"emails"`
		MainActivity struct {
			Text string `json:"text"`
		} `json:"mainActivity"`
	}
	if err := getJSON(ctx, c.Client, base+"/"+cnpj, &r); err != nil {
		return CNPJData{}, err
	}
	out := CNPJData{
		CNPJ:         firstNonEmpty(onlyAlnum(r.TaxID), cnpj),
		LegalName:    strings.TrimSpace(r.Company.Name),
		TradeName:    opt(r.Alias),
		Status:       strings.ToUpper(strings.TrimSpace(r.Status.Text)),
		Active:       strings.EqualFold(strings.TrimSpace(r.Status.Text), "ATIVA"),
		OpenedOn:     opt(r.Founded),
		MainActivity: opt(r.MainActivity.Text),
		Address: CNPJAddress{
			PostalCode: opt(onlyDigits(r.Address.Zip)), Street: opt(titleCase(r.Address.Street)), Number: opt(r.Address.Number),
			Complement: opt(titleCase(r.Address.Details)), District: opt(titleCase(r.Address.District)),
		},
		cityName:  r.Address.City,
		cityState: r.Address.State,
	}
	if r.Company.Simples != nil {
		out.Simples = r.Company.Simples.Optant
	}
	if ibge, err := strconv.Atoi(r.Address.Municipality.String()); err == nil {
		out.cityIBGE = int32(ibge)
	}
	if len(r.Phones) > 0 {
		out.Phone = opt(onlyDigits(r.Phones[0].Area + r.Phones[0].Number))
	}
	if len(r.Emails) > 0 {
		e := r.Emails[0].Address
		out.Email = lowerOpt(&e)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------------

var defaultClient = &http.Client{Timeout: 10 * time.Second}

// getJSON busca e decodifica. 404 vira ErrNotFound; outros erros (rede, 429
// de limite, 5xx) fazem tentar o próximo serviço.
func getJSON(ctx context.Context, client *http.Client, url string, out any) error {
	if client == nil {
		client = defaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ceasa-fruit")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode >= 300:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

var (
	nonDigits = regexp.MustCompile(`\D`)
	nonAlnum  = regexp.MustCompile(`[^0-9A-Za-z]`)
)

func onlyDigits(s string) string { return nonDigits.ReplaceAllString(s, "") }
func onlyAlnum(s string) string  { return strings.ToUpper(nonAlnum.ReplaceAllString(s, "")) }

func opt(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func lowerOpt(s *string) *string {
	if s == nil {
		return nil
	}
	return opt(strings.ToLower(*s))
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// titleCase: a Receita escreve tudo em maiúsculas ("AVENIDA BRASIL"); no
// cadastro fica "Avenida Brasil", com "de", "da", "dos"... em minúsculas.
func titleCase(s string) string {
	small := map[string]bool{"de": true, "da": true, "do": true, "das": true, "dos": true, "e": true}
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if i > 0 && small[w] {
			continue
		}
		r := []rune(w)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}
