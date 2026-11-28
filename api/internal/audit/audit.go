// Package audit mostra o histórico de alterações gravado pelo gatilho
// audit_row (migration 00013): quem mudou o quê, quando, de quanto para quanto.
//
// Duas rotas:
//   - GET /audit/{entityType}/{entityId}: o histórico de um cadastro (aba
//     "Histórico" do fornecedor, produto...). Quem pode ver o cadastro pode
//     ver o histórico dele.
//   - GET /audit: tudo, com filtros (tela do administrador).
//
// Tudo o que um "salvar" gravou (pessoa + fornecedor + contatos) tem o mesmo
// tx_id: vira um evento só, com a lista do que mudou em cada tabela.
package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// Quem pode ver o histórico de cada tipo de cadastro (qualquer uma das
// permissões). Quem tem audit.view vê tudo.
var entityPermissions = map[string][]auth.Permission{
	"party":   {auth.SuppliersView, auth.CustomersView},
	"product": {auth.ProductsView},
	"user":    {auth.UsersManage},
	"role":    {auth.UsersManage},
	"company": {auth.CompanyEdit},
	"vehicle": {auth.FleetView},
	"driver":  {auth.FleetView},
	// certificado digital: só o arquivo e a senha ficam de fora do histórico
	"certificate": {auth.CertificateManage},
}

// Campos que apontam para outro cadastro: a resposta traz o nome em refs.
var refFields = map[string]string{
	"product_id":  "product",
	"role_id":     "role",
	"driver_id":   "driver",
	"city_id":     "city",
	"supplier_id": "party",
	"party_id":    "party",
	"uploaded_by": "user",
}

// ---------------------------------------------------------------------------
// Tipos da API
// ---------------------------------------------------------------------------

// AuditField é um campo que mudou. Old/New vêm como estão no banco (texto,
// número, true/false, lista); a tela formata.
type AuditField struct {
	Name string          `json:"name" doc:"Column name, e.g. phone, consignment_commission"`
	Old  json.RawMessage `json:"old,omitempty" doc:"Value before (absent when created)"`
	New  json.RawMessage `json:"new,omitempty" doc:"Value after (absent when deleted)"`
}

// AuditChange é o que aconteceu com uma linha de uma tabela.
type AuditChange struct {
	Table  string       `json:"table" doc:"e.g. parties, party_contacts, suppliers"`
	RowID  *int64       `json:"rowId"`
	Action string       `json:"action" enum:"created,updated,deleted"`
	Fields []AuditField `json:"fields"`
}

// AuditRef é um cadastro com o nome para mostrar.
type AuditRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// AuditEvent é tudo o que uma pessoa gravou num "salvar" de um cadastro.
type AuditEvent struct {
	ID         int64         `json:"id" doc:"Id of the newest row of the event (for keys and pagination)"`
	OccurredAt time.Time     `json:"occurredAt"`
	Actor      *AuditRef     `json:"actor" doc:"null = the system (sync, seed, script)"`
	EntityType string        `json:"entityType" enum:"party,product,user,role,company,vehicle,driver,certificate"`
	EntityID   int64         `json:"entityId"`
	EntityName string        `json:"entityName"`
	Changes    []AuditChange `json:"changes"`

	txID int64
}

type historyBody struct {
	Events []AuditEvent `json:"events"`
	// Nomes dos cadastros citados nos campos, por "tipo:id" (ex.: "product:12").
	Refs map[string]string `json:"refs"`
	// Para "carregar mais": passe como ?before=. null = não há mais.
	NextBefore *int64 `json:"nextBefore"`
}

type historyOutput struct{ Body historyBody }

type entityInput struct {
	EntityType string `path:"entityType" enum:"party,product,user,role,company,vehicle,driver,certificate"`
	EntityID   int64  `path:"entityId" minimum:"1"`
	Before     int64  `query:"before" minimum:"0" doc:"Only rows older than this id"`
}

type listInput struct {
	Days       int32  `query:"days" minimum:"1" maximum:"366" default:"7"`
	ActorID    int64  `query:"actorId" minimum:"0" doc:"Only changes made by this user"`
	EntityType string `query:"entityType" enum:"party,product,user,role,company,vehicle,driver,certificate,all" default:"all"`
	Before     int64  `query:"before" minimum:"0"`
}

const (
	entityPageRows = 200
	listPageRows   = 300
)

// ---------------------------------------------------------------------------
// Rotas
// ---------------------------------------------------------------------------

type handler struct{ db postgres.DB }

// Register adiciona as rotas do histórico. api deve exigir login.
func Register(api huma.API, database postgres.DB) {
	h := &handler{db: database}
	tags := []string{"Audit"}

	// Permissão por tipo de cadastro: a rota aceita quem tem qualquer uma
	// delas, e o handler confere a do tipo pedido.
	var any []auth.Permission
	for _, perms := range entityPermissions {
		for _, p := range perms {
			if !slices.Contains(any, p) {
				any = append(any, p)
			}
		}
	}
	any = append(any, auth.AuditView)
	slices.Sort(any)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "get-entity-history", Method: http.MethodGet, Path: "/audit/{entityType}/{entityId}",
		Summary:     "AuditChange history of one record",
		Description: "Who changed what and when. Allowed for whoever can see that kind of record.",
		Tags:        tags,
	}, any...), h.entity)

	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "list-history", Method: http.MethodGet, Path: "/audit",
		Summary: "All changes (administrator)", Tags: tags,
	}, auth.AuditView), h.list)
}

func (h *handler) entity(ctx context.Context, in *entityInput) (*historyOutput, error) {
	s, _ := auth.FromContext(ctx)
	if !s.Can(auth.AuditView) && !slices.ContainsFunc(entityPermissions[in.EntityType], func(p auth.Permission) bool { return s.Can(p) }) {
		return nil, huma.Error403Forbidden("you do not have permission for this action")
	}
	var rows []db.AuditLog
	out := &historyOutput{}
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		var err error
		rows, err = tx.ListAuditForEntity(ctx, db.ListAuditForEntityParams{
			EntityType: in.EntityType, EntityID: in.EntityID, BeforeID: optional(in.Before), LimitRows: entityPageRows,
		})
		if err != nil {
			return err
		}
		out.Body, err = build(ctx, tx, rows, entityPageRows)
		return err
	})
	return out, err
}

func (h *handler) list(ctx context.Context, in *listInput) (*historyOutput, error) {
	var entity *string
	if in.EntityType != "all" {
		entity = &in.EntityType
	}
	out := &historyOutput{}
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		rows, err := tx.ListAudit(ctx, db.ListAuditParams{
			Since: time.Now().AddDate(0, 0, -int(in.Days)), ActorUserID: optional(in.ActorID),
			EntityType: entity, BeforeID: optional(in.Before), LimitRows: listPageRows,
		})
		if err != nil {
			return err
		}
		out.Body, err = build(ctx, tx, rows, listPageRows)
		return err
	})
	return out, err
}

// ---------------------------------------------------------------------------
// Montagem da resposta
// ---------------------------------------------------------------------------

// build junta as linhas (mais novas primeiro) em eventos e busca os nomes.
func build(ctx context.Context, tx postgres.Tx, rows []db.AuditLog, page int) (historyBody, error) {
	body := historyBody{Events: []AuditEvent{}, Refs: map[string]string{}}
	ids := map[string]map[int64]bool{} // tipo -> ids a buscar o nome
	want := func(kind string, id int64) {
		if ids[kind] == nil {
			ids[kind] = map[int64]bool{}
		}
		ids[kind][id] = true
	}

	for _, r := range rows {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(r.Changes, &raw); err != nil {
			return body, err
		}
		ch := AuditChange{Table: r.TableName, RowID: r.RowID, Action: map[string]string{"I": "created", "U": "updated", "D": "deleted"}[r.Action]}
		for name, v := range raw {
			if name == "id" {
				continue
			}
			f := AuditField{Name: name}
			if r.Action == "U" {
				var pair [2]json.RawMessage
				if err := json.Unmarshal(v, &pair); err != nil {
					return body, err
				}
				f.Old, f.New = nullToEmpty(pair[0]), nullToEmpty(pair[1])
			} else if r.Action == "I" {
				f.New = v
			} else {
				f.Old = v
			}
			// O id do próprio cadastro (party_id do contato da pessoa 42) não
			// é informação: já se sabe de quem é.
			if kind, ok := refFields[name]; ok {
				if kind == r.EntityType && (sameID(f.New, r.EntityID) || sameID(f.Old, r.EntityID)) && r.Action != "U" {
					continue
				}
				for _, v := range []json.RawMessage{f.Old, f.New} {
					if id, ok := asID(v); ok {
						want(kind, id)
					}
				}
			}
			ch.Fields = append(ch.Fields, f)
		}
		sort.Slice(ch.Fields, func(i, j int) bool { return ch.Fields[i].Name < ch.Fields[j].Name })

		n := len(body.Events)
		if n > 0 {
			last := &body.Events[n-1]
			if last.txID == r.TxID && last.EntityType == r.EntityType && last.EntityID == r.EntityID {
				last.Changes = append(last.Changes, ch)
				continue
			}
		}
		ev := AuditEvent{ID: r.ID, OccurredAt: r.OccurredAt, EntityType: r.EntityType, EntityID: r.EntityID, Changes: []AuditChange{ch}, txID: r.TxID}
		if r.ActorUserID != nil {
			ev.Actor = &AuditRef{ID: *r.ActorUserID}
			want("user", *r.ActorUserID)
		}
		want(r.EntityType, r.EntityID)
		body.Events = append(body.Events, ev)
	}

	// Dentro de cada evento, na ordem em que aconteceu.
	for i := range body.Events {
		slices.Reverse(body.Events[i].Changes)
	}
	// Página cheia: pode haver mais. O último evento pode ter ficado pela
	// metade (parte das linhas dele na próxima página): ele sai desta e volta
	// inteiro na próxima.
	if len(rows) == page {
		if len(body.Events) > 1 {
			body.Events = body.Events[:len(body.Events)-1]
			next := minRowID(rows, body.Events[len(body.Events)-1])
			body.NextBefore = &next
		} else {
			last := rows[len(rows)-1].ID
			body.NextBefore = &last
		}
	}

	names, err := lookupNames(ctx, tx, ids)
	if err != nil {
		return body, err
	}
	for i := range body.Events {
		ev := &body.Events[i]
		ev.EntityName = names[key(ev.EntityType, ev.EntityID)]
		if ev.EntityName == "" {
			ev.EntityName = deletedName(ev)
		}
		if ev.Actor != nil {
			ev.Actor.Name = names[key("user", ev.Actor.ID)]
		}
	}
	body.Refs = names
	return body, nil
}

// minRowID: o menor id de linha do evento (as linhas vêm da mais nova para a
// mais antiga, e as de um evento vêm juntas).
func minRowID(rows []db.AuditLog, ev AuditEvent) int64 {
	minID := ev.ID
	for _, r := range rows {
		if r.ID > ev.ID {
			continue
		}
		if r.TxID != ev.txID || r.EntityType != ev.EntityType || r.EntityID != ev.EntityID {
			break
		}
		minID = r.ID
	}
	return minID
}

// lookupNames busca os nomes de todos os cadastros citados, por tipo.
func lookupNames(ctx context.Context, tx postgres.Tx, ids map[string]map[int64]bool) (map[string]string, error) {
	out := map[string]string{}
	for kind, set := range ids {
		list := make([]int64, 0, len(set))
		for id := range set {
			list = append(list, id)
		}
		add := func(id int64, name string) { out[key(kind, id)] = name }
		var err error
		switch kind {
		case "user":
			err = collect(tx.AuditUserNames(ctx, list))(func(r db.AuditUserNamesRow) { add(r.ID, r.Name) })
		case "party":
			err = collect(tx.AuditPartyNames(ctx, list))(func(r db.AuditPartyNamesRow) { add(r.ID, r.Name) })
		case "product":
			err = collect(tx.AuditProductNames(ctx, list))(func(r db.AuditProductNamesRow) { add(r.ID, r.Name) })
		case "role":
			err = collect(tx.AuditRoleNames(ctx, list))(func(r db.AuditRoleNamesRow) { add(r.ID, r.Name) })
		case "driver":
			err = collect(tx.AuditDriverNames(ctx, list))(func(r db.AuditDriverNamesRow) { add(r.ID, r.Name) })
		case "vehicle":
			err = collect(tx.AuditVehicleNames(ctx, list))(func(r db.AuditVehicleNamesRow) { add(r.ID, r.Name) })
		case "company":
			err = collect(tx.AuditCompanyNames(ctx, list))(func(r db.AuditCompanyNamesRow) { add(r.ID, r.Name) })
		case "certificate":
			err = collect(tx.AuditCertificateNames(ctx, list))(func(r db.AuditCertificateNamesRow) { add(r.ID, r.Name) })
		case "city":
			err = collect(tx.AuditCityNames(ctx, list))(func(r db.AuditCityNamesRow) { add(r.ID, r.Name) })
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// collect deixa o switch acima curto: recebe (linhas, erro) de uma consulta e
// aplica fn em cada linha.
func collect[T any](rows []T, err error) func(fn func(T)) error {
	return func(fn func(T)) error {
		if err != nil {
			return err
		}
		for _, r := range rows {
			fn(r)
		}
		return nil
	}
}

// deletedName: cadastro que não existe mais (ex.: papel apagado) usa o nome
// que ficou gravado no próprio histórico.
func deletedName(ev *AuditEvent) string {
	for _, c := range ev.Changes {
		for _, f := range c.Fields {
			if f.Name == "name" || f.Name == "plate" || f.Name == "legal_name" {
				var s string
				if json.Unmarshal(f.Old, &s) == nil && s != "" {
					return s
				}
				if json.Unmarshal(f.New, &s) == nil && s != "" {
					return s
				}
			}
		}
	}
	return "#" + strconv.FormatInt(ev.EntityID, 10)
}

func key(kind string, id int64) string { return kind + ":" + strconv.FormatInt(id, 10) }

func asID(v json.RawMessage) (int64, bool) {
	var id int64
	if len(v) == 0 || json.Unmarshal(v, &id) != nil {
		return 0, false
	}
	return id, true
}

func sameID(v json.RawMessage, id int64) bool {
	got, ok := asID(v)
	return ok && got == id
}

// nullToEmpty: "null" vira ausente (a tela mostra "vazio").
func nullToEmpty(v json.RawMessage) json.RawMessage {
	if string(v) == "null" {
		return nil
	}
	return v
}

func optional(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
}
