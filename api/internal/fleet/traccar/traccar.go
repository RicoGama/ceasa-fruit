// Package traccar conversa com a API do Traccar, o servidor de rastreamento
// (código aberto) que fica na nuvem e recebe as posições dos rastreadores.
//
// Só o que o sistema usa:
//   - GET  /api/devices              aparelhos cadastrados (um por veículo, pelo IMEI)
//   - POST /api/devices              cadastra o aparelho de um veículo novo
//   - PUT  /api/devices/{id}         atualiza o nome (a placa) do aparelho
//   - GET  /api/positions?deviceId&from&to   posições de um período
//
// Autenticação por token (no Traccar: Configurações > Preferências > Token),
// mandado como "Authorization: Bearer <token>".
//
// Documentação da API: https://www.traccar.org/api-reference/
package traccar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client fala com um servidor Traccar.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New cria o cliente. baseURL é o endereço do Traccar, ex.: https://rastreio.empresa.com.br
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		// Prazo curto: a internet do box cai; melhor desistir e tentar de novo
		// na próxima rodada do que ficar pendurado.
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// Device é um rastreador cadastrado no Traccar.
type Device struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	UniqueID   string     `json:"uniqueId"` // o IMEI
	Status     string     `json:"status"`   // online, offline, unknown
	LastUpdate *time.Time `json:"lastUpdate"`
	PositionID int64      `json:"positionId"`
	Disabled   bool       `json:"disabled"`
	// O Traccar exige o objeto inteiro no PUT; guardamos o resto como veio.
	Raw map[string]any `json:"-"`
}

// Position é uma posição como o Traccar devolve.
type Position struct {
	ID         int64          `json:"id"`
	DeviceID   int64          `json:"deviceId"`
	ServerTime time.Time      `json:"serverTime"`
	DeviceTime time.Time      `json:"deviceTime"`
	FixTime    time.Time      `json:"fixTime"`
	Valid      bool           `json:"valid"`
	Latitude   float64        `json:"latitude"`
	Longitude  float64        `json:"longitude"`
	Altitude   float64        `json:"altitude"`
	Speed      float64        `json:"speed"` // em NÓS (padrão do Traccar)
	Course     float64        `json:"course"`
	Attributes map[string]any `json:"attributes"`
}

// SpeedKmh converte a velocidade de nós para km/h.
func (p Position) SpeedKmh() float64 { return p.Speed * 1.852 }

// Bool lê um atributo verdadeiro/falso (ignition, motion...). nil = o
// rastreador não mandou.
func (p Position) Bool(name string) *bool {
	v, ok := p.Attributes[name].(bool)
	if !ok {
		return nil
	}
	return &v
}

// Devices lista os aparelhos que este usuário do Traccar enxerga.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var raw []map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/devices", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Device, 0, len(raw))
	for _, r := range raw {
		b, _ := json.Marshal(r)
		var d Device
		if err := json.Unmarshal(b, &d); err != nil {
			return nil, fmt.Errorf("traccar: device: %w", err)
		}
		d.Raw = r
		out = append(out, d)
	}
	return out, nil
}

// CreateDevice cadastra um rastreador (nome = placa, uniqueId = IMEI).
func (c *Client) CreateDevice(ctx context.Context, name, imei string) (Device, error) {
	var d Device
	err := c.do(ctx, http.MethodPost, "/api/devices", map[string]any{"name": name, "uniqueId": imei}, &d)
	return d, err
}

// RenameDevice troca o nome do aparelho (quando a placa muda).
func (c *Client) RenameDevice(ctx context.Context, d Device, name string) error {
	body := map[string]any{}
	for k, v := range d.Raw {
		body[k] = v
	}
	body["name"] = name
	return c.do(ctx, http.MethodPut, "/api/devices/"+strconv.FormatInt(d.ID, 10), body, nil)
}

// Positions devolve as posições de um aparelho com fixTime entre from e to.
func (c *Client) Positions(ctx context.Context, deviceID int64, from, to time.Time) ([]Position, error) {
	q := url.Values{}
	q.Set("deviceId", strconv.FormatInt(deviceID, 10))
	q.Set("from", from.UTC().Format(time.RFC3339))
	q.Set("to", to.UTC().Format(time.RFC3339))
	var out []Position
	err := c.do(ctx, http.MethodGet, "/api/positions?"+q.Encode(), nil, &out)
	return out, err
}

// Error é uma resposta de erro do Traccar.
type Error struct {
	Status int
	Body   string
}

func (e *Error) Error() string {
	if e.Status == http.StatusUnauthorized {
		return "traccar: token recusado (401): confira TRACCAR_TOKEN"
	}
	return fmt.Sprintf("traccar: HTTP %d: %s", e.Status, e.Body)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("traccar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return &Error{Status: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("traccar: resposta inválida: %w", err)
	}
	return nil
}
