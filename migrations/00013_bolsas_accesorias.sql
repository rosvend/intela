-- Bolsas accesorias: reserva de errores tecnicos (RD 14) y rendimientos financieros (RD 10) (#121).
-- Numero 00013: 00012 lo tomo #118 (snapshots de parametros) al mergear antes.
-- reclamaciones_avales es tabla propia, no extension del CHECK de firmas: RD 14.5.11/12 usa otros roles.
-- reclamaciones.proceso_id (00001) ya cubre ProcesoOrigenID; solo hace falta monto_solicitado.
-- reservas_liberaciones/rendimientos_distribuciones: lo repartido queda escrito en la misma
-- transaccion que baja el saldo/monto, para que un crash entre el commit y el pago no pierda el rastro.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE reservas (
  proceso_id      TEXT PRIMARY KEY REFERENCES procesos(id) ON DELETE CASCADE,
  -- RD 14.5.4: la reserva no aplica al recaudo internacional.
  circuito        TEXT NOT NULL CHECK (circuito = 'nacional'),
  monto_inicial   NUMERIC(18,2) NOT NULL CHECK (monto_inicial >= 0),
  saldo           NUMERIC(18,2) NOT NULL CHECK (saldo >= 0),
  tasa_pct        NUMERIC(8,4)  NOT NULL CHECK (tasa_pct > 0 AND tasa_pct <= 5),
  -- RD 14.5.1: el organo aprobador siempre es la Asamblea General.
  organo_aprobador TEXT NOT NULL DEFAULT 'Asamblea General'
    CHECK (organo_aprobador = 'Asamblea General')
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE reservas_liberaciones (
  id          BIGSERIAL PRIMARY KEY,
  proceso_id  TEXT NOT NULL REFERENCES reservas(proceso_id) ON DELETE CASCADE,
  obra_id     TEXT NOT NULL REFERENCES obras(id),
  titular_id  TEXT NOT NULL REFERENCES titulares(id),
  ipi         TEXT NOT NULL,
  porcentaje  NUMERIC(8,4)  NOT NULL,
  importe     NUMERIC(18,2) NOT NULL CHECK (importe >= 0),
  liberado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX reservas_liberaciones_proceso ON reservas_liberaciones (proceso_id);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE reclamaciones
  ADD COLUMN IF NOT EXISTS monto_solicitado NUMERIC(18,2) CHECK (monto_solicitado > 0);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE reclamaciones_avales (
  reclamacion_id TEXT NOT NULL REFERENCES reclamaciones(id) ON DELETE CASCADE,
  rol            TEXT NOT NULL CHECK (rol IN (
                    'revisoria_fiscal_o_auditoria_interna',
                    'distribucion_y_contabilidad'
                  )),
  actor_id       TEXT NOT NULL REFERENCES usuarios(id),
  cuando         TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (reclamacion_id, rol),

  -- Mismo invariante que firmas (00001): un actor no cubre los dos avales.
  CONSTRAINT reclamaciones_avales_actor_unico
    UNIQUE (reclamacion_id, actor_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE rendimientos (
  -- RD 10.3: la PK compuesta hace la segregacion nacional/internacional estructural.
  circuito TEXT NOT NULL CHECK (circuito IN ('nacional', 'internacional')),
  -- Ano de la comunicacion publica (RD 10.1), no el ano en que llego el pago.
  vigencia TEXT NOT NULL CHECK (vigencia ~ '^[0-9]{4}$'),
  monto    NUMERIC(18,2) NOT NULL CHECK (monto >= 0),
  PRIMARY KEY (circuito, vigencia)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE rendimientos_distribuciones (
  id             BIGSERIAL PRIMARY KEY,
  circuito       TEXT NOT NULL,
  vigencia       TEXT NOT NULL,
  proceso_id     TEXT NOT NULL REFERENCES procesos(id),
  obra_id        TEXT NOT NULL REFERENCES obras(id),
  titular_id     TEXT NOT NULL REFERENCES titulares(id),
  ipi            TEXT NOT NULL,
  porcentaje     NUMERIC(8,4)  NOT NULL,
  importe        NUMERIC(18,2) NOT NULL CHECK (importe >= 0),
  distribuido_en TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (circuito, vigencia) REFERENCES rendimientos (circuito, vigencia) ON DELETE CASCADE
);
CREATE INDEX rendimientos_distribuciones_clave ON rendimientos_distribuciones (circuito, vigencia);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS rendimientos_distribuciones;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS rendimientos;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS reclamaciones_avales;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE reclamaciones DROP COLUMN IF EXISTS monto_solicitado;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS reservas_liberaciones;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS reservas;
-- +goose StatementEnd
