-- Solicitudes de afiliacion.
--
-- El padron de cobro sigue siendo `titulares`. Esta tabla es el flujo de
-- admision de RS 5.2 / RS 5.3: el Consejo Directivo estudia cada alta y
-- solo entonces se crea la fila de titular.
--
-- RUT y certificacion bancaria (R-12) son NOT NULL porque el asistente
-- existe para no dejarlos para despues. La exclusividad (R-28) se expresa
-- como CHECK: si declara pertenecer a otra SGC, tiene que haber evidencia
-- de renuncia, no solo el booleano.
--
-- COORDINACION DE NUMERO: esta migracion se llamaba 00003. Ese numero ya no
-- se puede usar. El PR #85 dejo produccion en la version 5; #72 entro como
-- 00006, #80 (`liquidacion`) toma el 00007 y #87 (`oni_publicacion`) el
-- 00008. goose corre con `allowMissing = false`, asi que un numero LIBRE
-- por debajo de la version ya aplicada aborta con
--
--     found 1 missing migrations before current version N
--
-- y el despliegue condiciona el rollout a que goose termine bien.
--
-- Se toma el 00009: el 2026-09-07 el mayor en `main` y en TODAS las ramas
-- abiertas era 00008. Un ADR admite huecos; una migracion no.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE afiliaciones (
  id                   TEXT PRIMARY KEY,
  nombre               TEXT NOT NULL CHECK (btrim(nombre) <> ''),
  email                TEXT NOT NULL CHECK (email LIKE '%@%'),
  documento_identidad  TEXT NOT NULL CHECK (btrim(documento_identidad) <> ''),
  ipi                  TEXT NOT NULL DEFAULT '',
  subtipo              TEXT NOT NULL CHECK (subtipo IN ('socio', 'administrado')),
  estado               TEXT NOT NULL CHECK (estado IN ('pendiente', 'admitido', 'rechazado')),
  persona_natural      BOOLEAN NOT NULL DEFAULT TRUE,
  pertenece_otra_sgc   BOOLEAN NOT NULL DEFAULT FALSE,
  clave_rut            TEXT NOT NULL CHECK (btrim(clave_rut) <> ''),
  clave_cert_bancaria  TEXT NOT NULL CHECK (btrim(clave_cert_bancaria) <> ''),
  clave_renuncia       TEXT NOT NULL DEFAULT '',
  titular_id           TEXT REFERENCES titulares(id),
  solicitado           TIMESTAMPTZ NOT NULL DEFAULT now(),
  resuelto             TIMESTAMPTZ,

  CONSTRAINT exclusividad_con_evidencia
    CHECK (NOT pertenece_otra_sgc OR btrim(clave_renuncia) <> ''),
  CONSTRAINT admitido_tiene_titular
    CHECK (estado <> 'admitido' OR titular_id IS NOT NULL)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Una solicitud activa (pendiente o ya admitida) no puede repetir el correo.
-- Una rechazada si: el aspirante puede volver a presentarse.
CREATE UNIQUE INDEX afiliaciones_email_activa
  ON afiliaciones (email)
  WHERE estado IN ('pendiente', 'admitido');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS afiliaciones;
-- +goose StatementEnd
