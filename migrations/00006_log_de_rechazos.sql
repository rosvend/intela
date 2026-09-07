-- El log de rechazos de la ingesta.
--
-- COORDINACION DE NUMERO: esta migracion se llamaba 00004, y ese numero ya no
-- se puede usar. El PR #85 renumero la suya a 00005 y su despliegue corrio
-- `goose up` de verdad, asi que produccion esta en la version 5.
--
-- Un numero LIBRE por debajo de la version ya aplicada no es un hueco
-- inofensivo: `goose` corre con `allowMissing = false` -el runner llama a
-- RunContext sin opciones-, asi que se para en seco con
--
--     found 1 missing migrations before current version 5
--
-- antes de aplicar nada. Y el despliegue corre las migraciones ANTES de sacar
-- la API y condiciona el rollout a que terminen bien, asi que el hueco no
-- retrasa el esquema: bloquea el despliegue entero.
--
-- Se toma el 00006, que el 2026-09-07 estaba libre en `main` y en TODAS las
-- ramas abiertas del repo. No se toca el 00003, que siguen reclamando #80
-- (`liquidacion`), #87 (`oni_publicacion`) y #88 (`afiliaciones`): cual de las
-- tres se lo queda no lo decide esta PR -- aunque las tres tendran que subirlo
-- por encima del 00005 por este mismo motivo.
--
-- Para una migracion nueva el numero se toma SIEMPRE por encima del mayor que
-- exista, nunca en un hueco. El contenido no cambia con el renombre: solo
-- cambia la version que goose lee del nombre del archivo.
--
-- Una fila de reporte que no se puede normalizar NO se descarta: queda con su
-- motivo, y no pondera nada. Es criterio de aceptacion de OE-1 y de KR-1, y es
-- lo que permite volver a pedirle al cliente exactamente lo que falta.
--
-- Va en tabla APARTE de `usos` y no en una columna `rechazo_motivo` de `usos`.
-- El razonamiento completo esta en el ADR 0016; en corto, tres cosas que la
-- columna habria costado:
--
--   1. `usos` esta lleno de CHECK -modalidad en las cuatro, escalon en los
--      seis, uso_resuelto_tiene_obra, las medidas no negativas- y una fila
--      estructuralmente rota, por definicion, viola alguno. Meterla en `usos`
--      obliga a RELAJAR esos CHECK, y esos CHECK son lo unico que garantiza que
--      lo que hay en `usos` es canonico.
--   2. La vista `oni_publico` selecciona `WHERE u.oni`, y `oni` tiene
--      DEFAULT TRUE. Una fila rechazada en `usos` aparece en el listado publico
--      de obras no identificadas (R-18, RD 13.8.1) sin que nadie lo pida.
--   3. Excluir el rechazo de las lecturas canonicas pasaria a depender de que
--      cada consulta futura no olvide un `WHERE rechazo_motivo IS NULL`. Aqui
--      la exclusion es estructural: la fila no esta en la tabla.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE usos_rechazados (
  -- El mismo espacio de identificadores que `usos`: un id vive en una de las
  -- dos tablas, nunca en las dos. Asi se puede rastrear una linea de un archivo
  -- sin saber de antemano si llego a ser canonica.
  id         TEXT PRIMARY KEY,
  reporte_id TEXT NOT NULL REFERENCES reportes(id) ON DELETE CASCADE,

  -- Campos identificatorios, tal como vinieron y SIN NINGUN CHECK de dominio.
  -- Que aqui quepa modalidad = 'radio' es justamente el punto: esta tabla
  -- recibe lo que `usos` rechaza, y un CHECK aqui volveria a dejar la fila sin
  -- sitio donde caer.
  fuente     TEXT NOT NULL DEFAULT '',
  titulo     TEXT NOT NULL DEFAULT '',
  ids_fuente TEXT NOT NULL DEFAULT '',
  modalidad  TEXT NOT NULL DEFAULT '',

  -- NO SE COPIAN LAS COLUMNAS DE MEDIDA, y no es un olvido. Una fila rechazada
  -- no pondera; sin duracion, emisiones ni vistas aqui, no hay forma de que
  -- una consulta futura la sume "solo para ver". Se guarda para poder pedirle
  -- al cliente la linea que falta, no para calcular con ella.
  --
  -- Y tampoco hay columna de dinero, por la misma razon que en `usos`.

  -- Un rechazo sin motivo no es un rechazo, es una perdida. Mismo patron que
  -- resultados_obra.retenida_tiene_motivo: en este sistema lo que se aparta se
  -- aparta CON su razon.
  motivo     TEXT NOT NULL CHECK (btrim(motivo) <> ''),
  rechazado  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX usos_rechazados_reporte ON usos_rechazados (reporte_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS usos_rechazados;
-- +goose StatementEnd
