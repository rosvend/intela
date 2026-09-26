-- La bandeja de anomalias de un periodo (#37, OE-5 / KR-4).
-- Decision: ADR 0021. Razones de cada columna y restriccion: docs/planes/37/diseno.md (D8).

-- +goose Up

-- +goose StatementBegin
CREATE TABLE alertas (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  -- Mismo patron que reportes.periodo; el nucleo valida mas estricto (recaudo.ValidarPeriodo).
  periodo      TEXT NOT NULL CHECK (periodo ~ '^[0-9]{4}(-[0-9]{2})?$'),
  tipo         TEXT NOT NULL CHECK (tipo IN (
                 'oni',
                 'duplicado_archivo',
                 'duplicado_registro',
                 'titular_sin_porcentaje',
                 'reserva_declaracion_incompleta',
                 'tipo_obra_sin_mapear'
               )),

  -- Referencia polimorfica sin FK, como asientos (00001).
  ref_tipo     TEXT NOT NULL CHECK (ref_tipo IN ('uso', 'reporte', 'obra')),
  ref_id       TEXT NOT NULL CHECK (btrim(ref_id) <> ''),
  ref_titular  TEXT NOT NULL DEFAULT '',

  detalle      TEXT NOT NULL CHECK (btrim(detalle) <> ''),
  detectada    TIMESTAMPTZ NOT NULL,

  resuelta     BOOLEAN NOT NULL DEFAULT FALSE,
  resuelta_por TEXT REFERENCES usuarios(id),
  resuelta_en  TIMESTAMPTZ,
  nota         TEXT NOT NULL DEFAULT '',
  -- Cerrada por el sistema porque una reevaluacion ya no la detecto.
  autocerrada  BOOLEAN NOT NULL DEFAULT FALSE,

  -- Resuelta = firma de una persona XOR autocierre del sistema, con instante.
  CONSTRAINT alerta_resuelta_tiene_firma
    CHECK (
      (resuelta AND resuelta_en IS NOT NULL
        AND ((resuelta_por IS NOT NULL AND NOT autocerrada)
          OR (resuelta_por IS NULL AND autocerrada)))
      OR (NOT resuelta AND NOT autocerrada AND resuelta_por IS NULL AND resuelta_en IS NULL)
    ),

  CONSTRAINT alerta_resuelta_tiene_nota
    CHECK (NOT resuelta OR btrim(nota) <> ''),

  CONSTRAINT alerta_ref_titular_solo_de_titulares
    CHECK (ref_titular = '' OR tipo = 'titular_sin_porcentaje'),

  -- Clave natural: hace idempotente a la evaluacion.
  CONSTRAINT alerta_unica_por_hallazgo
    UNIQUE (periodo, tipo, ref_tipo, ref_id, ref_titular)
);
-- +goose StatementEnd

-- Bandeja del periodo, de la mas reciente a la mas antigua.
-- +goose StatementBegin
CREATE INDEX alertas_periodo ON alertas (periodo, detectada DESC, id);
-- +goose StatementEnd

-- Lo que cuenta la compuerta: abiertas por periodo y tipo.
-- +goose StatementBegin
CREATE INDEX alertas_abiertas ON alertas (periodo, tipo) WHERE NOT resuelta;
-- +goose StatementEnd

-- Del registro ofensor a sus alertas (bandeja de #39).
-- +goose StatementBegin
CREATE INDEX alertas_ref ON alertas (ref_tipo, ref_id);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP INDEX IF EXISTS alertas_ref;
DROP INDEX IF EXISTS alertas_abiertas;
DROP INDEX IF EXISTS alertas_periodo;
DROP TABLE IF EXISTS alertas;
-- +goose StatementEnd
