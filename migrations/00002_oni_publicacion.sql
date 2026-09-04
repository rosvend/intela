-- Publicacion del listado ONI (R-18, RD 13.8.1 a 13.8.4) y ancla de
-- prescripcion (R-19, RD 13.8.7).
--
-- La vista oni_publico de 00001 leia la cola viva. Eso no es un listado
-- publicado: resolver un ONI despues lo haria desaparecer, y no habia fecha
-- de proceso ni direcciones. Esta migracion congela la instantanea y redefine
-- la vista sobre lo publicado. Sigue sin una sola columna de dinero.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE usos
  ADD COLUMN publicado_en TIMESTAMPTZ;
-- +goose StatementEnd

-- El ancla de R-19 se escribe una sola vez. Actualizar otras columnas
-- (resolver el ONI a mano) sigue permitido; cambiar la fecha no.
-- +goose StatementBegin
CREATE FUNCTION oni_ancla_inmutable() RETURNS TRIGGER AS $fn$
BEGIN
  IF OLD.publicado_en IS NOT NULL
     AND NEW.publicado_en IS DISTINCT FROM OLD.publicado_en THEN
    RAISE EXCEPTION
      'R-19 (RD 13.8.7): el ancla de prescripcion de % no se puede reescribir',
      OLD.id;
  END IF;
  RETURN NEW;
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER usos_ancla_prescripcion
  BEFORE UPDATE ON usos
  FOR EACH ROW EXECUTE FUNCTION oni_ancla_inmutable();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE oni_publicaciones (
  id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  periodo                TEXT NOT NULL CHECK (periodo ~ '^[0-9]{4}(-[0-9]{2})?$'),
  fecha_proceso          TIMESTAMPTZ NOT NULL,
  direccion_fisica       TEXT NOT NULL CHECK (btrim(direccion_fisica) <> ''),
  direccion_electronica  TEXT NOT NULL CHECK (btrim(direccion_electronica) <> ''),
  -- Un periodo, una publicacion. Republicar reescribiria el ancla de R-19.
  UNIQUE (periodo)
);
CREATE INDEX oni_publicaciones_fecha ON oni_publicaciones (fecha_proceso DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION oni_publicacion_inmutable() RETURNS TRIGGER AS $fn$
BEGIN
  RAISE EXCEPTION
    'R-18/R-19: el listado publicado es inmutable. % sobre % no esta permitido',
    TG_OP, TG_TABLE_NAME;
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER oni_publicaciones_inmutables
  BEFORE UPDATE OR DELETE ON oni_publicaciones
  FOR EACH ROW EXECUTE FUNCTION oni_publicacion_inmutable();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER oni_publicaciones_sin_truncate
  BEFORE TRUNCATE ON oni_publicaciones
  FOR EACH STATEMENT EXECUTE FUNCTION oni_publicacion_inmutable();
-- +goose StatementEnd

-- Instantanea identificatoria. Los campos se copian (no se joinea usos) para
-- que resolver o borrar un uso despues no cambie lo que se publico.
-- +goose StatementBegin
CREATE TABLE oni_publicacion_items (
  publicacion_id UUID NOT NULL REFERENCES oni_publicaciones(id),
  uso_id         TEXT NOT NULL REFERENCES usos(id),
  titulo         TEXT NOT NULL CHECK (btrim(titulo) <> ''),
  fuente         TEXT NOT NULL,
  ids_fuente     TEXT NOT NULL DEFAULT '',
  modalidad      TEXT NOT NULL CHECK (modalidad IN ('tv','cine','ott','hotel')),
  PRIMARY KEY (publicacion_id, uso_id)
);
CREATE INDEX oni_publicacion_items_uso ON oni_publicacion_items (uso_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER oni_publicacion_items_inmutables
  BEFORE UPDATE OR DELETE ON oni_publicacion_items
  FOR EACH ROW EXECUTE FUNCTION oni_publicacion_inmutable();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER oni_publicacion_items_sin_truncate
  BEFORE TRUNCATE ON oni_publicacion_items
  FOR EACH STATEMENT EXECUTE FUNCTION oni_publicacion_inmutable();
-- +goose StatementEnd

-- +goose StatementBegin
DROP VIEW IF EXISTS oni_publico;
-- +goose StatementEnd

-- Misma garantia que 00001: titulo e identificadores, NUNCA montos. Ahora
-- ademas fecha, periodo y las dos direcciones que exige RD 13.8.4.
-- +goose StatementBegin
CREATE VIEW oni_publico AS
  SELECT
    i.uso_id                AS id,
    i.titulo,
    i.fuente,
    i.ids_fuente,
    i.modalidad,
    p.periodo,
    p.fecha_proceso,
    p.direccion_fisica,
    p.direccion_electronica
  FROM oni_publicacion_items i
  JOIN oni_publicaciones p ON p.id = i.publicacion_id;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP VIEW IF EXISTS oni_publico;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS oni_publicacion_items_sin_truncate ON oni_publicacion_items;
DROP TRIGGER IF EXISTS oni_publicacion_items_inmutables ON oni_publicacion_items;
DROP TRIGGER IF EXISTS oni_publicaciones_sin_truncate ON oni_publicaciones;
DROP TRIGGER IF EXISTS oni_publicaciones_inmutables ON oni_publicaciones;
DROP TRIGGER IF EXISTS usos_ancla_prescripcion ON usos;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS oni_publicacion_items;
DROP TABLE IF EXISTS oni_publicaciones;
-- +goose StatementEnd

-- +goose StatementBegin
DROP FUNCTION IF EXISTS oni_publicacion_inmutable();
DROP FUNCTION IF EXISTS oni_ancla_inmutable();
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE usos DROP COLUMN IF EXISTS publicado_en;
-- +goose StatementEnd

-- Restaura la vista viva de 00001 para que el down de esa migracion la
-- encuentre.
-- +goose StatementBegin
CREATE VIEW oni_publico AS
  SELECT u.id, u.titulo, u.fuente, u.ids_fuente, u.modalidad, r.periodo
  FROM usos u
  JOIN reportes r ON r.id = u.reporte_id
  WHERE u.oni;
-- +goose StatementEnd
