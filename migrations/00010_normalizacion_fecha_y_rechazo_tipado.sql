-- Normalizacion (#26): fecha/hora canonicas en usos, y discriminante tipado
-- en el log de rechazos (B3).
--
-- COORDINACION DE NUMERO: el mayor aplicado en main al escribir esto era
-- 00009. Se toma el siguiente libre. Nunca un hueco por debajo de la version
-- ya aplicada (goose allowMissing=false; ver 00006).

-- +goose Up
-- +goose StatementBegin
ALTER TABLE usos
  ADD COLUMN fecha TEXT NOT NULL DEFAULT '',
  ADD COLUMN hora  TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE usos_rechazados
  ADD COLUMN tipo   TEXT NOT NULL DEFAULT 'adaptador'
    CHECK (tipo IN ('normalizacion', 'anomalia', 'adaptador')),
  ADD COLUMN codigo TEXT NOT NULL DEFAULT 'rechazo_formato';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE usos_rechazados DROP COLUMN IF EXISTS codigo;
ALTER TABLE usos_rechazados DROP COLUMN IF EXISTS tipo;
ALTER TABLE usos DROP COLUMN IF EXISTS hora;
ALTER TABLE usos DROP COLUMN IF EXISTS fecha;
-- +goose StatementEnd
