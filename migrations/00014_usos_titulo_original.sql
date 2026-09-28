-- Titulo original de la fila de uso (#32, review de PR #146).
--
-- COORDINACION DE NUMERO: 00012 lo tomo #118 y 00013 es el escalon 3 de este
-- mismo PR. Numero: 00014, primero libre por encima de 00013. Nunca un hueco
-- por debajo de la version ya aplicada (goose allowMissing=false; ver 00006).
--
-- Caracol entrega `Titulo` (el emitido en Colombia) y `Titulo_original`, y
-- difieren en 16 de 59 filas. Hasta ahora solo se guardaba el primero, asi que
-- el escalon 3 no podia casar por el original: se perdia la mitad del recall
-- justo donde los catalogos de las fuentes no comparten ni idioma.
--
-- NOT NULL DEFAULT '': vacio = la fuente no lo trae, el mismo criterio que
-- `titulo`. `usos_rechazados` NO la lleva: una fila rechazada no se identifica,
-- y ese log guarda lo necesario para revisarla, no para casarla.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE usos ADD COLUMN titulo_original TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE usos DROP COLUMN IF EXISTS titulo_original;
-- +goose StatementEnd
