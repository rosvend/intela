-- El log de rechazos de la ingesta.
--
-- Una fila de reporte que no se puede normalizar NO se descarta: queda con su
-- motivo, y no pondera nada. Es criterio de aceptacion de OE-1 y de KR-1, y es
-- lo que permite volver a pedirle al cliente exactamente lo que falta.
--
-- Va en tabla APARTE de `usos` y no en una columna `rechazo_motivo` de `usos`.
-- El razonamiento completo esta en el ADR 0014; en corto, tres cosas que la
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
