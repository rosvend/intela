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
-- NUMERO: 00010, por encima del mayor reclamado en `main` y en las ramas
-- abiertas el 2026-09-12 (ver la cabecera de 00006: nunca un hueco).

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
