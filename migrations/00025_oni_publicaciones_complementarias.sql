-- Publicaciones ONI complementarias (issue #184, R-18, R-19, RD 13.8.7).
--
-- 00023 tenia UNIQUE (periodo) en oni_publicaciones. Si un uso llegaba
-- despues de publicar el periodo (reporte tardio o uso reabierto) y quedaba
-- ONI, nunca se podia publicar porque republicar el periodo devolvia 409
-- (ErrYaPublicado), y su publicado_en quedaba NULL sin arrancar nunca el reloj
-- de prescripcion de R-19.
--
-- Esta migracion elimina UNIQUE (periodo), introduce la columna secuencia
-- (1 = publicacion inicial, 2+ = publicaciones complementarias) con
-- UNIQUE (periodo, secuencia), y permite publicaciones complementarias que
-- congelan y anclan unicamente los usos ONI con publicado_en NULL.
--
-- COORDINACION DE NUMERO: 00025 es el primer numero libre por encima de
-- 00024 (`00024_resultados_titular_declaracion_version.sql`). goose corre
-- con allowMissing = false; un numero libre por debajo de la version ya
-- aplicada aborta el despliegue.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE oni_publicaciones
  ADD COLUMN secuencia INT NOT NULL DEFAULT 1 CHECK (secuencia >= 1);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE oni_publicaciones
  DROP CONSTRAINT IF EXISTS oni_publicaciones_periodo_key;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE oni_publicaciones
  ADD CONSTRAINT oni_publicaciones_periodo_secuencia_key UNIQUE (periodo, secuencia);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS oni_publicaciones_periodo_secuencia
  ON oni_publicaciones (periodo, secuencia DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS usos_oni_pendientes_publicar
  ON usos (reporte_id) WHERE escalon = 'oni' AND publicado_en IS NULL;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP INDEX IF EXISTS usos_oni_pendientes_publicar;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS oni_publicaciones_periodo_secuencia;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE oni_publicaciones
  DROP CONSTRAINT IF EXISTS oni_publicaciones_periodo_secuencia_key;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE oni_publicaciones
  ADD CONSTRAINT oni_publicaciones_periodo_key UNIQUE (periodo);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE oni_publicaciones
  DROP COLUMN IF EXISTS secuencia;
-- +goose StatementEnd
