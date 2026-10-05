-- Caducidad por inactividad de las sesiones (ASVS V3.3.2, ADR 0025). 00029: main y esta rama llegan a 00028.
-- PorToken rechaza una sesion cuyo ultimo_uso es anterior a la inactividad maxima y lo corre al usarla.
-- Las filas que ya existen arrancan en now(): ninguna sesion viva se corta al desplegar.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE sesiones
  ADD COLUMN ultimo_uso TIMESTAMPTZ NOT NULL DEFAULT now();
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE sesiones DROP COLUMN IF EXISTS ultimo_uso;
-- +goose StatementEnd
