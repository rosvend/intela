-- Una fila fuera de repertorio no es una obra no identificada.
--
-- Criterio 4 de #28 (R-27, RD 9.5): un uso de una fuente fuera de repertorio se
-- excluye antes de la cascada y NO se marca ONI. Hasta aqui el esquema no tenia
-- donde decirlo: la ingesta siembra escalon='pendiente' con oni=TRUE, y como
-- `oni_publico` filtra `WHERE u.oni`, la fila excluida salia en el listado
-- publico de obras no identificadas (R-18, RD 13.8.1) igual que una que de
-- verdad no se pudo reconocer.
--
-- Se anade 'excluido' al vocabulario de `escalon` y se abre en
-- `uso_resuelto_tiene_obra` una rama para ese caso y solo ese: excluida es
-- SIN obra y SIN ONI a la vez. El resto de escalones sigue atado a la regla de
-- siempre (oni <=> sin obra). `manual_tiene_autor` no cambia.
--
-- La vista `oni_publico` y los indices parciales `usos_pendientes` y `usos_oni`
-- no se tocan: con oni=FALSE y escalon<>'pendiente' la fila excluida ya queda
-- fuera de los tres.
--
-- NUMERO: 00007, el primero libre por encima de la version que produccion ya
-- aplico (00006). Renumerado desde 00010 en la review de esta PR.
--
-- 00010 seguia la LETRA de la cabecera de 00006 -"por encima del mayor que
-- exista"-, pero esa regla solo protege a quien la aplica. Esta es la primera
-- de las cinco PRs con migracion en llegar a `main`, y al desplegarse deja
-- produccion en la version que diga este nombre. Con 00010, los 00007 de #106
-- y #80, el 00008 de #87 y el 00009 de #88 quedan POR DEBAJO de la version
-- aplicada, y `goose` -allowMissing = false- los rechaza con
--
--     found N missing migrations before current version 10
--
-- que es exactamente el bloqueo de despliegue que describe 00006, solo que
-- infligido a otras cuatro PRs a la vez. Con 00007 produccion queda en 7, y el
-- 00008 de #87 y el 00009 de #88 siguen siendo validos sin tocar nada.
--
-- La regla utilizable no es "por encima del mayor que exista" sino "el primero
-- libre por encima de la version APLICADA, y se reasigna al mergear". #106 y
-- #80, que tambien reclaman 00007, tendran que subirlo: dos archivos con
-- nombres distintos y la misma version no dan conflicto en git, y hacen que
-- `goose` entre en panic con "duplicate version 7" en pleno despliegue.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE usos DROP CONSTRAINT usos_escalon_check;
ALTER TABLE usos ADD CONSTRAINT usos_escalon_check
  CHECK (escalon IN ('pendiente','alias','id_global','difuso','manual','oni','excluido'));

ALTER TABLE usos DROP CONSTRAINT uso_resuelto_tiene_obra;
ALTER TABLE usos ADD CONSTRAINT uso_resuelto_tiene_obra
  CHECK (
    (escalon = 'excluido' AND NOT oni AND obra_id IS NULL)
    OR (escalon <> 'excluido'
        AND ((oni AND obra_id IS NULL) OR (NOT oni AND obra_id IS NOT NULL)))
  );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Sin 'excluido' en el vocabulario, una fila excluida vuelve a como la dejaba
-- la cascada antes de esta migracion: pendiente y en ONI, sin tocar.
UPDATE usos SET escalon = 'pendiente', oni = TRUE, evidencia = '', puntaje = 0
 WHERE escalon = 'excluido';

ALTER TABLE usos DROP CONSTRAINT uso_resuelto_tiene_obra;
ALTER TABLE usos ADD CONSTRAINT uso_resuelto_tiene_obra
  CHECK ((oni AND obra_id IS NULL) OR (NOT oni AND obra_id IS NOT NULL));

ALTER TABLE usos DROP CONSTRAINT usos_escalon_check;
ALTER TABLE usos ADD CONSTRAINT usos_escalon_check
  CHECK (escalon IN ('pendiente','alias','id_global','difuso','manual','oni'));
-- +goose StatementEnd
