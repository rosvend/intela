-- Esquema canonico de usos para RD 9 (#119) y modalidades restantes (#120).
--
-- Amplia modalidad, anade las medidas que las formulas piden (espectadores,
-- exhibiciones), atribucion de canal, registro de canales con clasificacion
-- por vigencia (RD 9.5.4), y cierra tipo_obra a las categorias de RD 9.1.1.
--
-- Numero: 00011, primero libre por encima de 00010 (normalizacion #26/#103).
-- Nunca en el hueco 00003/00004 (cabecera de 00006_log_de_rechazos.sql).
--
-- oni_publico (00001) proyecta modalidad sin filtrar: al ampliar el CHECK,
-- teatro/transporte/suscripcion pueden aparecer en el listado publico. Revisar
-- R-18 / RD 13.8 antes de exponer esas filas en produccion.
--
-- Los IF EXISTS / IF NOT EXISTS protegen los objetos que Postgres permite
-- crear o quitar condicionalmente. ADD CONSTRAINT no admite IF NOT EXISTS:
-- se hace DROP antes. El CHECK inline de modalidad creado por 00001 recibe
-- automaticamente de Postgres el nombre usos_modalidad_check, que es el que
-- se reemplaza aqui.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE usos DROP CONSTRAINT IF EXISTS usos_modalidad_check;
ALTER TABLE usos ADD CONSTRAINT usos_modalidad_check
  CHECK (modalidad IN (
    'tv', 'cine', 'ott', 'hotel', 'teatro', 'transporte', 'suscripcion'
  ));
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE usos
  ADD COLUMN IF NOT EXISTS espectadores  NUMERIC(18,2) NOT NULL DEFAULT 0
    CHECK (espectadores >= 0),
  ADD COLUMN IF NOT EXISTS exhibiciones  BIGINT        NOT NULL DEFAULT 0
    CHECK (exhibiciones >= 0),
  -- Sin FK a canales: canal_id conserva el identificador declarado por una
  -- fuente aunque el catalogo anual aun no lo conozca. DEFAULT '' hace
  -- explicita la ausencia, como el usuario_id libre anterior a 00009.
  ADD COLUMN IF NOT EXISTS canal_id      TEXT          NOT NULL DEFAULT '';
-- +goose StatementEnd

-- El CHECK se anade despues de canonizar los datos existentes. Los valores
-- desconocidos no se corrigen automaticamente: antes de desplegar, esta
-- consulta lista los que harian fallar el ALTER para reconciliarlos:
--
--     SELECT DISTINCT tipo_obra
--       FROM usos
--      WHERE lower(btrim(tipo_obra)) NOT IN (
--        '', 'cinematografica', 'unitario', 'serie', 'telenovela', 'sketches'
--      );
-- +goose StatementBegin
UPDATE usos
   SET tipo_obra = lower(trim(tipo_obra))
 WHERE tipo_obra <> lower(trim(tipo_obra));
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE usos DROP CONSTRAINT IF EXISTS usos_tipo_obra_check;
ALTER TABLE usos ADD CONSTRAINT usos_tipo_obra_check
  CHECK (tipo_obra IN (
    '', 'cinematografica', 'unitario', 'serie', 'telenovela', 'sketches'
  ));
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE canales (
  id                TEXT PRIMARY KEY,
  nombre            TEXT NOT NULL,
  -- Clasificacion estructural (RD 9.5.1–9.5.3 y el resto cerrado).
  -- lideres_rating / estandar NO viven aqui: se resuelven por ano en
  -- canales_clasificacion (RD 9.5.4 / 9.5.5).
  grupo_estructural TEXT NOT NULL CHECK (grupo_estructural IN (
    'privado_nacional', 'regional_publico', 'premium', 'cerrado'
  ))
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE canales_clasificacion (
  canal_id        TEXT NOT NULL REFERENCES canales(id) ON DELETE CASCADE,
  -- Ano de la audiencia usada para clasificar (el inmediatamente anterior
  -- al periodo que se reparte). Reejecutar un periodo pasado lee la fila
  -- de aquel anio, no la vigente hoy (ADR 0005, RD 9.5.4).
  anio_audiencia  INT  NOT NULL CHECK (anio_audiencia >= 1900),
  grupo_efectivo  TEXT NOT NULL CHECK (grupo_efectivo IN (
    'privado_nacional', 'regional_publico', 'premium',
    'lideres_rating', 'estandar'
  )),
  -- PK en vez de EXCLUDE gist: la vigencia es una clave anual discreta, no
  -- un rango solapable; por canal y ano solo puede existir una clasificacion.
  PRIMARY KEY (canal_id, anio_audiencia)
);
CREATE INDEX canales_clasificacion_anio ON canales_clasificacion (anio_audiencia);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS canales_clasificacion;
DROP TABLE IF EXISTS canales;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE usos DROP CONSTRAINT IF EXISTS usos_tipo_obra_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE usos
  DROP COLUMN IF EXISTS canal_id,
  DROP COLUMN IF EXISTS exhibiciones,
  DROP COLUMN IF EXISTS espectadores;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE usos DROP CONSTRAINT IF EXISTS usos_modalidad_check;
ALTER TABLE usos ADD CONSTRAINT usos_modalidad_check
  CHECK (modalidad IN ('tv', 'cine', 'ott', 'hotel'));
-- +goose StatementEnd
