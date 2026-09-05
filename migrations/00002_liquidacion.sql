-- Ordenes de pago (R-10, R-11, R-12) y documentos del titular para cobrar.
--
-- Bruto, cada deduccion y neto viven en tablas distintas a proposito: OE-4 y
-- OE-6 piden el desglose en cada consulta. Colapsar las deducciones en el
-- neto haria inexpresable el resumen que exige RD 13.2.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE ordenes_pago (
  id          TEXT PRIMARY KEY,
  proceso_id  TEXT NOT NULL REFERENCES procesos(id),
  titular_id  TEXT NOT NULL REFERENCES titulares(id),
  periodo     TEXT NOT NULL CHECK (periodo ~ '^[0-9]{4}(-[0-9]{2})?$'),
  bruto       NUMERIC(18,2) NOT NULL CHECK (bruto >= 0),
  neto        NUMERIC(18,2) NOT NULL CHECK (neto  >= 0),
  estado      TEXT NOT NULL CHECK (estado IN (
                'enviada',
                'aceptada',
                'aceptada_por_silencio',
                'diferida',
                'acumulada',
                'objetada')),
  -- Dia civil del envio. El plazo de 15 dias de R-10 / RD 13.2 se cuenta
  -- sobre esta fecha, no sobre un timestamptz: calendario, no 15*24h.
  enviada     DATE NOT NULL,
  arrastres   TEXT[] NOT NULL DEFAULT '{}',
  UNIQUE (proceso_id, titular_id),
  CONSTRAINT orden_neto_no_supera_bruto CHECK (neto <= bruto)
);
CREATE INDEX ordenes_pago_titular ON ordenes_pago (titular_id);
CREATE INDEX ordenes_pago_estado  ON ordenes_pago (estado);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE ordenes_pago_deducciones (
  orden_id TEXT NOT NULL REFERENCES ordenes_pago(id) ON DELETE CASCADE,
  concepto TEXT NOT NULL CHECK (btrim(concepto) <> ''),
  monto    NUMERIC(18,2) NOT NULL CHECK (monto >= 0),
  PRIMARY KEY (orden_id, concepto)
);
-- +goose StatementEnd

-- R-12 / RD 13.1.6: RUT y certificacion bancaria. Su ausencia bloquea el
-- pago, no la liquidacion. La clave del objeto es la evidencia; el tipo
-- es lo que consulta EsPagable.
-- +goose StatementBegin
CREATE TABLE documentos_titular (
  titular_id   TEXT NOT NULL REFERENCES titulares(id),
  tipo         TEXT NOT NULL CHECK (tipo IN ('rut', 'certificacion_bancaria')),
  clave_objeto TEXT NOT NULL DEFAULT '',
  aportado     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (titular_id, tipo)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS documentos_titular;
DROP TABLE IF EXISTS ordenes_pago_deducciones;
DROP TABLE IF EXISTS ordenes_pago;
-- +goose StatementEnd
