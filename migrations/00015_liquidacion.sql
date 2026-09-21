-- Ordenes de pago (R-10, R-11, R-12) y documentos del titular para cobrar.
--
-- Bruto y neto son COLUMNAS de `ordenes_pago`; lo que vive en una tabla aparte
-- es solo el desglose de deducciones, que es de cardinalidad abierta -- las
-- tres de la corrida hoy, un anticipo de R-33 manana -- y no cabe en columnas
-- fijas. OE-4 y OE-6 piden ver los tres numeros por separado en cada consulta,
-- y colapsar las deducciones en el neto haria inexpresable el resumen de
-- RD 13.2.
--
-- UNA orden por (titular, periodo, circuito), no por corrida (ADR 0019): un
-- periodo se cierra con tantas corridas como bolsas tenga el circuito, y el 2%
-- de un SMMLV de R-11 se mide sobre lo que el titular cobra por el periodo.
-- Por eso la clave unica es esa terna y no (proceso_id, titular_id).
--
-- COORDINACION DE NUMERO: esta migracion nacio como 00002, paso a 00003
-- cuando el catalogo (#86) se quedo esa version, y luego a 00007 cuando
-- #85/#72 ocuparon 00005/00006. El 00007 ya no se puede usar: main
-- mergeo `00007_uso_excluido_no_es_oni.sql`, y goose aborta con
-- `duplicate version 7` si dos archivos llevan el mismo numero.
--
-- goose corre con `allowMissing = false`. Un numero LIBRE por debajo de
-- la version ya aplicada aborta con
--
--     found 1 missing migrations before current version N
--
-- antes de aplicar nada, y el despliegue condiciona el rollout a que
-- goose termine bien. Por eso no se rellena el hueco 00003/00004.
--
-- Se toma el 00015: main mergeo `00013_matching_difuso.sql` y
-- `00014_usos_titulo_original.sql`, y goose aborta con `duplicate version 13`
-- si dos archivos llevan el mismo numero. El mayor en `main` es 00014. Un ADR
-- admite huecos; una migracion no. No rellenar huecos por debajo de la
-- version ya aplicada.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE ordenes_pago (
  id          TEXT PRIMARY KEY,

  -- La corrida de REFERENCIA, no la unica que aporto: cuando varias corridas
  -- del periodo y circuito contribuyen, es la primera por orden lexicografico.
  -- Se queda como clave foranea porque es lo que ata la orden a un proceso que
  -- existe de verdad; `procesos` lleva la lista completa.
  proceso_id  TEXT NOT NULL REFERENCES procesos(id),

  -- Todas las corridas cuyas lineas entraron en esta orden, ordenadas. Sin
  -- ella una orden agregada no dice de donde salio su bruto, que es justo la
  -- pregunta 4 del ADR 0006. Sin clave foranea -- PostgreSQL no las admite
  -- sobre elementos de un array --, asi que es un registro de procedencia y no
  -- una integridad que la base haga cumplir.
  procesos    TEXT[] NOT NULL DEFAULT '{}',

  titular_id  TEXT NOT NULL REFERENCES titulares(id),
  periodo     TEXT NOT NULL CHECK (periodo ~ '^[0-9]{4}(-[0-9]{2})?$'),

  -- El circuito es parte de la IDENTIDAD de la orden, no un adorno: el
  -- nacional y el internacional del mismo periodo son dos recorridos distintos
  -- (RD 7.4, ADR 0008) y no se suman en una sola orden, igual que no se suman
  -- en una sola bolsa (ver el UNIQUE de `bolsas`).
  circuito    TEXT NOT NULL CHECK (circuito IN ('nacional','internacional')),

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
  -- sobre esta fecha, no sobre un timestamptz: calendario, no 15*24h. Se
  -- escribe cuando la notificacion ya dejo acuse: el plazo corre desde el
  -- envio, y una fecha sin acuse le opondria al titular un plazo que empezo
  -- sin que a el le llegara nada.
  enviada     DATE NOT NULL,
  arrastres   TEXT[] NOT NULL DEFAULT '{}',

  -- La clave del ADR 0019. Es lo que hace idempotente la generacion: dos
  -- corridas del mismo periodo y circuito producen el mismo id de orden para
  -- el mismo titular, asi que la segunda choca aqui en vez de abrir una orden
  -- paralela que partiria el umbral de R-11 en dos mitades.
  UNIQUE (titular_id, periodo, circuito),
  CONSTRAINT orden_neto_no_supera_bruto CHECK (neto <= bruto)
);
CREATE INDEX ordenes_pago_titular ON ordenes_pago (titular_id);
CREATE INDEX ordenes_pago_estado  ON ordenes_pago (estado);
-- La lectura por la que se decide si un periodo ya se liquido, y la que
-- responde el listado de administracion.
CREATE INDEX ordenes_pago_periodo_circuito ON ordenes_pago (periodo, circuito);
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
