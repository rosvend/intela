-- Escalon 3 de la cascada (#32): titulo normalizado indexable y los candidatos
-- que la cascada considero y no eligio.
--
-- Diseno y alternativas: docs/planes/32-difuso/diseno.md
-- Por que GiST y no GIN, medido: docs/planes/32-difuso/explain-trgm.md

-- +goose Up

-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS unaccent;

-- IMMUTABLE envolviendo a unaccent(), que en PostgreSQL 16 es STABLE en sus dos
-- formas: es el rodeo documentado, y sin el no se puede crear ni la columna
-- generada ni el indice. Precio asumido: si se reescribe unaccent.rules hay que
-- REINDEX. La expresion replica normalize_text de src/scripts/sample.py para que
-- "el mismo titulo" tenga un solo dueno.
CREATE FUNCTION titulo_normalizado(t TEXT) RETURNS TEXT
  LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS $$
    SELECT btrim(regexp_replace(
             lower(public.unaccent('public.unaccent'::regdictionary, t)),
             '[^a-z0-9]+', ' ', 'g'))
  $$;

-- STORED: la consulta del escalon 3 tambien puntua contra esta columna.
ALTER TABLE obras
  ADD COLUMN titulo_norm TEXT GENERATED ALWAYS AS (titulo_normalizado(titulo)) STORED;

-- GiST y no GIN: con GIN el planificador prefiere el barrido secuencial (68 ms
-- frente a 3,1 ms), y un indice que no se elige no existe. `obras_titulo_trgm`
-- se queda: sirve al ILIKE, que es otro operador.
CREATE INDEX obras_titulo_norm_gist ON obras USING gist (titulo_norm gist_trgm_ops);
-- +goose StatementEnd

-- Los candidatos de la banda ambigua. No es una cache: es la evidencia de que
-- algo se considero y se descarto, con cuanto. Sin ella, la bandeja de #39
-- mostraria el catalogo de hoy y no el de cuando se decidio (RD 16).
--
-- Sin columna de dinero, igual que `usos`: identificacion no toca dinero
-- (ADR 0003). `puntaje` repite el dominio de `usos.puntaje` para que una no
-- acepte lo que la otra rechaza a mitad de una corrida.
-- +goose StatementBegin
CREATE TABLE candidatos_match (
  uso_id  TEXT NOT NULL REFERENCES usos(id)  ON DELETE CASCADE,
  obra_id TEXT NOT NULL REFERENCES obras(id) ON DELETE CASCADE,
  puntaje NUMERIC(6,5) NOT NULL CHECK (puntaje >= 0 AND puntaje <= 1),
  orden   INT NOT NULL CHECK (orden >= 0),
  PRIMARY KEY (uso_id, obra_id)
);

CREATE INDEX candidatos_match_uso ON candidatos_match (uso_id, orden);
-- +goose StatementEnd

-- +goose Down

-- El orden importa: la columna depende de la funcion y la funcion de la
-- extension.
-- +goose StatementBegin
DROP TABLE IF EXISTS candidatos_match;

DROP INDEX IF EXISTS obras_titulo_norm_gist;
ALTER TABLE obras DROP COLUMN IF EXISTS titulo_norm;
DROP FUNCTION IF EXISTS titulo_normalizado(TEXT);
DROP EXTENSION IF EXISTS unaccent;
-- +goose StatementEnd
