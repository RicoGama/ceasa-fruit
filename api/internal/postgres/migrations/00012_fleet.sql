-- Frota: veículos, motoristas e as posições que vêm do rastreador.
--
-- Caminho de uma posição:
--   rastreador no caminhão (chip de celular)
--     -> Traccar, num servidor na nuvem (recebe 24h, mesmo com a internet do box fora)
--     -> a API do box busca as posições novas (internal/fleet/sync.go)
--     -> vehicle_positions + a última posição copiada em vehicles
--     -> tempo real -> mapa
--
-- Ficam para depois (dependem da reunião): rotas de entrega, pedidos no
-- caminhão, custos (combustível, manutenção).

-- +goose Up
CREATE TABLE drivers (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name               TEXT        NOT NULL,
    phone              TEXT,
    document           VARCHAR(11) CHECK (document ~ '^[0-9]{11}$'),            -- CPF
    license_number     TEXT,                                                    -- número da CNH
    license_category   TEXT        CHECK (license_category IN ('B', 'C', 'D', 'E', 'AB', 'AC', 'AD', 'AE')),
    license_expires_on DATE,
    active             BOOLEAN     NOT NULL DEFAULT TRUE,
    notes              TEXT,
    version            INTEGER     NOT NULL DEFAULT 1,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX drivers_document_key ON drivers (document) WHERE document IS NOT NULL;

CREATE TABLE vehicles (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Placa sem traço, nos dois formatos: ABC1234 (antiga) e ABC1D23 (Mercosul).
    plate          VARCHAR(7)  NOT NULL UNIQUE CHECK (plate ~ '^[A-Z]{3}[0-9][A-Z0-9][0-9]{2}$'),
    description    TEXT,                       -- "Volvo VM 270 baú"
    kind           TEXT        NOT NULL DEFAULT 'truck'
                   CHECK (kind IN ('truck', 'van', 'pickup', 'car', 'motorcycle', 'other')),
    capacity_kg    INTEGER     CHECK (capacity_kg > 0),
    driver_id      BIGINT      REFERENCES drivers (id),   -- motorista habitual
    -- IMEI do rastreador (vem na etiqueta do aparelho). Vazio = sem rastreador.
    tracker_imei   VARCHAR(20) UNIQUE CHECK (tracker_imei ~ '^[0-9]{10,20}$'),
    active         BOOLEAN     NOT NULL DEFAULT TRUE,
    notes          TEXT,
    version        INTEGER     NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Sincronização com o Traccar (preenchido pela API, nunca pela tela).
    traccar_device_id   BIGINT,
    traccar_last_update TIMESTAMPTZ,  -- última vez que o rastreador falou com o Traccar

    -- Última posição, copiada aqui para o mapa ao vivo não varrer o histórico.
    -- Não mexe em "version": a sincronização não atrapalha quem edita o cadastro.
    last_fix_at    TIMESTAMPTZ,
    last_latitude  DOUBLE PRECISION,
    last_longitude DOUBLE PRECISION,
    last_speed_kmh REAL,
    last_course    SMALLINT,
    last_ignition  BOOLEAN,
    last_motion    BOOLEAN
);
CREATE INDEX vehicles_driver_idx ON vehicles (driver_id);

CREATE TABLE vehicle_positions (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    vehicle_id          BIGINT      NOT NULL REFERENCES vehicles (id) ON DELETE CASCADE,
    traccar_position_id BIGINT      NOT NULL,
    fix_at              TIMESTAMPTZ NOT NULL,  -- quando o GPS mediu (pode chegar horas depois, sem sinal)
    server_at           TIMESTAMPTZ,           -- quando chegou no Traccar
    latitude            DOUBLE PRECISION NOT NULL,
    longitude           DOUBLE PRECISION NOT NULL,
    speed_kmh           REAL,
    course              SMALLINT,
    altitude            REAL,
    ignition            BOOLEAN,
    motion              BOOLEAN,
    valid               BOOLEAN,               -- false = GPS sem satélites suficientes
    attributes          JSONB,                 -- o resto do que o rastreador mandou (bateria, odômetro...)
    UNIQUE (vehicle_id, traccar_position_id)
);
CREATE INDEX vehicle_positions_vehicle_fix_idx ON vehicle_positions (vehicle_id, fix_at);
CREATE INDEX vehicle_positions_fix_idx ON vehicle_positions (fix_at);

-- Situação da conexão com o Traccar (uma linha só): o mapa avisa quando as
-- posições pararam de chegar porque a internet do box caiu.
CREATE TABLE fleet_sync (
    id              SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    last_attempt_at TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    last_error      TEXT,
    last_error_at   TIMESTAMPTZ
);
INSERT INTO fleet_sync (id) VALUES (1);

-- Tempo real (regra do projeto: TestEveryTableNotifiesChanges).
CREATE TRIGGER drivers_notify AFTER INSERT OR UPDATE OR DELETE ON drivers
    FOR EACH ROW EXECUTE FUNCTION notify_change('drivers', 'id');
CREATE TRIGGER vehicles_notify AFTER INSERT OR UPDATE OR DELETE ON vehicles
    FOR EACH ROW EXECUTE FUNCTION notify_change('vehicles', 'id');
-- Muitas posições na mesma transação viram um aviso por veículo (o Postgres
-- junta avisos iguais).
CREATE TRIGGER vehicle_positions_notify AFTER INSERT OR UPDATE OR DELETE ON vehicle_positions
    FOR EACH ROW EXECUTE FUNCTION notify_change('vehicle_positions', 'vehicle_id');
CREATE TRIGGER fleet_sync_notify AFTER INSERT OR UPDATE ON fleet_sync
    FOR EACH ROW EXECUTE FUNCTION notify_change('fleet_sync', 'id');

-- Permissões padrão (o administrador pode mudar na tela de papéis).
INSERT INTO role_permissions (role_id, permission)
SELECT r.id, p.permission
FROM roles r
JOIN (VALUES
    ('manager', 'fleet.view'), ('manager', 'fleet.edit')
) AS p (role_key, permission) ON p.role_key = r.key
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM role_permissions WHERE permission IN ('fleet.view', 'fleet.edit');
DROP TABLE fleet_sync;
DROP TABLE vehicle_positions;
DROP TABLE vehicles;
DROP TABLE drivers;
