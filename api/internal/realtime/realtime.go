// Package realtime leva as mudanças do banco até as telas abertas, na hora.
//
// Caminho de um aviso:
//
//	gravação (qualquer uma) -> gatilho no Postgres -> NOTIFY app_events (só após o COMMIT)
//	  -> Hub (uma conexão escutando com LISTEN)
//	  -> cada navegador conectado em GET /events (Server-Sent Events)
//	  -> o front recarrega a lista ou o cadastro daquele assunto
//
// O aviso é só "assunto + id" (ex.: {"topic":"suppliers","id":42}). Os dados
// em si o front busca pela API, que confere as permissões como sempre. Mesmo
// assim, cada pessoa só recebe avisos dos assuntos que pode ver.
//
// Por que SSE e não WebSocket? O tráfego é de mão única (servidor -> tela), o
// SSE é HTTP comum (passa pelo proxy do Nuxt, usa o cookie de sessão) e o
// navegador reconecta sozinho quando a rede cai.
package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
)

// Channel é o canal do Postgres onde os gatilhos avisam (migration 00007).
const Channel = "app_events"

// Event é um aviso de mudança.
//   - topic "resync": a API ficou um tempo sem ouvir o banco (ou a conexão
//     caiu); o front recarrega tudo o que está mostrando;
//   - topic "me": mudou o usuário logado ou o papel dele; o front relê as
//     permissões.
type Event struct {
	Topic string `json:"topic"`
	ID    int64  `json:"id"`
}

// Hub escuta o banco e distribui os avisos para quem está conectado.
type Hub struct {
	connConfig *pgx.ConnConfig
	logger     *slog.Logger

	mu     sync.Mutex
	subs   map[chan Event]struct{}
	closed bool

	// OnAlive é chamado ao conectar e depois a cada minuto enquanto a tela
	// estiver aberta (usado para a "última atividade" do registro de acessos:
	// quem está só olhando a tela também conta como presente). Opcional.
	OnAlive func(ctx context.Context, s *auth.Session)
}

// New cria o Hub. connConfig é a configuração de conexão do banco: o Hub abre
// uma conexão própria (fora do pool), que fica dedicada ao LISTEN.
func New(connConfig *pgx.ConnConfig, logger *slog.Logger) *Hub {
	return &Hub{connConfig: connConfig, logger: logger, subs: map[chan Event]struct{}{}}
}

// Run escuta o banco até o ctx acabar. Se a conexão cair, tenta de novo com
// espera crescente (até 30s) e, ao voltar, manda "resync" para todos: os
// avisos do período sem conexão se perderam.
func (h *Hub) Run(ctx context.Context) {
	wait := time.Second
	for {
		err := h.listen(ctx)
		if ctx.Err() != nil {
			return
		}
		h.logger.Warn("realtime: lost database connection, retrying", "error", err, "in", wait)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}
		wait = min(wait*2, 30*time.Second)
	}
}

func (h *Hub) listen(ctx context.Context) error {
	conn, err := pgx.ConnectConfig(ctx, h.connConfig)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return err
	}
	h.logger.Info("realtime: listening for changes")
	h.Publish(Event{Topic: "resync"}) // pode ter perdido algo enquanto estava fora

	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var ev Event
		if err := json.Unmarshal([]byte(n.Payload), &ev); err != nil {
			h.logger.Error("realtime: bad payload", "payload", n.Payload, "error", err)
			continue
		}
		h.Publish(ev)
	}
}

// Publish entrega o aviso a todos os conectados. Quem está com a fila cheia
// (tela travada, rede lenta) é desconectado: o navegador reconecta sozinho e
// recebe um "resync", em vez de travar a entrega para todo mundo.
func (h *Hub) Publish(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
			delete(h.subs, ch)
			close(ch)
		}
	}
}

// subscribe registra um ouvinte. O cancel devolvido tira o registro.
func (h *Hub) subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan Event, 64)
	if h.closed {
		close(ch)
		return ch, func() {}
	}
	h.subs[ch] = struct{}{}
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
	}
}

// Close desconecta todos (usado no desligamento do servidor: sem isso, as
// conexões SSE, que nunca terminam sozinhas, segurariam o desligamento).
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}

// ---------------------------------------------------------------------------
// Quem recebe o quê
// ---------------------------------------------------------------------------

// topicPermission: quem precisa de qual permissão para receber cada assunto.
// Assunto que não está aqui (company, resync) vai para todos os logados.
var topicPermission = map[string]auth.Permission{
	"suppliers":  auth.SuppliersView,
	"customers":  auth.CustomersView,
	"access_log": auth.AuditView,
	"products":   auth.ProductsView,
	"users":      auth.UsersManage,
	"roles":      auth.UsersManage,
	// fornecedor × produto: a aba do fornecedor e a lista no produto
	"supplier_products": auth.SuppliersView,
	"product_suppliers": auth.SuppliersView,
}

// route decide o que esta sessão recebe de um aviso (pode ser nada, o próprio
// aviso, e/ou um "me" quando a mudança é no próprio usuário ou no papel dele).
func route(s *auth.Session, ev Event) []Event {
	var out []Event
	if p, ok := topicPermission[ev.Topic]; !ok || s.Can(p) {
		out = append(out, ev)
	}
	if (ev.Topic == "users" && ev.ID == s.UserID) || (ev.Topic == "roles" && ev.ID == s.RoleID) {
		out = append(out, Event{Topic: "me", ID: s.UserID})
	}
	return out
}

// ---------------------------------------------------------------------------
// GET /events (Server-Sent Events)
// ---------------------------------------------------------------------------

// heartbeat: um comentário a cada 25s mantém a conexão viva em proxies e
// roteadores que derrubam conexões paradas.
const heartbeat = 25 * time.Second

// ServeHTTP mantém a conexão aberta e escreve um "data: {...}" por aviso.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	// O servidor tem WriteTimeout (15s) para as rotas comuns; esta conexão
	// fica aberta o tempo todo, então tiramos o limite só dela.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // proxies não podem segurar os avisos

	events, cancel := h.subscribe()
	defer cancel()

	// retry: se a conexão cair, o navegador tenta de novo em 3s.
	if _, err := fmt.Fprint(w, "retry: 3000\n\n"); err != nil {
		return
	}
	_ = rc.Flush()

	alive := func() {
		if h.OnAlive != nil {
			h.OnAlive(r.Context(), s)
		}
	}
	alive()
	lastAlive := time.Now()

	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			if time.Since(lastAlive) >= time.Minute {
				alive()
				lastAlive = time.Now()
			}
		case ev, open := <-events:
			if !open {
				return // desconectado pelo Hub (fila cheia ou desligamento)
			}
			for _, out := range route(s, ev) {
				data, _ := json.Marshal(out)
				if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
					return
				}
			}
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
