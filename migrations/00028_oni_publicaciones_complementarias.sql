-- Publicaciones ONI complementarias (#184). 00028: main llega a 00026 y #199 tomo 00027. Ver docs/planes/184-oni-tardios.md.

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
CREATE INDEX IF NOT EXISTS usos_oni_pendientes_publicar
  ON usos (reporte_id) WHERE escalon = 'oni' AND publicado_en IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS oni_publicacion_items_uso;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX IF NOT EXISTS oni_publicacion_items_uso
  ON oni_publicacion_items (uso_id);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP INDEX IF EXISTS oni_publicacion_items_uso;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS oni_publicacion_items_uso
  ON oni_publicacion_items (uso_id);
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS usos_oni_pendientes_publicar;
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
