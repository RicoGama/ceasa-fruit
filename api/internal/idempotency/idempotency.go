// Package idempotency garante que o mesmo pedido, enviado duas vezes, grava
// uma vez só.
//
// O problema: o Wi-Fi do box falha. A pessoa clica em "Salvar", o pedido
// chega e é gravado, mas a resposta se perde; a tela mostra erro e a pessoa
// tenta de novo. Sem proteção, isso vira duas vendas, dois pagamentos, duas
// baixas no estoque.
//
// Como funciona (o mesmo padrão dos meios de pagamento):
//  1. a tela manda um cabeçalho Idempotency-Key em todo POST/PUT/PATCH/DELETE;
//     se tentar de novo uma ação cujo resultado ficou incerto, manda a MESMA
//     chave (web/app/plugins/api.ts);
//  2. a chave é gravada DENTRO da transação da gravação de verdade
//     (internal/postgres/tx.go): ou fica tudo, ou nada;
//  3. este middleware guarda a resposta e, quando a chave se repete:
//     - já terminou e é o mesmo pedido: devolve a MESMA resposta, sem gravar;
//     - ainda está em andamento: 409, "aguarde";
//     - mesma chave com outro conteúdo: 422 (erro de programa, não do usuário).
//
// Pedido sem o cabeçalho funciona como sempre (scripts, testes).
package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// Header é o nome do cabeçalho (o mesmo do rascunho de padrão da IETF).
const Header = "Idempotency-Key"

// ReplayedHeader marca uma resposta devolvida do guardado.
const ReplayedHeader = "Idempotent-Replayed"

// Retention: por quanto tempo as chaves ficam guardadas.
const Retention = 7 * 24 * time.Hour

// Uma chave em andamento há mais que isso, sem resposta guardada, quer dizer
// que a API caiu entre gravar e responder: a gravação existe.
const stuckAfter = time.Minute

const maxStoredBody = 1 << 20 // 1 MB

var keyRe = regexp.MustCompile(`^[A-Za-z0-9_-]{16,100}$`)

// Rotas que não entram: login/logout (a resposta traz o cookie da sessão, que
// não pode ser "repetido"), o painel da fila e o tempo real.
func skip(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return true
	}
	p := r.URL.Path
	return strings.HasPrefix(p, "/auth/") || strings.HasPrefix(p, "/admin/") || p == "/events"
}

// Middleware protege os pedidos que mandam Idempotency-Key. Deve ficar depois
// do middleware de sessão (a chave vale por usuário).
func Middleware(database postgres.DB, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(Header)
			if key == "" || skip(r) {
				next.ServeHTTP(w, r)
				return
			}
			if !keyRe.MatchString(key) {
				problem(w, http.StatusBadRequest, "invalid Idempotency-Key")
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
			if err != nil {
				problem(w, http.StatusBadRequest, "could not read the request")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			var userID *int64
			if s, ok := auth.FromContext(r.Context()); ok {
				userID = &s.UserID
			}
			claim := &postgres.IdempotencyClaim{
				Key: key, UserID: userID, Method: r.Method, Path: r.URL.RequestURI(),
				Hash: fingerprint(userID, r.Method, r.URL.RequestURI(), body),
			}

			// Já vimos esta chave?
			var prev db.IdempotencyKey
			err = database.InReadTx(r.Context(), func(tx postgres.Tx) error {
				var err error
				prev, err = tx.GetIdempotencyKey(r.Context(), key)
				return err
			})
			switch {
			case err == nil:
				replay(w, prev, claim.Hash)
				return
			case !errors.Is(err, pgx.ErrNoRows):
				logger.Error("idempotency: lookup failed", "error", err)
				problem(w, http.StatusInternalServerError, "internal server error")
				return
			}

			// Primeira vez: executa, guardando a resposta.
			rec := &recorder{ResponseWriter: w, status: http.StatusOK}
			ctx := postgres.WithIdempotency(r.Context(), claim)
			next.ServeHTTP(rec, r.WithContext(ctx))

			// Só guarda se alguma gravação aconteceu (a chave entrou no banco
			// com ela). Erro antes de gravar (validação, permissão): nada é
			// guardado e a tela pode tentar de novo, corrigida, com a mesma chave.
			if !claim.Claimed() {
				return
			}
			saveCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()
			stored := rec.body.Bytes()
			if rec.truncated {
				stored = nil
			}
			ct := rec.Header().Get("Content-Type")
			if err := database.InTx(saveCtx, func(tx postgres.Tx) error {
				return tx.CompleteIdempotencyKey(saveCtx, db.CompleteIdempotencyKeyParams{
					Key: key, Status: ptr(int16(rec.status)), ContentType: &ct, ResponseBody: stored,
				})
			}); err != nil {
				// A gravação está feita; só a resposta não ficou guardada. Uma
				// nova tentativa recebe "já registrado, recarregue".
				logger.Error("idempotency: could not store the response", "key", key, "error", err)
			}
		})
	}
}

// replay responde a um pedido repetido.
func replay(w http.ResponseWriter, prev db.IdempotencyKey, hash []byte) {
	switch {
	case !bytes.Equal(prev.RequestHash, hash):
		problem(w, http.StatusUnprocessableEntity, "Idempotency-Key reused with a different request")
	case prev.Status == nil && time.Since(prev.CreatedAt) < stuckAfter:
		w.Header().Set("Retry-After", "1")
		problem(w, http.StatusConflict, "this request is already being processed")
	case prev.Status == nil:
		problem(w, http.StatusConflict, "this operation was already recorded; reload to see the result")
	default:
		if prev.ContentType != nil && *prev.ContentType != "" {
			w.Header().Set("Content-Type", *prev.ContentType)
		}
		w.Header().Set(ReplayedHeader, "true")
		w.WriteHeader(int(*prev.Status))
		_, _ = w.Write(prev.ResponseBody)
	}
}

// fingerprint identifica o pedido: a mesma chave só vale para o mesmo usuário,
// a mesma rota e o mesmo conteúdo.
func fingerprint(userID *int64, method, path string, body []byte) []byte {
	h := sha256.New()
	var uid [8]byte
	if userID != nil {
		binary.BigEndian.PutUint64(uid[:], uint64(*userID))
	}
	h.Write(uid[:])
	h.Write([]byte(method + " " + path + "\n"))
	h.Write(body)
	return h.Sum(nil)
}

// recorder repassa a resposta ao navegador e guarda uma cópia.
type recorder struct {
	http.ResponseWriter
	status    int
	body      bytes.Buffer
	truncated bool
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.body.Len()+len(b) <= maxStoredBody {
		r.body.Write(b)
	} else {
		r.truncated = true
	}
	return r.ResponseWriter.Write(b)
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// problem responde no mesmo formato de erro do Huma (RFC 9457).
func problem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"title": http.StatusText(status), "status": status, "detail": detail,
	})
}

// Cleanup apaga as chaves mais velhas que Retention, a cada hora, até o ctx acabar.
func Cleanup(ctx context.Context, database postgres.DB, logger *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		var n int64
		err := database.InTx(ctx, func(tx postgres.Tx) error {
			var err error
			n, err = tx.DeleteOldIdempotencyKeys(ctx, time.Now().Add(-Retention))
			return err
		})
		if err != nil && ctx.Err() == nil {
			logger.Warn("idempotency: cleanup failed", "error", err)
		} else if n > 0 {
			logger.Info("idempotency: old keys deleted", "rows", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func ptr[T any](v T) *T { return &v }
