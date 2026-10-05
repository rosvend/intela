-- Indice semantico de los reglamentos para la herramienta buscar_reglamento (#67).
--
-- Una fila por numeral citable (RD 9.1.1) y por modelo de embeddings: vectores de
-- modelos distintos no se comparan, asi que el modelo es parte de la clave y la
-- columna no fija dimension. Sin indice ANN: son unos cientos de filas y el
-- recorrido secuencial es exacto; un HNSW exige dimension fija.
--
-- Requiere la extension pgvector: imagen pgvector/pgvector:0.8.1-pg16 en compose y
-- en testhelp; en RDS PostgreSQL 16 la crea el usuario maestro (rds_superuser).
--
-- Es un indice derivado de docs/reglamentos/: cmd/indexadorreglamento lo
-- reemplaza entero. Sin dinero ni asientos (ADR 0006 no aplica).
--
-- COORDINACION DE NUMERO: 00029, primero libre sobre 00028 en main al abrir la
-- rama. Si otra rama lo toma antes, pasar al primero libre (allowMissing=false).

-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE reglamento_secciones (
  modelo TEXT NOT NULL CHECK (modelo <> ''),
  cita TEXT NOT NULL CHECK (cita <> ''),
  reglamento TEXT NOT NULL,
  titulo TEXT NOT NULL,
  texto TEXT NOT NULL,
  embedding vector NOT NULL CHECK (vector_dims(embedding) > 0),
  indexado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (modelo, cita)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS reglamento_secciones;
-- +goose StatementEnd
