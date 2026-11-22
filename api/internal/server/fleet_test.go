package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/RicoGama/ceasa-fruit/api/internal/fleet"
	"github.com/RicoGama/ceasa-fruit/api/internal/fleet/traccar"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// fakeTraccar imita a API do Traccar o suficiente para a sincronização.
type fakeTraccar struct {
	mu             sync.Mutex
	devices        []map[string]any
	positions      map[int64][]traccar.Position // por deviceId
	nextPos        int64
	down           bool // simula a internet do box fora
	positionsCalls int
}

func newFakeTraccar(t *testing.T) (*fakeTraccar, *httptest.Server) {
	f := &fakeTraccar{positions: map[int64][]traccar.Position{}, nextPos: 100}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.down {
			// Fecha a conexão sem responder: erro de rede, como sem internet.
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		if r.Header.Get("Authorization") != "Bearer token-certo" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/devices":
			_ = json.NewEncoder(w).Encode(f.devices)
		case r.Method == http.MethodPost && r.URL.Path == "/api/devices":
			var d map[string]any
			_ = json.NewDecoder(r.Body).Decode(&d)
			d["id"] = float64(len(f.devices) + 1)
			d["lastUpdate"] = nil
			f.devices = append(f.devices, d)
			_ = json.NewEncoder(w).Encode(d)
		case r.Method == http.MethodPut:
			var d map[string]any
			_ = json.NewDecoder(r.Body).Decode(&d)
			for i := range f.devices {
				if f.devices[i]["id"] == d["id"] {
					f.devices[i]["name"] = d["name"]
				}
			}
			_ = json.NewEncoder(w).Encode(d)
		case r.Method == http.MethodGet && r.URL.Path == "/api/positions":
			f.positionsCalls++
			id, _ := strconv.ParseInt(r.URL.Query().Get("deviceId"), 10, 64)
			from, _ := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
			to, _ := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
			out := []traccar.Position{}
			for _, p := range f.positions[id] {
				if !p.FixTime.Before(from) && !p.FixTime.After(to) {
					out = append(out, p)
				}
			}
			_ = json.NewEncoder(w).Encode(out)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

// send simula o rastreador mandando posições (a "chegada" no Traccar é agora).
func (f *fakeTraccar) send(imei string, fixes ...traccar.Position) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.devices {
		if d["uniqueId"] != imei {
			continue
		}
		id := int64(d["id"].(float64))
		for _, p := range fixes {
			f.nextPos++
			p.ID, p.DeviceID, p.ServerTime, p.Valid = f.nextPos, id, time.Now(), true
			f.positions[id] = append(f.positions[id], p)
		}
		d["lastUpdate"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
}

func pos(fix time.Time, lat, lng, knots float64, ignition bool) traccar.Position {
	return traccar.Position{FixTime: fix, Latitude: lat, Longitude: lng, Speed: knots,
		Attributes: map[string]any{"ignition": ignition, "motion": knots > 0}}
}

type liveResp struct {
	Sync struct {
		Configured    bool       `json:"configured"`
		LastSuccessAt *time.Time `json:"lastSuccessAt"`
		LastError     *string    `json:"lastError"`
	} `json:"sync"`
	Vehicles []struct {
		ID           int64  `json:"id"`
		Plate        string `json:"plate"`
		Status       string `json:"status"`
		LastPosition *struct {
			FixAt    time.Time `json:"fixAt"`
			SpeedKmh float64   `json:"speedKmh"`
			Ignition *bool     `json:"ignition"`
		} `json:"lastPosition"`
		Driver *struct {
			Name string `json:"name"`
		} `json:"driver"`
	} `json:"vehicles"`
}

func TestIntegrationFleet(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	ctx := context.Background()
	if _, err := env.pool.Exec(ctx, "TRUNCATE drivers, vehicles RESTART IDENTITY CASCADE; UPDATE fleet_sync SET last_attempt_at = NULL, last_success_at = NULL, last_error = NULL, last_error_at = NULL"); err != nil {
		t.Fatal(err)
	}
	createAdmin(t, env.pool, "admin@test.com", "admin-password")
	admin := newClient(t, env.srv.URL)
	admin.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "admin@test.com", "password": "admin-password"}, nil)

	// --- Cadastros -------------------------------------------------------------------
	var driver struct {
		ID               int64   `json:"id"`
		Document         *string `json:"document"`
		LicenseExpiresOn *string `json:"licenseExpiresOn"`
		Version          int32   `json:"version"`
	}
	admin.expect(http.StatusCreated, "POST", "/drivers", map[string]any{
		"name": " Jorge Motorista ", "document": "529.982.247-25", "licenseCategory": "D", "licenseExpiresOn": "2027-05-10",
	}, &driver)
	if *driver.Document != "52998224725" || *driver.LicenseExpiresOn != "2027-05-10" {
		t.Fatalf("driver: %+v", driver)
	}
	admin.expect(http.StatusConflict, "POST", "/drivers", map[string]any{"name": "Outro", "document": "52998224725"}, nil)
	admin.expect(http.StatusUnprocessableEntity, "POST", "/drivers", map[string]any{"name": "Outro", "document": "11111111111"}, nil)

	var truck struct {
		ID          int64   `json:"id"`
		Plate       string  `json:"plate"`
		TrackerImei *string `json:"trackerImei"`
		Status      string  `json:"status"`
		Version     int32   `json:"version"`
	}
	admin.expect(http.StatusCreated, "POST", "/vehicles", map[string]any{
		"plate": "kxt-1a23", "description": "Volvo VM 270 baú", "capacityKg": 8000,
		"driverId": driver.ID, "trackerImei": "8642 7507 9000 001",
	}, &truck)
	if truck.Plate != "KXT1A23" || *truck.TrackerImei != "864275079000001" || truck.Status != "waiting" {
		t.Fatalf("vehicle: %+v", truck)
	}
	admin.expect(http.StatusCreated, "POST", "/vehicles", map[string]any{"plate": "ABC1234", "kind": "van"}, nil) // sem rastreador
	admin.expect(http.StatusConflict, "POST", "/vehicles", map[string]any{"plate": "KXT1A23"}, nil)
	admin.expect(http.StatusConflict, "POST", "/vehicles", map[string]any{"plate": "XYZ9876", "trackerImei": "864275079000001"}, nil)
	admin.expect(http.StatusUnprocessableEntity, "POST", "/vehicles", map[string]any{"plate": "12345"}, nil)
	admin.expect(http.StatusUnprocessableEntity, "POST", "/vehicles", map[string]any{"plate": "XYZ9876", "driverId": 999}, nil)
	admin.expect(http.StatusConflict, "PUT", fmt.Sprintf("/vehicles/%d", truck.ID), map[string]any{"plate": "KXT1A23", "version": 99}, nil)

	// --- Sincronização ------------------------------------------------------------------
	fake, srv := newFakeTraccar(t)
	syncer := &fleet.Syncer{
		DB: postgres.NewDB(env.pool), Client: traccar.New(srv.URL, "token-certo"),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Settle: time.Nanosecond, // o falso grava na hora; o caso do atraso tem teste próprio
	}
	if err := syncer.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fake.devices) != 1 || fake.devices[0]["name"] != "KXT1A23" || fake.devices[0]["uniqueId"] != "864275079000001" {
		t.Fatalf("device not created in Traccar: %+v", fake.devices)
	}

	now := time.Now().UTC().Truncate(time.Second)
	fake.send("864275079000001",
		pos(now.Add(-2*time.Minute), -22.8277, -43.3440, 0, false),
		pos(now.Add(-1*time.Minute), -22.8290, -43.3460, 20, true), // 20 nós = 37 km/h
	)
	if err := syncer.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var live liveResp
	admin.expect(http.StatusOK, "GET", "/fleet/live", nil, &live)
	v := live.Vehicles[1] // KXT1A23 (ordem por placa: ABC1234 primeiro)
	if v.Plate != "KXT1A23" || v.Status != "moving" || v.LastPosition == nil ||
		int(v.LastPosition.SpeedKmh) != 37 || v.Driver == nil || v.Driver.Name != "Jorge Motorista" {
		t.Fatalf("live after sync: %+v", v)
	}
	if live.Vehicles[0].Status != "no_tracker" || live.Sync.LastSuccessAt == nil || live.Sync.LastError != nil {
		t.Fatalf("live: %+v", live)
	}

	// Sem novidade no Traccar: não pede posições de novo.
	calls := fake.positionsCalls
	if err := syncer.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.positionsCalls != calls {
		t.Fatalf("asked for positions without news: %d -> %d", calls, fake.positionsCalls)
	}

	// Sem sinal: o rastreador guardou posições e manda tudo depois. Uma é
	// MAIS ANTIGA que a última gravada (entre as duas que já temos).
	fake.send("864275079000001",
		pos(now.Add(-90*time.Second), -22.8285, -43.3450, 10, true),
		pos(now.Add(-10*time.Second), -22.8320, -43.3500, 0, true), // parado, motor ligado
	)
	if err := syncer.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	_ = env.pool.QueryRow(ctx, "SELECT count(*) FROM vehicle_positions").Scan(&count)
	if count != 4 {
		t.Fatalf("positions stored: %d, want 4 (no duplicates, late one included)", count)
	}
	admin.expect(http.StatusOK, "GET", "/fleet/live", nil, &live)
	if s := live.Vehicles[1].Status; s != "idle" {
		t.Fatalf("status after late positions: %s, want idle (stopped with engine on)", s)
	}

	// Internet do box fora: a situação aparece no mapa, em português.
	fake.mu.Lock()
	fake.down = true
	fake.mu.Unlock()
	if err := syncer.SyncOnce(ctx); err == nil {
		t.Fatal("expected error with Traccar down")
	}
	admin.expect(http.StatusOK, "GET", "/fleet/live", nil, &live)
	if live.Sync.LastError == nil || *live.Sync.LastError != "Sem conexão com o servidor de rastreamento (a internet do box pode estar fora)." {
		t.Fatalf("sync error shown: %v", live.Sync.LastError)
	}
	// Enquanto estava fora, o caminhão andou; volta a internet e chega tudo.
	fake.mu.Lock()
	fake.down = false
	fake.mu.Unlock()
	fake.send("864275079000001", pos(now.Add(5*time.Second), -22.8400, -43.3600, 30, true))
	if err := syncer.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	admin.expect(http.StatusOK, "GET", "/fleet/live", nil, &live)
	if live.Sync.LastError != nil || live.Vehicles[1].Status != "moving" {
		t.Fatalf("after internet back: %+v", live)
	}

	// Token errado: mensagem própria.
	bad := &fleet.Syncer{DB: postgres.NewDB(env.pool), Client: traccar.New(srv.URL, "token-errado"), Logger: syncer.Logger}
	if err := bad.SyncOnce(ctx); fleet.FriendlyError(err) != "O servidor de rastreamento recusou o acesso: confira o token (TRACCAR_TOKEN)." {
		t.Fatalf("bad token: %v -> %q", err, fleet.FriendlyError(err))
	}

	// --- Trajeto ------------------------------------------------------------------------
	var track struct {
		Points []struct {
			T time.Time `json:"t"`
		} `json:"points"`
		DistanceKm  float64 `json:"distanceKm"`
		MaxSpeedKmh float64 `json:"maxSpeedKmh"`
	}
	q := fmt.Sprintf("/vehicles/%d/track?from=%s&to=%s", truck.ID,
		now.Add(-time.Hour).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339))
	admin.expect(http.StatusOK, "GET", q, nil, &track)
	if len(track.Points) != 5 || !track.Points[1].T.Before(track.Points[2].T) || track.DistanceKm < 2 || int(track.MaxSpeedKmh) != 55 {
		t.Fatalf("track: %d points, %.1f km, max %.0f", len(track.Points), track.DistanceKm, track.MaxSpeedKmh)
	}
	admin.expect(http.StatusUnprocessableEntity, "GET", fmt.Sprintf("/vehicles/%d/track?from=%s&to=%s", truck.ID,
		now.Add(-72*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339)), nil, nil)

	// --- Trocar o rastreador desfaz o vínculo com o aparelho antigo ---------------------
	admin.expect(http.StatusOK, "GET", fmt.Sprintf("/vehicles/%d", truck.ID), nil, &truck)
	admin.expect(http.StatusOK, "PUT", fmt.Sprintf("/vehicles/%d", truck.ID), map[string]any{
		"plate": "KXT1A23", "trackerImei": "864275079000002", "driverId": driver.ID, "version": truck.Version,
	}, &truck)
	var linked *int64
	_ = env.pool.QueryRow(ctx, "SELECT traccar_device_id FROM vehicles WHERE id = $1", truck.ID).Scan(&linked)
	if linked != nil {
		t.Fatalf("device link kept after changing the tracker")
	}
	if err := syncer.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fake.devices) != 2 {
		t.Fatalf("new tracker not created in Traccar: %d devices", len(fake.devices))
	}

	// --- Permissões ------------------------------------------------------------------------
	admin.expect(http.StatusCreated, "POST", "/users", map[string]any{
		"name": "Vendedor", "email": "vend@test.com", "roleId": roleID(t, env.pool, "seller"), "password": "senha-vend-123",
	}, nil)
	seller := newClient(t, env.srv.URL)
	seller.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "vend@test.com", "password": "senha-vend-123"}, nil)
	seller.expect(http.StatusForbidden, "GET", "/fleet/live", nil, nil)
	seller.expect(http.StatusForbidden, "POST", "/vehicles", map[string]any{"plate": "AAA1111"}, nil)

	admin.expect(http.StatusCreated, "POST", "/users", map[string]any{
		"name": "Gerente", "email": "ger@test.com", "roleId": roleID(t, env.pool, "manager"), "password": "senha-ger-123",
	}, nil)
	manager := newClient(t, env.srv.URL)
	manager.expect(http.StatusOK, "POST", "/auth/login", map[string]any{"email": "ger@test.com", "password": "senha-ger-123"}, nil)
	manager.expect(http.StatusOK, "GET", "/fleet/live", nil, nil)
	manager.expect(http.StatusNoContent, "DELETE", fmt.Sprintf("/drivers/%d", driver.ID), nil, nil)
}

// O Traccar avisa a novidade (lastUpdate) um instante antes de a posição
// aparecer na consulta: a sincronização precisa conferir de novo.
func TestIntegrationFleetLatePositionCommit(t *testing.T) {
	env := setupIntegrationEnv(t)
	resetDatabase(t, env.pool)
	ctx := context.Background()
	if _, err := env.pool.Exec(ctx, "TRUNCATE drivers, vehicles RESTART IDENTITY CASCADE; INSERT INTO vehicles (plate, tracker_imei) VALUES ('KXT1A23', '864275079000001')"); err != nil {
		t.Fatal(err)
	}
	fake, srv := newFakeTraccar(t)
	syncer := &fleet.Syncer{DB: postgres.NewDB(env.pool), Client: traccar.New(srv.URL, "token-certo"),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := syncer.SyncOnce(ctx); err != nil { // cria o aparelho
		t.Fatal(err)
	}
	now := time.Now().UTC()
	fake.send("864275079000001", pos(now.Add(-time.Minute), -22.82, -43.34, 10, true))
	// A novidade já aparece no aparelho, mas a posição ainda "não foi gravada".
	fake.mu.Lock()
	hidden := fake.positions[1]
	fake.positions[1] = nil
	fake.mu.Unlock()
	if err := syncer.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.positions[1] = hidden // agora gravou
	fake.mu.Unlock()
	if err := syncer.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = env.pool.QueryRow(ctx, "SELECT count(*) FROM vehicle_positions").Scan(&n)
	if n != 1 {
		t.Fatalf("position committed late was lost: %d stored", n)
	}
}

func TestFleetStatus(t *testing.T) {
	now := time.Now()
	on, off := true, false
	cases := []struct {
		tracker bool
		p       *fleet.LivePosition
		want    string
	}{
		{false, nil, "no_tracker"},
		{true, nil, "waiting"},
		{true, &fleet.LivePosition{FixAt: now.Add(-time.Hour), SpeedKmh: 50}, "offline"},
		{true, &fleet.LivePosition{FixAt: now, SpeedKmh: 50}, "moving"},
		{true, &fleet.LivePosition{FixAt: now, SpeedKmh: 0, Ignition: &on}, "idle"},
		{true, &fleet.LivePosition{FixAt: now, SpeedKmh: 0, Ignition: &off}, "stopped"},
		{true, &fleet.LivePosition{FixAt: now, SpeedKmh: 1}, "stopped"},
	}
	for _, c := range cases {
		if got := fleet.Status(c.tracker, c.p, now); got != c.want {
			t.Errorf("Status(%v, %+v) = %s, want %s", c.tracker, c.p, got, c.want)
		}
	}
}
