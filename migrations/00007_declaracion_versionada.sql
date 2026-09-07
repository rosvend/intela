-- Declaracion de Obra versionada por vigencia (#23).
--
-- `declaraciones` tenia PRIMARY KEY (obra_id, titular_id): una sola fila por
-- par, sin espacio para una segunda version. La columna `version` ya estaba
-- desde 00001 pero no formaba parte de la clave, asi que no protegia nada -era
-- decorativa hasta este PR-.
--
-- Editar una declaracion tiene que CONSERVAR la anterior (Objetivo 5: un
-- reparto de un periodo pasado usa el split que estaba vigente ENTONCES, no el
-- de hoy). Eso pide saber, para una obra y un instante, cual version regia.
--
-- Se resuelve con una tabla de CABECERA por version -`declaracion_versiones`,
-- con su ventana de vigencia- y `declaraciones` como sus PARTES. La cabecera y
-- no la fila de parte es la que lleva la vigencia, porque una version tiene
-- una parte por titular y la ventana es del conjunto, no de cada fila: meterla
-- en `declaraciones` no deja forma limpia de decir "una sola version abierta
-- por obra" cuando esa version son N filas.
--
-- El patron -EXCLUDE sobre un rango de tiempo para que dos vigencias de la
-- misma clave no se solapen- es el mismo que ya resuelve esto para
-- `parametros` en 00001. `btree_gist` ya esta habilitada desde ahi.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE declaracion_versiones (
  obra_id       TEXT NOT NULL REFERENCES obras(id) ON DELETE CASCADE,
  version       INT  NOT NULL CHECK (version >= 1),
  vigente_desde TIMESTAMPTZ NOT NULL,
  -- NULL = version abierta, la vigente ahora mismo.
  vigente_hasta TIMESTAMPTZ,

  PRIMARY KEY (obra_id, version),

  CONSTRAINT declaracion_vigencia_coherente
    CHECK (vigente_hasta IS NULL OR vigente_hasta > vigente_desde),

  -- Sin este EXCLUDE, dos versiones de la misma obra con vigencias que se
  -- solapan dejarian "la vigente en el instante T" con mas de una respuesta,
  -- y el ADR 0005 exige que esa pregunta tenga una sola.
  CONSTRAINT declaracion_sin_solape EXCLUDE USING gist (
    obra_id WITH =,
    tstzrange(vigente_desde, vigente_hasta, '[)') WITH &&
  )
);

-- Acelera "cual es la version abierta de esta obra" -la pregunta que hace
-- cada escritura, para saber cual cerrar antes de abrir la siguiente-. Parcial
-- porque solo hay como mucho una fila por obra que la cumple.
CREATE INDEX declaracion_versiones_abierta
  ON declaracion_versiones (obra_id) WHERE vigente_hasta IS NULL;
-- +goose StatementEnd

-- Backfill: toda fila que ya exista en `declaraciones` trae version = 1 por el
-- DEFAULT de 00001. Sin una cabecera para esa version, la FK de mas abajo no
-- podria crearse sobre datos ya cargados (seed incluido).
-- +goose StatementBegin
-- NULL::timestamptz y no NULL a secas: con SELECT DISTINCT, Postgres resuelve
-- el tipo del literal ANTES de coercionarlo al tipo de la columna destino, y
-- un NULL sin tipo cae en TEXT -que el INSERT ya no convierte solo, y aborta
-- la migracion entera con "column vigente_hasta is of type timestamp with
-- time zone but expression is of type text".
INSERT INTO declaracion_versiones (obra_id, version, vigente_desde, vigente_hasta)
SELECT DISTINCT obra_id, 1, now(), NULL::timestamptz
FROM declaraciones;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE declaraciones
  DROP CONSTRAINT declaraciones_pkey,
  ADD CONSTRAINT declaraciones_pkey PRIMARY KEY (obra_id, version, titular_id),
  ADD CONSTRAINT declaraciones_version_fkey
    FOREIGN KEY (obra_id, version) REFERENCES declaracion_versiones (obra_id, version)
    ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose Down

-- Restaurar la PK vieja (obra_id, titular_id) solo es posible si nadie llego
-- a escribir una segunda version -si la hubo, el downgrade tiene que fallar
-- en vez de descartar filas en silencio, y un PRIMARY KEY duplicado es
-- exactamente ese fallo.
-- +goose StatementBegin
ALTER TABLE declaraciones
  DROP CONSTRAINT declaraciones_version_fkey,
  DROP CONSTRAINT declaraciones_pkey,
  ADD CONSTRAINT declaraciones_pkey PRIMARY KEY (obra_id, titular_id);

DROP TABLE declaracion_versiones;
-- +goose StatementEnd
