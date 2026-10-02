-- Ejemplos etiquetados de la cola manual (#53).
--
-- Cada resolucion de una persona (asignar o descartar) queda como un ejemplo:
-- las candidatas que vio, el titulo normalizado con el que se agrupa, la
-- decision y si coincidio con lo que el rankeador habia sugerido.
--
-- No hay camino de auto-resolucion en esta tabla. La sugerencia se guarda para
-- medirla (aceptada o cambiada). Quien escribe el escalon del uso sigue siendo
-- la resolucion manual de #175, y solo cuando una persona lo pide.
--
-- Sin columnas de dinero: identificar no toca dinero (ADR 0007).
--
-- COORDINACION DE NUMERO: este archivo nacio como 00024, pero 00024 ya lo
-- tomo 00024_resultados_titular_declaracion_version.sql (#198), que entro a
-- main antes. Numero: 00025, primero libre por encima de 00024. Nunca un hueco
-- por debajo de la version ya aplicada (goose allowMissing=false; ver 00006).
-- #195 y #197 tambien pedian 00024 y siguen abiertos; si alguno entra antes
-- de este, hay que volver a tomar el primero libre por encima del ultimo en
-- main al momento de mergear.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE ejemplos_resolucion (
  uso_id TEXT PRIMARY KEY REFERENCES usos(id) ON DELETE CASCADE,
  clave TEXT NOT NULL,
  fuente TEXT NOT NULL,
  candidatos JSONB NOT NULL,
  decision TEXT NOT NULL CHECK (decision IN ('asignar', 'descartar')),
  obra_elegida TEXT REFERENCES obras(id),
  sugerencia_decision TEXT NOT NULL CHECK (sugerencia_decision IN ('asignar', 'descartar', 'ninguna')),
  sugerencia_obra_id TEXT REFERENCES obras(id),
  confianza NUMERIC NOT NULL CHECK (confianza >= 0 AND confianza <= 1),
  motivo TEXT NOT NULL,
  orden JSONB NOT NULL,
  aceptada BOOLEAN NOT NULL,
  actor_id TEXT NOT NULL REFERENCES usuarios(id),
  CHECK (
    (decision = 'asignar' AND obra_elegida IS NOT NULL)
    OR (decision = 'descartar' AND obra_elegida IS NULL)
  ),
  CHECK (sugerencia_decision <> 'asignar' OR sugerencia_obra_id IS NOT NULL)
);

CREATE INDEX ejemplos_resolucion_clave ON ejemplos_resolucion (clave);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS ejemplos_resolucion;
-- +goose StatementEnd
