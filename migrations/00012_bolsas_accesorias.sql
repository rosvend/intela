-- Bolsas accesorias: reserva de errores tecnicos (RD 14) y rendimientos
-- financieros (RD 10) (#121).
--
-- Numero: 00012, primero libre por encima de 00011 (esquema canonico de
-- usos #119/#120). Nunca en el hueco 00003/00004 (cabecera de
-- 00006_log_de_rechazos.sql).
--
-- `reservas` es una fila por corrida (ADR 0019: una corrida = una bolsa = un
-- proceso), con la proveniencia como PK: RD 14.4 exige liberar el remanente
-- proporcional a como se distribuyo ESA corrida, incluso anos despues.
--
-- `reclamaciones_avales` mira a `firmas` (00001) pero es tabla propia y no
-- una extension de su CHECK: RD 14.5.11/14.5.12 nombran roles distintos
-- (revisoria fiscal/auditoria interna, distribucion y contabilidad) de los
-- que ya fijo `firmas` para las compuertas de RD 13.5, y una reclamacion no
-- es una revision de un proceso.
--
-- `reclamaciones` (00001) ya tiene proceso_id: ReclamacionReserva.ProcesoOrigenID
-- se guarda ahi, no en una columna nueva. Solo hace falta monto_solicitado.
--
-- `rendimientos` es un ledger por (circuito, vigencia): RD 10.3 exige
-- segregacion nacional/internacional, y la PK compuesta lo hace estructural,
-- no solo documentado.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE reservas (
  proceso_id      TEXT PRIMARY KEY REFERENCES procesos(id) ON DELETE CASCADE,
  -- RD 14.5.4: la reserva no aplica al recaudo internacional. El CHECK lo
  -- hace imposible en la fila, ademas de en el constructor de dominio.
  circuito        TEXT NOT NULL CHECK (circuito = 'nacional'),
  monto_inicial   NUMERIC(18,2) NOT NULL CHECK (monto_inicial >= 0),
  saldo           NUMERIC(18,2) NOT NULL CHECK (saldo >= 0),
  tasa_pct        NUMERIC(8,4)  NOT NULL CHECK (tasa_pct > 0 AND tasa_pct <= 5),
  -- RD 14.5.1: el organo aprobador es siempre la Asamblea General. Fijo por
  -- CHECK, igual que en el constructor de dominio: no hay forma de guardar
  -- una reserva con otro organo.
  organo_aprobador TEXT NOT NULL DEFAULT 'Asamblea General'
    CHECK (organo_aprobador = 'Asamblea General')
);
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

  -- Mismo invariante que `firmas` (00001): un actor no puede cubrir los dos
  -- avales de la misma reclamacion.
  CONSTRAINT reclamaciones_avales_actor_unico
    UNIQUE (reclamacion_id, actor_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE rendimientos (
  -- RD 10.3: nacional e internacional nunca comparten fila. La PK compuesta
  -- es la segregacion, no una columna discriminadora que alguien pueda
  -- olvidar filtrar.
  circuito TEXT NOT NULL CHECK (circuito IN ('nacional', 'internacional')),
  -- Vigencia es el ano de la comunicacion publica que se reparte (RD 10.1),
  -- no el ano en que llego el pago. Texto, como periodo en procesos: no hay
  -- time.Time en el dominio que lo consume.
  vigencia TEXT NOT NULL CHECK (vigencia ~ '^[0-9]{4}$'),
  monto    NUMERIC(18,2) NOT NULL CHECK (monto >= 0),
  PRIMARY KEY (circuito, vigencia)
);
-- +goose StatementEnd

-- +goose Down

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
DROP TABLE IF EXISTS reservas;
-- +goose StatementEnd
