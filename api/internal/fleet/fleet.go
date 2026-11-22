// Package fleet cuida da frota: veículos, motoristas, o mapa ao vivo e o
// trajeto de cada veículo. As posições vêm do Traccar (ver sync.go).
package fleet

import (
	"context"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RicoGama/ceasa-fruit/api/internal/auth"
	"github.com/RicoGama/ceasa-fruit/api/internal/db"
	"github.com/RicoGama/ceasa-fruit/api/internal/document"
	"github.com/RicoGama/ceasa-fruit/api/internal/postgres"
)

// OfflineAfter: sem posição nova há mais que isso, o veículo aparece como
// "sem sinal" (o rastreador manda a cada 30 s rodando e a cada poucos
// minutos parado).
const OfflineAfter = 30 * time.Minute

// MaxTrackWindow: o trajeto é pedido em janelas de no máximo 48 h.
const MaxTrackWindow = 48 * time.Hour

// Options configura o pacote.
type Options struct {
	// TraccarConfigured: há um servidor de rastreamento configurado
	// (TRACCAR_URL). Sem ele, o mapa explica como configurar.
	TraccarConfigured bool
}

type handler struct {
	db   postgres.DB
	opts Options
}

// Register adiciona as rotas da frota. api deve ser um grupo que exige login.
func Register(api huma.API, database postgres.DB, opts Options) {
	h := &handler{db: database, opts: opts}
	drivers := []string{"Fleet: drivers"}
	vehicles := []string{"Fleet: vehicles"}

	// Motoristas
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "list-drivers", Method: http.MethodGet, Path: "/drivers", Summary: "List drivers", Tags: drivers,
	}, auth.FleetView), h.listDrivers)
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "get-driver", Method: http.MethodGet, Path: "/drivers/{id}", Summary: "Get a driver", Tags: drivers,
	}, auth.FleetView), h.getDriver)
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "create-driver", Method: http.MethodPost, Path: "/drivers", Summary: "Create a driver",
		Tags: drivers, DefaultStatus: http.StatusCreated,
	}, auth.FleetEdit), h.createDriver)
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "update-driver", Method: http.MethodPut, Path: "/drivers/{id}", Summary: "Update a driver", Tags: drivers,
	}, auth.FleetEdit), h.updateDriver)
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "deactivate-driver", Method: http.MethodDelete, Path: "/drivers/{id}", Summary: "Deactivate a driver",
		Tags: drivers, DefaultStatus: http.StatusNoContent,
	}, auth.FleetEdit), h.deactivateDriver)

	// Veículos
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "list-vehicles", Method: http.MethodGet, Path: "/vehicles", Summary: "List vehicles", Tags: vehicles,
	}, auth.FleetView), h.listVehicles)
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "get-vehicle", Method: http.MethodGet, Path: "/vehicles/{id}", Summary: "Get a vehicle", Tags: vehicles,
	}, auth.FleetView), h.getVehicle)
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "create-vehicle", Method: http.MethodPost, Path: "/vehicles", Summary: "Create a vehicle",
		Tags: vehicles, DefaultStatus: http.StatusCreated,
	}, auth.FleetEdit), h.createVehicle)
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "update-vehicle", Method: http.MethodPut, Path: "/vehicles/{id}", Summary: "Update a vehicle", Tags: vehicles,
	}, auth.FleetEdit), h.updateVehicle)
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "deactivate-vehicle", Method: http.MethodDelete, Path: "/vehicles/{id}", Summary: "Deactivate a vehicle",
		Tags: vehicles, DefaultStatus: http.StatusNoContent,
	}, auth.FleetEdit), h.deactivateVehicle)

	// Mapa
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "get-fleet-live", Method: http.MethodGet, Path: "/fleet/live",
		Summary: "Live map: every active vehicle with its last position", Tags: vehicles,
	}, auth.FleetView), h.live)
	huma.Register(api, auth.Protect(api, huma.Operation{
		OperationID: "get-vehicle-track", Method: http.MethodGet, Path: "/vehicles/{id}/track",
		Summary: "Positions of a vehicle in a period (at most 48 h)", Tags: vehicles,
	}, auth.FleetView), h.track)
}

// ---------------------------------------------------------------------------
// Motoristas
// ---------------------------------------------------------------------------

// DriverFields são os campos editáveis de um motorista.
type DriverFields struct {
	Name             string  `json:"name" minLength:"2" maxLength:"100"`
	Phone            *string `json:"phone,omitempty" maxLength:"20"`
	Document         *string `json:"document,omitempty" maxLength:"14" doc:"CPF, with or without mask"`
	LicenseNumber    *string `json:"licenseNumber,omitempty" maxLength:"20" doc:"Driver's license (CNH) number"`
	LicenseCategory  *string `json:"licenseCategory,omitempty" enum:"B,C,D,E,AB,AC,AD,AE"`
	LicenseExpiresOn *string `json:"licenseExpiresOn,omitempty" format:"date" doc:"YYYY-MM-DD"`
	Active           *bool   `json:"active,omitempty" doc:"Default true"`
	Notes            *string `json:"notes,omitempty" maxLength:"1000"`

	expires *time.Time
}

func (f *DriverFields) normalize() []error {
	var errs []error
	fail := func(loc, msg string) {
		errs = append(errs, &huma.ErrorDetail{Location: "body." + loc, Message: msg})
	}
	f.Name = strings.TrimSpace(f.Name)
	if len(f.Name) < 2 {
		fail("name", "expected length >= 2")
	}
	f.Phone = digitsOrNil(f.Phone)
	if f.Phone != nil && (len(*f.Phone) < 8 || len(*f.Phone) > 13) {
		fail("phone", "invalid phone")
	}
	f.Document = digitsOrNil(f.Document)
	if f.Document != nil && (len(*f.Document) != 11 || !document.IsValid(*f.Document)) {
		fail("document", "invalid CPF")
	}
	f.LicenseNumber = trimmed(f.LicenseNumber)
	f.Notes = trimmed(f.Notes)
	if f.LicenseExpiresOn = trimmed(f.LicenseExpiresOn); f.LicenseExpiresOn != nil {
		t, err := time.Parse(time.DateOnly, *f.LicenseExpiresOn)
		if err != nil {
			fail("licenseExpiresOn", "invalid date")
		} else {
			f.expires = &t
		}
	}
	if f.Active == nil {
		yes := true
		f.Active = &yes
	}
	return errs
}

// Driver é o motorista como a API devolve.
type Driver struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Phone            *string   `json:"phone"`
	Document         *string   `json:"document"`
	LicenseNumber    *string   `json:"licenseNumber"`
	LicenseCategory  *string   `json:"licenseCategory" enum:"B,C,D,E,AB,AC,AD,AE"`
	LicenseExpiresOn *string   `json:"licenseExpiresOn" doc:"YYYY-MM-DD"`
	Active           bool      `json:"active"`
	Notes            *string   `json:"notes"`
	VehicleCount     int64     `json:"vehicleCount" doc:"Active vehicles that have this driver as the usual one"`
	Version          int32     `json:"version"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func driverFromDB(d db.Driver, vehicleCount int64) Driver {
	return Driver{
		ID: d.ID, Name: d.Name, Phone: d.Phone, Document: d.Document, LicenseNumber: d.LicenseNumber,
		LicenseCategory: d.LicenseCategory, LicenseExpiresOn: dateString(d.LicenseExpiresOn), Active: d.Active,
		Notes: d.Notes, VehicleCount: vehicleCount, Version: d.Version, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type listDriversInput struct {
	Search string `query:"search" maxLength:"100"`
	Status string `query:"status" enum:"active,inactive,all" default:"all"`
}

type listDriversOutput struct {
	Body struct {
		Items []Driver `json:"items"`
	}
}

type idInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type driverOutput struct{ Body Driver }

type createDriverInput struct{ Body DriverFields }

func (in *createDriverInput) Resolve(huma.Context) []error { return in.Body.normalize() }

// DriverUpdate são os campos + a versão que a tela leu.
type DriverUpdate struct {
	DriverFields
	Version int32 `json:"version" minimum:"1"`
}

type updateDriverInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body DriverUpdate
}

func (in *updateDriverInput) Resolve(huma.Context) []error { return in.Body.normalize() }

func (h *handler) listDrivers(ctx context.Context, in *listDriversInput) (*listDriversOutput, error) {
	var search *string
	if s := strings.TrimSpace(in.Search); s != "" {
		search = &s
	}
	active := activeFilter(in.Status)
	out := &listDriversOutput{}
	out.Body.Items = []Driver{}
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		rows, err := tx.ListDrivers(ctx, db.ListDriversParams{Search: search, Active: active})
		for _, r := range rows {
			out.Body.Items = append(out.Body.Items, driverFromDB(db.Driver{
				ID: r.ID, Name: r.Name, Phone: r.Phone, Document: r.Document, LicenseNumber: r.LicenseNumber,
				LicenseCategory: r.LicenseCategory, LicenseExpiresOn: r.LicenseExpiresOn, Active: r.Active,
				Notes: r.Notes, Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			}, r.VehicleCount))
		}
		return err
	})
	return out, err
}

func (h *handler) getDriver(ctx context.Context, in *idInput) (*driverOutput, error) {
	var out Driver
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		d, err := tx.GetDriver(ctx, in.ID)
		out = driverFromDB(d, 0)
		return err
	})
	if err != nil {
		return nil, mapError(err, "driver not found")
	}
	return &driverOutput{Body: out}, nil
}

func (h *handler) createDriver(ctx context.Context, in *createDriverInput) (*driverOutput, error) {
	f := in.Body
	var out Driver
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		d, err := tx.CreateDriver(ctx, db.CreateDriverParams{
			Name: f.Name, Phone: f.Phone, Document: f.Document, LicenseNumber: f.LicenseNumber,
			LicenseCategory: f.LicenseCategory, LicenseExpiresOn: f.expires, Active: *f.Active, Notes: f.Notes,
		})
		out = driverFromDB(d, 0)
		return err
	})
	if err != nil {
		return nil, mapError(err, "driver not found")
	}
	return &driverOutput{Body: out}, nil
}

func (h *handler) updateDriver(ctx context.Context, in *updateDriverInput) (*driverOutput, error) {
	f := in.Body
	var out Driver
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		d, err := tx.UpdateDriver(ctx, db.UpdateDriverParams{
			ID: in.ID, Version: f.Version, Name: f.Name, Phone: f.Phone, Document: f.Document,
			LicenseNumber: f.LicenseNumber, LicenseCategory: f.LicenseCategory, LicenseExpiresOn: f.expires,
			Active: *f.Active, Notes: f.Notes,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// Não existe (404) ou a versão mudou (409)?
			if _, err := tx.GetDriver(ctx, in.ID); err != nil {
				return err
			}
			return postgres.StaleError()
		}
		out = driverFromDB(d, 0)
		return err
	})
	if err != nil {
		return nil, mapError(err, "driver not found")
	}
	return &driverOutput{Body: out}, nil
}

func (h *handler) deactivateDriver(ctx context.Context, in *idInput) (*struct{}, error) {
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		if _, err := tx.GetDriver(ctx, in.ID); err != nil {
			return err
		}
		_, err := tx.DeactivateDriver(ctx, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err, "driver not found")
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Veículos
// ---------------------------------------------------------------------------

var (
	plateRe = regexp.MustCompile(`^[A-Z]{3}[0-9][A-Z0-9][0-9]{2}$`)
	imeiRe  = regexp.MustCompile(`^[0-9]{10,20}$`)
)

// VehicleFields são os campos editáveis de um veículo.
type VehicleFields struct {
	Plate       string  `json:"plate" minLength:"7" maxLength:"8" doc:"ABC1234 or ABC1D23, with or without dash"`
	Description *string `json:"description,omitempty" maxLength:"150" doc:"Brand, model, body (e.g. Volvo VM 270 box truck)"`
	Kind        string  `json:"kind,omitempty" enum:"truck,van,pickup,car,motorcycle,other" default:"truck"`
	CapacityKg  *int32  `json:"capacityKg,omitempty" minimum:"1" maximum:"100000"`
	DriverID    *int64  `json:"driverId,omitempty" minimum:"1" doc:"Usual driver"`
	TrackerImei *string `json:"trackerImei,omitempty" maxLength:"25" doc:"IMEI printed on the tracker; empty = no tracker"`
	Active      *bool   `json:"active,omitempty" doc:"Default true"`
	Notes       *string `json:"notes,omitempty" maxLength:"1000"`
}

func (f *VehicleFields) normalize() []error {
	var errs []error
	fail := func(loc, msg string, v any) {
		errs = append(errs, &huma.ErrorDetail{Location: "body." + loc, Message: msg, Value: v})
	}
	f.Plate = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(f.Plate))
	if !plateRe.MatchString(f.Plate) {
		fail("plate", "invalid plate", f.Plate)
	}
	f.Description = trimmed(f.Description)
	f.Notes = trimmed(f.Notes)
	if f.Kind == "" {
		f.Kind = "truck"
	}
	f.TrackerImei = digitsOrNil(f.TrackerImei)
	if f.TrackerImei != nil && !imeiRe.MatchString(*f.TrackerImei) {
		fail("trackerImei", "invalid IMEI", *f.TrackerImei)
	}
	if f.Active == nil {
		yes := true
		f.Active = &yes
	}
	return errs
}

// LivePosition é a última posição conhecida do veículo.
type LivePosition struct {
	FixAt     time.Time `json:"fixAt" doc:"When the GPS measured it"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	SpeedKmh  float32   `json:"speedKmh"`
	Course    int16     `json:"course" doc:"Heading in degrees (0 = north)"`
	Ignition  *bool     `json:"ignition"`
	Motion    *bool     `json:"motion"`
}

// DriverRef é o motorista resumido dentro do veículo.
type DriverRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Vehicle é o veículo como a API devolve.
type Vehicle struct {
	ID           int64         `json:"id"`
	Plate        string        `json:"plate"`
	Description  *string       `json:"description"`
	Kind         string        `json:"kind" enum:"truck,van,pickup,car,motorcycle,other"`
	CapacityKg   *int32        `json:"capacityKg"`
	Driver       *DriverRef    `json:"driver"`
	TrackerImei  *string       `json:"trackerImei"`
	Active       bool          `json:"active"`
	Notes        *string       `json:"notes"`
	LastPosition *LivePosition `json:"lastPosition"`
	// moving = andando; idle = parado com o motor ligado; stopped = parado e
	// desligado; offline = sem posição há mais de 30 min; waiting = rastreador
	// cadastrado, mas nenhuma posição ainda; no_tracker = sem rastreador.
	Status    string    `json:"status" enum:"moving,idle,stopped,offline,waiting,no_tracker"`
	Version   int32     `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type vehicleRow struct {
	ID                          int64
	Plate                       string
	Description                 *string
	Kind                        string
	CapacityKg                  *int32
	DriverID                    *int64
	DriverName                  *string
	TrackerImei                 *string
	Active                      bool
	Notes                       *string
	Version                     int32
	CreatedAt, UpdatedAt        time.Time
	LastFixAt                   *time.Time
	LastLatitude, LastLongitude *float64
	LastSpeedKmh                *float32
	LastCourse                  *int16
	LastIgnition, LastMotion    *bool
}

func vehicleFrom(r vehicleRow, now time.Time) Vehicle {
	v := Vehicle{
		ID: r.ID, Plate: r.Plate, Description: r.Description, Kind: r.Kind, CapacityKg: r.CapacityKg,
		TrackerImei: r.TrackerImei, Active: r.Active, Notes: r.Notes, Version: r.Version,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.DriverID != nil && r.DriverName != nil {
		v.Driver = &DriverRef{ID: *r.DriverID, Name: *r.DriverName}
	}
	if r.LastFixAt != nil && r.LastLatitude != nil && r.LastLongitude != nil {
		v.LastPosition = &LivePosition{
			FixAt: *r.LastFixAt, Latitude: *r.LastLatitude, Longitude: *r.LastLongitude,
			SpeedKmh: deref(r.LastSpeedKmh), Course: deref(r.LastCourse),
			Ignition: r.LastIgnition, Motion: r.LastMotion,
		}
	}
	v.Status = Status(v.TrackerImei != nil, v.LastPosition, now)
	return v
}

// Status resume a situação do veículo para o mapa.
func Status(hasTracker bool, p *LivePosition, now time.Time) string {
	switch {
	case !hasTracker:
		return "no_tracker"
	case p == nil:
		return "waiting"
	case now.Sub(p.FixAt) > OfflineAfter:
		return "offline"
	case p.SpeedKmh >= 3:
		return "moving"
	case p.Ignition != nil && *p.Ignition:
		return "idle"
	default:
		return "stopped"
	}
}

func fromVehicleDB(v db.Vehicle, driverName *string) vehicleRow {
	return vehicleRow{
		ID: v.ID, Plate: v.Plate, Description: v.Description, Kind: v.Kind, CapacityKg: v.CapacityKg,
		DriverID: v.DriverID, DriverName: driverName, TrackerImei: v.TrackerImei, Active: v.Active, Notes: v.Notes,
		Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, LastFixAt: v.LastFixAt,
		LastLatitude: v.LastLatitude, LastLongitude: v.LastLongitude, LastSpeedKmh: v.LastSpeedKmh,
		LastCourse: v.LastCourse, LastIgnition: v.LastIgnition, LastMotion: v.LastMotion,
	}
}

type listVehiclesInput struct {
	Search string `query:"search" maxLength:"100" doc:"Plate, description or driver"`
	Status string `query:"status" enum:"active,inactive,all" default:"all"`
}

type listVehiclesOutput struct {
	Body struct {
		Items []Vehicle `json:"items"`
	}
}

type vehicleOutput struct{ Body Vehicle }

type createVehicleInput struct{ Body VehicleFields }

func (in *createVehicleInput) Resolve(huma.Context) []error { return in.Body.normalize() }

// VehicleUpdate são os campos + a versão que a tela leu.
type VehicleUpdate struct {
	VehicleFields
	Version int32 `json:"version" minimum:"1"`
}

type updateVehicleInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body VehicleUpdate
}

func (in *updateVehicleInput) Resolve(huma.Context) []error { return in.Body.normalize() }

func (h *handler) listVehicles(ctx context.Context, in *listVehiclesInput) (*listVehiclesOutput, error) {
	var search *string
	if s := strings.TrimSpace(in.Search); s != "" {
		search = &s
	}
	out := &listVehiclesOutput{}
	out.Body.Items = []Vehicle{}
	now := time.Now()
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		rows, err := tx.ListVehicles(ctx, db.ListVehiclesParams{Search: search, Active: activeFilter(in.Status)})
		for _, r := range rows {
			out.Body.Items = append(out.Body.Items, vehicleFrom(fromVehicleDB(db.Vehicle{
				ID: r.ID, Plate: r.Plate, Description: r.Description, Kind: r.Kind, CapacityKg: r.CapacityKg,
				DriverID: r.DriverID, TrackerImei: r.TrackerImei, Active: r.Active, Notes: r.Notes, Version: r.Version,
				CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LastFixAt: r.LastFixAt, LastLatitude: r.LastLatitude,
				LastLongitude: r.LastLongitude, LastSpeedKmh: r.LastSpeedKmh, LastCourse: r.LastCourse,
				LastIgnition: r.LastIgnition, LastMotion: r.LastMotion,
			}, r.DriverName), now))
		}
		return err
	})
	return out, err
}

func (h *handler) loadVehicle(ctx context.Context, tx postgres.Tx, id int64) (Vehicle, error) {
	r, err := tx.GetVehicle(ctx, id)
	if err != nil {
		return Vehicle{}, err
	}
	return vehicleFrom(fromVehicleDB(db.Vehicle{
		ID: r.ID, Plate: r.Plate, Description: r.Description, Kind: r.Kind, CapacityKg: r.CapacityKg,
		DriverID: r.DriverID, TrackerImei: r.TrackerImei, Active: r.Active, Notes: r.Notes, Version: r.Version,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LastFixAt: r.LastFixAt, LastLatitude: r.LastLatitude,
		LastLongitude: r.LastLongitude, LastSpeedKmh: r.LastSpeedKmh, LastCourse: r.LastCourse,
		LastIgnition: r.LastIgnition, LastMotion: r.LastMotion,
	}, r.DriverName), time.Now()), nil
}

func (h *handler) getVehicle(ctx context.Context, in *idInput) (*vehicleOutput, error) {
	var out Vehicle
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		var err error
		out, err = h.loadVehicle(ctx, tx, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err, "vehicle not found")
	}
	return &vehicleOutput{Body: out}, nil
}

func (h *handler) createVehicle(ctx context.Context, in *createVehicleInput) (*vehicleOutput, error) {
	f := in.Body
	var out Vehicle
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		id, err := tx.CreateVehicle(ctx, db.CreateVehicleParams{
			Plate: f.Plate, Description: f.Description, Kind: f.Kind, CapacityKg: f.CapacityKg,
			DriverID: f.DriverID, TrackerImei: f.TrackerImei, Active: *f.Active, Notes: f.Notes,
		})
		if err != nil {
			return err
		}
		out, err = h.loadVehicle(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, mapError(err, "vehicle not found")
	}
	return &vehicleOutput{Body: out}, nil
}

func (h *handler) updateVehicle(ctx context.Context, in *updateVehicleInput) (*vehicleOutput, error) {
	f := in.Body
	var out Vehicle
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		n, err := tx.UpdateVehicle(ctx, db.UpdateVehicleParams{
			ID: in.ID, Version: f.Version, Plate: f.Plate, Description: f.Description, Kind: f.Kind,
			CapacityKg: f.CapacityKg, DriverID: f.DriverID, TrackerImei: f.TrackerImei, Active: *f.Active, Notes: f.Notes,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			if _, err := tx.GetVehicle(ctx, in.ID); err != nil {
				return err
			}
			return postgres.StaleError()
		}
		out, err = h.loadVehicle(ctx, tx, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err, "vehicle not found")
	}
	return &vehicleOutput{Body: out}, nil
}

func (h *handler) deactivateVehicle(ctx context.Context, in *idInput) (*struct{}, error) {
	err := h.db.InTx(ctx, func(tx postgres.Tx) error {
		if _, err := tx.GetVehicle(ctx, in.ID); err != nil {
			return err
		}
		_, err := tx.DeactivateVehicle(ctx, in.ID)
		return err
	})
	if err != nil {
		return nil, mapError(err, "vehicle not found")
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Mapa ao vivo
// ---------------------------------------------------------------------------

// SyncStatus é a situação da conexão com o servidor de rastreamento.
type SyncStatus struct {
	Configured    bool       `json:"configured" doc:"A Traccar server is configured (TRACCAR_URL)"`
	LastSuccessAt *time.Time `json:"lastSuccessAt"`
	LastError     *string    `json:"lastError" doc:"Already in Portuguese, for the screen"`
	LastErrorAt   *time.Time `json:"lastErrorAt" doc:"Since when this problem is happening"`
}

type liveOutput struct {
	Body struct {
		Sync     SyncStatus `json:"sync"`
		Vehicles []Vehicle  `json:"vehicles"`
	}
}

func (h *handler) live(ctx context.Context, _ *struct{}) (*liveOutput, error) {
	out := &liveOutput{}
	out.Body.Vehicles = []Vehicle{}
	out.Body.Sync.Configured = h.opts.TraccarConfigured
	now := time.Now()
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		st, err := tx.GetFleetSync(ctx)
		if err != nil {
			return err
		}
		out.Body.Sync.LastSuccessAt, out.Body.Sync.LastError, out.Body.Sync.LastErrorAt = st.LastSuccessAt, st.LastError, st.LastErrorAt
		rows, err := tx.ListFleetLive(ctx)
		for _, r := range rows {
			out.Body.Vehicles = append(out.Body.Vehicles, vehicleFrom(vehicleRow{
				ID: r.ID, Plate: r.Plate, Description: r.Description, Kind: r.Kind, DriverID: r.DriverID,
				DriverName: r.DriverName, TrackerImei: r.TrackerImei, Active: true, LastFixAt: r.LastFixAt,
				LastLatitude: r.LastLatitude, LastLongitude: r.LastLongitude, LastSpeedKmh: r.LastSpeedKmh,
				LastCourse: r.LastCourse, LastIgnition: r.LastIgnition, LastMotion: r.LastMotion,
			}, now))
		}
		return err
	})
	return out, err
}

// ---------------------------------------------------------------------------
// Trajeto
// ---------------------------------------------------------------------------

type trackInput struct {
	ID   int64     `path:"id" minimum:"1"`
	From time.Time `query:"from" required:"true" doc:"Start (RFC 3339)"`
	To   time.Time `query:"to" required:"true" doc:"End (RFC 3339); at most 48 h after from"`
}

func (in *trackInput) Resolve(huma.Context) []error {
	switch {
	case !in.To.After(in.From):
		return []error{&huma.ErrorDetail{Location: "query.to", Message: "end must be after start"}}
	case in.To.Sub(in.From) > MaxTrackWindow:
		return []error{&huma.ErrorDetail{Location: "query.to", Message: "period longer than 48 hours"}}
	}
	return nil
}

// TrackPoint é um ponto do trajeto (nomes curtos: podem ser milhares).
type TrackPoint struct {
	T        time.Time `json:"t"`
	Lat      float64   `json:"lat"`
	Lng      float64   `json:"lng"`
	SpeedKmh float32   `json:"speedKmh"`
	Ignition *bool     `json:"ignition"`
}

type trackOutput struct {
	Body struct {
		Points        []TrackPoint `json:"points"`
		DistanceKm    float64      `json:"distanceKm"`
		MaxSpeedKmh   float32      `json:"maxSpeedKmh"`
		MovingMinutes int          `json:"movingMinutes" doc:"Time with speed above 3 km/h"`
	}
}

func (h *handler) track(ctx context.Context, in *trackInput) (*trackOutput, error) {
	out := &trackOutput{}
	out.Body.Points = []TrackPoint{}
	err := h.db.InReadTx(ctx, func(tx postgres.Tx) error {
		if _, err := tx.GetVehicle(ctx, in.ID); err != nil {
			return err
		}
		rows, err := tx.ListVehicleTrack(ctx, db.ListVehicleTrackParams{VehicleID: in.ID, FromAt: in.From, ToAt: in.To})
		for _, r := range rows {
			out.Body.Points = append(out.Body.Points, TrackPoint{
				T: r.FixAt, Lat: r.Latitude, Lng: r.Longitude, SpeedKmh: deref(r.SpeedKmh), Ignition: r.Ignition,
			})
		}
		return err
	})
	if err != nil {
		return nil, mapError(err, "vehicle not found")
	}
	pts := out.Body.Points
	var moving time.Duration
	for i, p := range pts {
		out.Body.MaxSpeedKmh = max(out.Body.MaxSpeedKmh, p.SpeedKmh)
		if i == 0 {
			continue
		}
		prev := pts[i-1]
		out.Body.DistanceKm += haversineKm(prev.Lat, prev.Lng, p.Lat, p.Lng)
		// Tempo andando: só conta intervalos curtos (buracos longos = sem sinal).
		if gap := p.T.Sub(prev.T); prev.SpeedKmh >= 3 && gap < 10*time.Minute {
			moving += gap
		}
	}
	out.Body.DistanceKm = math.Round(out.Body.DistanceKm*10) / 10
	out.Body.MovingMinutes = int(moving.Minutes())
	return out, nil
}

// haversineKm é a distância em km entre dois pontos na superfície da Terra.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*rad, (lng2-lng1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

// ---------------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------------

func mapError(err error, notFound string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return huma.Error404NotFound(notFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "vehicles_plate_key":
			return huma.Error409Conflict("a vehicle with this plate already exists")
		case "vehicles_tracker_imei_key":
			return huma.Error409Conflict("this tracker is already in another vehicle")
		case "drivers_document_key":
			return huma.Error409Conflict("a driver with this CPF already exists")
		case "vehicles_driver_id_fkey":
			return huma.Error422UnprocessableEntity("validation failed", &huma.ErrorDetail{
				Location: "body.driverId", Message: "driver not found",
			})
		}
	}
	return err
}

func activeFilter(status string) *bool {
	switch status {
	case "active":
		v := true
		return &v
	case "inactive":
		v := false
		return &v
	}
	return nil
}

var nonDigits = regexp.MustCompile(`\D`)

func digitsOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	d := nonDigits.ReplaceAllString(*s, "")
	if d == "" {
		return nil
	}
	return &d
}

func trimmed(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
