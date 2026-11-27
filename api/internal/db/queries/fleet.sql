-- Frota: motoristas, veículos, posições e a sincronização com o Traccar.

-- ---------------------------------------------------------------------------
-- Motoristas
-- ---------------------------------------------------------------------------

-- name: ListDrivers :many
SELECT d.*,
       (SELECT count(*) FROM vehicles v WHERE v.driver_id = d.id AND v.active) AS vehicle_count
FROM drivers d
WHERE (sqlc.narg('search')::text IS NULL OR unaccent(d.name) ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR d.active = sqlc.narg('active'))
ORDER BY d.active DESC, lower(d.name), d.id;

-- name: GetDriver :one
SELECT * FROM drivers WHERE id = @id;

-- name: CreateDriver :one
INSERT INTO drivers (name, phone, document, license_number, license_category, license_expires_on, active, notes)
VALUES (@name, @phone, @document, @license_number, @license_category, @license_expires_on, @active, @notes)
RETURNING *;

-- Só grava se a versão for a que a tela leu (0 linhas = alterado por outra pessoa).
-- name: UpdateDriver :one
UPDATE drivers
SET name = @name, phone = @phone, document = @document, license_number = @license_number,
    license_category = @license_category, license_expires_on = @license_expires_on,
    active = @active, notes = @notes, version = version + 1, updated_at = now()
WHERE id = @id AND version = @version
RETURNING *;

-- name: DeactivateDriver :execrows
UPDATE drivers SET active = false, version = version + 1, updated_at = now()
WHERE id = @id AND active;

-- ---------------------------------------------------------------------------
-- Veículos
-- ---------------------------------------------------------------------------

-- name: ListVehicles :many
SELECT v.*, d.name AS driver_name
FROM vehicles v
LEFT JOIN drivers d ON d.id = v.driver_id
WHERE (sqlc.narg('search')::text IS NULL
       OR v.plate ILIKE '%' || like_escape(sqlc.narg('search')) || '%'
       OR unaccent(coalesce(v.description, '')) ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%'
       OR unaccent(coalesce(d.name, '')) ILIKE '%' || like_escape(unaccent(sqlc.narg('search'))) || '%')
  AND (sqlc.narg('active')::boolean IS NULL OR v.active = sqlc.narg('active'))
ORDER BY v.active DESC, v.plate;

-- name: GetVehicle :one
SELECT v.*, d.name AS driver_name
FROM vehicles v
LEFT JOIN drivers d ON d.id = v.driver_id
WHERE v.id = @id;

-- name: CreateVehicle :one
INSERT INTO vehicles (plate, description, kind, capacity_kg, driver_id, tracker_imei, active, notes)
VALUES (@plate, @description, @kind, @capacity_kg, @driver_id, @tracker_imei, @active, @notes)
RETURNING id;

-- Trocar o rastreador zera o vínculo com o Traccar: a sincronização acha o
-- aparelho novo pelo IMEI.
-- name: UpdateVehicle :execrows
UPDATE vehicles
SET plate = @plate, description = @description, kind = @kind, capacity_kg = @capacity_kg,
    driver_id = @driver_id, active = @active, notes = @notes,
    traccar_device_id = CASE WHEN tracker_imei IS DISTINCT FROM @tracker_imei THEN NULL ELSE traccar_device_id END,
    traccar_last_update = CASE WHEN tracker_imei IS DISTINCT FROM @tracker_imei THEN NULL ELSE traccar_last_update END,
    tracker_imei = @tracker_imei,
    version = version + 1, updated_at = now()
WHERE id = @id AND version = @version;

-- name: DeactivateVehicle :execrows
UPDATE vehicles SET active = false, version = version + 1, updated_at = now()
WHERE id = @id AND active;

-- ---------------------------------------------------------------------------
-- Sincronização com o Traccar (internal/fleet/sync.go)
-- ---------------------------------------------------------------------------

-- Veículos com rastreador (só uma sincronização roda por vez: a da API).
-- name: ListTrackedVehiclesForSync :many
SELECT id, plate, tracker_imei, traccar_device_id, traccar_last_update, last_fix_at
FROM vehicles
WHERE active AND tracker_imei IS NOT NULL
ORDER BY id;

-- name: SetVehicleTraccarDevice :exec
UPDATE vehicles SET traccar_device_id = @traccar_device_id WHERE id = @id;

-- name: InsertVehiclePosition :execrows
INSERT INTO vehicle_positions (
    vehicle_id, traccar_position_id, fix_at, server_at, latitude, longitude,
    speed_kmh, course, altitude, ignition, motion, valid, attributes
) VALUES (
    @vehicle_id, @traccar_position_id, @fix_at, @server_at, @latitude, @longitude,
    @speed_kmh, @course, @altitude, @ignition, @motion, @valid, @attributes
)
ON CONFLICT (vehicle_id, traccar_position_id) DO NOTHING;

-- Atualiza a última posição só se a nova for mais recente (posições guardadas
-- sem sinal chegam depois, mais antigas).
-- name: UpdateVehicleLastPosition :exec
UPDATE vehicles
SET last_fix_at = @fix_at, last_latitude = @latitude, last_longitude = @longitude,
    last_speed_kmh = @speed_kmh, last_course = @course, last_ignition = @ignition, last_motion = @motion
WHERE id = @id AND (last_fix_at IS NULL OR last_fix_at < @fix_at);

-- name: SetVehicleTraccarLastUpdate :exec
UPDATE vehicles SET traccar_last_update = @traccar_last_update WHERE id = @id;

-- name: GetFleetSync :one
SELECT * FROM fleet_sync WHERE id = 1;

-- name: SaveFleetSync :exec
UPDATE fleet_sync
SET last_attempt_at = @last_attempt_at, last_success_at = @last_success_at,
    last_error = @last_error, last_error_at = @last_error_at
WHERE id = 1;

-- Limpeza do histórico antigo (FLEET_RETENTION_DAYS).
-- name: DeleteOldVehiclePositions :execrows
DELETE FROM vehicle_positions WHERE fix_at < @before;

-- ---------------------------------------------------------------------------
-- Mapa e trajeto
-- ---------------------------------------------------------------------------

-- name: ListFleetLive :many
SELECT v.id, v.plate, v.description, v.kind, v.tracker_imei, v.driver_id, d.name AS driver_name,
       v.last_fix_at, v.last_latitude, v.last_longitude, v.last_speed_kmh, v.last_course,
       v.last_ignition, v.last_motion
FROM vehicles v
LEFT JOIN drivers d ON d.id = v.driver_id
WHERE v.active
ORDER BY v.plate;

-- name: ListVehicleTrack :many
SELECT fix_at, latitude, longitude, speed_kmh, course, ignition
FROM vehicle_positions
WHERE vehicle_id = @vehicle_id AND fix_at >= @from_at AND fix_at < @to_at AND coalesce(valid, true)
ORDER BY fix_at, id
LIMIT 20000;
