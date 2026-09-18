-- Snapshots congelados de parametros normativos (#118, ADR 0004 y ADR 0005).
--
-- COORDINACION DE NUMERO: el mayor aplicado en main al escribir esto era
-- 00010. Se toma el siguiente libre. Nunca un hueco por debajo de la version
-- ya aplicada (goose allowMissing=false; ver 00006).
--
-- `parametros` guarda la HISTORIA de vigencias: que valor rige en cada fecha.
-- Esta tabla guarda el CORTE que una corrida consumio, y son dos cosas
-- distintas. Sin ella, recalcular volveria a resolver contra `parametros`, y
-- una fila de vigencia cargada despues cambiaria en silencio el resultado de
-- una corrida ya hecha -- que es justo lo que el ADR 0005 existe para impedir.
--
-- NO hay clave foranea hacia `parametros`, y es deliberado: la ataria a la
-- fila viva de la que salio. Lo congelado tiene que sobrevivir a que esa fila
-- se cierre, se corrija o desaparezca; si no, no esta congelado.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE snapshots_parametros (
  -- Direccionado por contenido: sha256 sobre los pares (clave, valor)
  -- ordenados. El id ES la suma de verificacion de lo que hay debajo, asi que
  -- resolver dos veces la misma fecha con los mismos valores da el mismo id, y
  -- dos conjuntos distintos no pueden compartirlo. El CHECK es la forma, no la
  -- correspondencia: que el hash CUADRE con las filas lo comprueba la lectura.
  snapshot_id   TEXT NOT NULL CHECK (snapshot_id ~ '^snp-[0-9a-f]{64}$'),
  clave         TEXT NOT NULL,
  valor         NUMERIC(18,6) NOT NULL,

  -- La procedencia viaja CON el valor congelado en vez de leerse de
  -- `parametros` al consultarlo. La pregunta 7 del ADR 0006 -quien aprobo cada
  -- cifra- tiene que seguir teniendo respuesta dentro de diez anos (RD 13.2,
  -- RD 13.4), aunque el organo haya cambiado de criterio tres veces desde
  -- entonces y la fila viva ya diga otra cosa.
  organo        TEXT NOT NULL CHECK (btrim(organo) <> ''),
  reglamento    TEXT NOT NULL CHECK (btrim(reglamento) <> ''),
  vigente_desde DATE NOT NULL,

  congelado     TIMESTAMPTZ NOT NULL DEFAULT now(),

  PRIMARY KEY (snapshot_id, clave)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION snapshot_congelado() RETURNS TRIGGER AS $fn$
BEGIN
  -- TG_TABLE_NAME y no un literal, por lo mismo que en bitacora_solo_append:
  -- un mensaje que nombre la tabla equivocada manda a quien lo lea a mirar
  -- donde no es.
  RAISE EXCEPTION
    'ADR 0005: un snapshot congelado no se reescribe. % sobre % no esta permitido. Un valor nuevo es un snapshot NUEVO, con otro id',
    TG_OP, TG_TABLE_NAME;
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Inmutable EXIGIDO POR EL MOTOR, no por convenio. Es la misma razon que en
-- `asientos`: la disciplina de no reescribir no puede depender de que nadie
-- escriba la sentencia. Un UPDATE aqui cambia una cifra ya pagada y la deja
-- pareciendo la que siempre fue.
-- +goose StatementBegin
CREATE TRIGGER snapshots_parametros_inmutables
  BEFORE UPDATE OR DELETE ON snapshots_parametros
  FOR EACH ROW EXECUTE FUNCTION snapshot_congelado();
-- +goose StatementEnd

-- Y TRUNCATE aparte, porque un trigger FOR EACH ROW no se dispara con el.
-- +goose StatementBegin
CREATE TRIGGER snapshots_parametros_sin_truncate
  BEFORE TRUNCATE ON snapshots_parametros
  FOR EACH STATEMENT EXECUTE FUNCTION snapshot_congelado();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS snapshots_parametros_sin_truncate ON snapshots_parametros;
DROP TRIGGER IF EXISTS snapshots_parametros_inmutables ON snapshots_parametros;
DROP TABLE IF EXISTS snapshots_parametros;
DROP FUNCTION IF EXISTS snapshot_congelado();
-- +goose StatementEnd
