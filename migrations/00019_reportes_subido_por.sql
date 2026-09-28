-- Quien subio cada reporte (issue #116).
--
-- De una entrega quedaba la fuente, el periodo y la huella del archivo, pero
-- no quien la hizo llegar. La escritura que decide como se pondera la bolsa de
-- un periodo ENTERO era la unica del sistema sin atribucion, mientras que
-- `usos.resuelto_por` ya registraba el actor de una resolucion manual, que es
-- una accion menos consecuente. Sin esta columna la cadena de trazabilidad del
-- ADR 0006 se corta en "que archivo" y no llega a "quien lo entrego".
--
-- Nullable y SIN relleno, a proposito: las filas anteriores a esta migracion
-- no tienen actor conocido y no se les inventa uno. NULL significa "anterior a
-- la atribucion", que es un hecho cierto y verificable; un backfill con
-- cualquier usuario del padron seria una atribucion falsa en la tabla que la
-- auditoria revisa (RD 16), y una atribucion falsa es peor que su ausencia
-- declarada. La misma distincion vale para las entregas del sembrador, que no
-- tienen a nadie detras.
--
-- La FK a usuarios(id) es la misma forma que usos.resuelto_por: lo que se
-- guarda es el id del usuario autenticado, no un nombre ni un correo, que
-- pueden cambiar sin que cambie quien hizo la entrega.
--
-- Numero: 00019, primero libre por encima de 00018. goose corre con
-- allowMissing = false; un numero libre por debajo de la version ya aplicada
-- aborta el despliegue. El hueco del 00004 no se toca: lo explica la cabecera
-- del 00006.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE reportes
  ADD COLUMN subido_por TEXT REFERENCES usuarios(id);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE reportes DROP COLUMN IF EXISTS subido_por;
-- +goose StatementEnd
