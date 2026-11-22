package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/fleet/traccar"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// Syncer traz as posições do Traccar (na nuvem) para o banco do box.
//
// A cada rodada (15 s por padrão):
//  1. lista os aparelhos do Traccar (uma chamada só, leve);
//  2. para cada veículo com rastreador: cadastra o aparelho no Traccar se
//     ainda não existir (nome = placa) e liga o veículo a ele;
//  3. só busca posições de quem tem novidade (o "lastUpdate" do aparelho
//     mudou desde a última rodada);
//  4. busca A PARTIR DA ÚLTIMA POSIÇÃO GRAVADA, e não só "os últimos
//     minutos": sem sinal, o rastreador guarda as posições e manda depois
//     com o horário de quando mediu; com a internet do box fora, o Traccar
//     acumula. Nos dois casos, a próxima rodada boa traz tudo o que faltou.
//
// Posições repetidas são ignoradas pelo banco (UNIQUE no id do Traccar):
// buscar de novo um pedaço já gravado não duplica nada.
type Syncer struct {
	DB        postgres.DB
	Client    *traccar.Client
	Logger    *slog.Logger
	Interval  time.Duration // padrão 15 s
	Retention time.Duration // histórico guardado; 0 = para sempre
	// Settle: por quanto tempo depois de uma novidade ainda confere de novo
	// (padrão 30 s). O Traccar marca o aparelho como atualizado um instante
	// ANTES de gravar a posição; quem pergunta nesse intervalo não a vê.
	Settle time.Duration
	// Now existe para os testes controlarem o relógio.
	Now func() time.Time

	lastStatusSave time.Time
	lastError      string
	lastCleanup    time.Time
}

const (
	// Margem antes da última posição gravada: rastreadores às vezes mandam
	// fora de ordem por alguns segundos/minutos.
	lookback = 10 * time.Minute
	// Primeira busca de um veículo novo: só as últimas 24 h.
	firstSync = 24 * time.Hour
	// Pedaços de no máximo 12 h por chamada (depois de dias sem internet).
	chunk = 12 * time.Hour
)

func (s *Syncer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Run sincroniza até o ctx acabar.
func (s *Syncer) Run(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	s.Logger.Info("fleet: syncing with Traccar", "every", interval)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := s.SyncOnce(ctx); err != nil && ctx.Err() == nil {
			s.Logger.Warn("fleet: sync failed", "error", err)
		}
		s.cleanup(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// SyncOnce faz uma rodada completa. Devolve o primeiro erro (e grava a
// situação em fleet_sync para o mapa mostrar).
func (s *Syncer) SyncOnce(ctx context.Context) error {
	started := s.now()
	err := s.syncOnce(ctx)
	s.saveStatus(ctx, started, err)
	return err
}

func (s *Syncer) syncOnce(ctx context.Context) error {
	devices, err := s.Client.Devices(ctx)
	if err != nil {
		return err
	}
	byIMEI := make(map[string]traccar.Device, len(devices))
	for _, d := range devices {
		byIMEI[d.UniqueID] = d
	}

	var vehicles []db.ListTrackedVehiclesForSyncRow
	err = s.DB.InReadTx(ctx, func(tx postgres.Tx) error {
		vehicles, err = tx.ListTrackedVehiclesForSync(ctx)
		return err
	})
	if err != nil {
		return err
	}

	var firstErr error
	for _, v := range vehicles {
		if err := s.syncVehicle(ctx, v, byIMEI); err != nil {
			s.Logger.Warn("fleet: vehicle sync failed", "plate", v.Plate, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *Syncer) syncVehicle(ctx context.Context, v db.ListTrackedVehiclesForSyncRow, byIMEI map[string]traccar.Device) error {
	imei := *v.TrackerImei
	dev, ok := byIMEI[imei]
	switch {
	case !ok:
		// Veículo novo: cadastra o aparelho no Traccar. A posição só vem
		// quando o rastreador estiver configurado para mandar para lá.
		created, err := s.Client.CreateDevice(ctx, v.Plate, imei)
		if err != nil {
			return err
		}
		s.Logger.Info("fleet: device created in Traccar", "plate", v.Plate, "imei", imei)
		dev = created
	case dev.Name != v.Plate:
		if err := s.Client.RenameDevice(ctx, dev, v.Plate); err != nil {
			return err
		}
	}
	if v.TraccarDeviceID == nil || *v.TraccarDeviceID != dev.ID {
		if err := s.DB.InTx(ctx, func(tx postgres.Tx) error {
			return tx.SetVehicleTraccarDevice(ctx, db.SetVehicleTraccarDeviceParams{ID: v.ID, TraccarDeviceID: &dev.ID})
		}); err != nil {
			return err
		}
	}

	// Nada novo desde a última rodada: não gasta internet. (O Postgres guarda
	// microssegundos: compara na mesma precisão.)
	if dev.LastUpdate == nil {
		return nil
	}
	lastUpdate := dev.LastUpdate.Truncate(time.Microsecond)
	if v.TraccarLastUpdate != nil && !lastUpdate.After(*v.TraccarLastUpdate) {
		return nil
	}

	now := s.now()
	from := now.Add(-firstSync)
	if v.LastFixAt != nil {
		from = v.LastFixAt.Add(-lookback)
	}
	to := now.Add(5 * time.Minute) // relógio do rastreador um pouco adiantado
	for start := from; start.Before(to); start = start.Add(chunk) {
		end := start.Add(chunk)
		if end.After(to) {
			end = to
		}
		if end.Before(to) {
			end = end.Add(time.Second) // sobreposição: nada cai entre dois pedaços
		}
		positions, err := s.Client.Positions(ctx, dev.ID, start, end)
		if err != nil {
			return err
		}
		if err := s.store(ctx, v.ID, positions); err != nil {
			return err
		}
	}
	// Só marca como sincronizado depois de gravar tudo: se cair no meio, a
	// próxima rodada busca de novo (e o UNIQUE evita duplicar). Novidade
	// muito recente fica marcada um pouco antes, para a rodada seguinte
	// conferir de novo (ver Settle).
	settle := s.Settle
	if settle <= 0 {
		settle = 30 * time.Second
	}
	mark := lastUpdate
	if limit := now.Add(-settle).Truncate(time.Microsecond); mark.After(limit) {
		mark = limit
	}
	return s.DB.InTx(ctx, func(tx postgres.Tx) error {
		return tx.SetVehicleTraccarLastUpdate(ctx, db.SetVehicleTraccarLastUpdateParams{ID: v.ID, TraccarLastUpdate: &mark})
	})
}

// store grava as posições numa transação e atualiza a última posição do veículo.
func (s *Syncer) store(ctx context.Context, vehicleID int64, positions []traccar.Position) error {
	if len(positions) == 0 {
		return nil
	}
	return s.DB.InTx(ctx, func(tx postgres.Tx) error {
		var latest *traccar.Position
		for i := range positions {
			p := &positions[i]
			attrs, _ := json.Marshal(p.Attributes)
			speed := float32(p.SpeedKmh())
			course := int16(p.Course)
			alt := float32(p.Altitude)
			server := p.ServerTime
			valid := p.Valid
			if _, err := tx.InsertVehiclePosition(ctx, db.InsertVehiclePositionParams{
				VehicleID: vehicleID, TraccarPositionID: p.ID, FixAt: p.FixTime, ServerAt: &server,
				Latitude: p.Latitude, Longitude: p.Longitude, SpeedKmh: &speed, Course: &course,
				Altitude: &alt, Ignition: p.Bool("ignition"), Motion: p.Bool("motion"), Valid: &valid,
				Attributes: attrs,
			}); err != nil {
				return err
			}
			// Posição sem GPS válido não vira "a última posição" do mapa.
			if p.Valid && (latest == nil || p.FixTime.After(latest.FixTime)) {
				latest = p
			}
		}
		if latest == nil {
			return nil
		}
		speed := float32(latest.SpeedKmh())
		course := int16(latest.Course)
		return tx.UpdateVehicleLastPosition(ctx, db.UpdateVehicleLastPositionParams{
			ID: vehicleID, FixAt: &latest.FixTime, Latitude: &latest.Latitude, Longitude: &latest.Longitude,
			SpeedKmh: &speed, Course: &course, Ignition: latest.Bool("ignition"), Motion: latest.Bool("motion"),
		})
	})
}

// saveStatus grava a situação em fleet_sync. Para não avisar as telas a
// cada 15 s, só grava quando a situação muda ou a cada minuto.
func (s *Syncer) saveStatus(ctx context.Context, at time.Time, syncErr error) {
	msg := FriendlyError(syncErr)
	if msg == s.lastError && at.Sub(s.lastStatusSave) < time.Minute {
		return
	}
	err := s.DB.InTx(ctx, func(tx postgres.Tx) error {
		cur, err := tx.GetFleetSync(ctx)
		if err != nil {
			return err
		}
		p := db.SaveFleetSyncParams{
			LastAttemptAt: &at, LastSuccessAt: cur.LastSuccessAt,
			LastError: cur.LastError, LastErrorAt: cur.LastErrorAt,
		}
		if syncErr == nil {
			p.LastSuccessAt = &at
			p.LastError, p.LastErrorAt = nil, nil
		} else {
			p.LastError = &msg
			if cur.LastError == nil || *cur.LastError != msg {
				p.LastErrorAt = &at // desde quando está com este problema
			}
		}
		return tx.SaveFleetSync(ctx, p)
	})
	if err != nil {
		s.Logger.Warn("fleet: could not save sync status", "error", err)
		return
	}
	s.lastStatusSave, s.lastError = at, msg
}

// cleanup apaga as posições mais velhas que Retention, uma vez por dia.
func (s *Syncer) cleanup(ctx context.Context) {
	if s.Retention <= 0 || s.now().Sub(s.lastCleanup) < 24*time.Hour {
		return
	}
	s.lastCleanup = s.now()
	var n int64
	err := s.DB.InTx(ctx, func(tx postgres.Tx) error {
		var err error
		n, err = tx.DeleteOldVehiclePositions(ctx, s.now().Add(-s.Retention))
		return err
	})
	if err != nil {
		s.Logger.Warn("fleet: cleanup failed", "error", err)
		return
	}
	if n > 0 {
		s.Logger.Info("fleet: old positions deleted", "rows", n)
	}
}

// FriendlyError traduz o erro da sincronização para quem olha o mapa.
func FriendlyError(err error) string {
	if err == nil {
		return ""
	}
	var te *traccar.Error
	var ne net.Error
	var dnsErr *net.DNSError
	switch {
	case errors.As(err, &te) && (te.Status == http.StatusUnauthorized || te.Status == http.StatusForbidden):
		return "O servidor de rastreamento recusou o acesso: confira o token (TRACCAR_TOKEN)."
	case errors.As(err, &te):
		return "O servidor de rastreamento respondeu com erro (HTTP " + strconv.Itoa(te.Status) + ")."
	case errors.As(err, &dnsErr), errors.As(err, &ne), errors.Is(err, context.DeadlineExceeded):
		return "Sem conexão com o servidor de rastreamento (a internet do box pode estar fora)."
	default:
		return "Não foi possível sincronizar com o servidor de rastreamento."
	}
}
