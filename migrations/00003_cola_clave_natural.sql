-- La cola de trabajos gana clave natural y espera de reintento.
--
-- `cola_trabajos` nacio en 00001 con lo justo para que un worker tomara filas:
-- tipo, payload, estado, error, intentos. Le faltan las dos cosas que hacen
-- que una corrida no se repita y que un fallo no se pierda.
--
-- 1. CLAVE NATURAL (tipo, periodo, corrida). Sin ella, encolar dos veces el
--    reparto de 2026 crea dos filas, dos workers las toman y el periodo se
--    paga dos veces. Con ella, el segundo INSERT choca contra la restriccion y
--    el adaptador lo convierte en un no-op.
--
--    `corrida` es lo que el issue #35 llama "attempt": 1 es la corrida
--    original del periodo y >1 una corrida de AJUSTE, que es la unica forma
--    legitima de volver sobre un periodo ya repartido (RD 14.5.10 a 14.5.12
--    ajustan con una liquidacion nueva, no reabriendo la anterior). Se llama
--    asi y no `intento` para no confundirla con `intentos`, que cuenta
--    reintentos tecnicos del mismo trabajo y no tiene nada que ver.
--
-- 2. `disponible_en`. Un trabajo que falla vuelve a `pendiente` con la fecha
--    movida hacia adelante; sin columna donde escribir esa fecha, el worker
--    lo vuelve a tomar en el tic siguiente y gira en vacio hasta agotar los
--    intentos en unos segundos. La espera exponencial la decide el nucleo
--    (aplicacion.Reintentos); aqui solo se guarda el instante.
--
-- Migracion aparte y no edicion de 00001: 00001 ya esta aplicada en cualquier
-- entorno que exista, y goose lleva tabla de versiones. Editarla en sitio
-- deja las bases nuevas y las viejas con esquemas distintos, que es
-- exactamente el escenario que el ADR 0008 nombra -una corrida en vuelo
-- encontrandose un esquema que no conoce-.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE cola_trabajos
  ADD COLUMN periodo       TEXT        NOT NULL DEFAULT '0000',
  ADD COLUMN corrida       INT         NOT NULL DEFAULT 1,
  ADD COLUMN disponible_en TIMESTAMPTZ NOT NULL DEFAULT now();

-- El DEFAULT de `periodo` no es cosmetico: es el BACKFILL de las filas que ya
-- existieran. '0000' es un centinela, no un periodo -significa "trabajo legado,
-- de antes de que `periodo` existiera"-. Un ano de cuatro digitos que nunca va
-- a ser un periodo de reparto real, asi que no se confunde con una instruccion
-- de pago, que es en lo que se convertiria derivarlo de `creado`. Y a la vez SI
-- casa con el patron del CHECK de mas abajo, que es lo que mantiene la fila
-- legada actualizable. Por que eso importa tanto, en el bloque siguiente.
--
-- El DEFAULT se quita en cuanto el backfill esta hecho. A partir de aqui el
-- periodo lo pone quien encola, y una fila nueva que se olvide de ponerlo falla
-- con 23502 en vez de heredar el centinela en silencio.
ALTER TABLE cola_trabajos ALTER COLUMN periodo DROP DEFAULT;

-- `cola_periodo_valido` entra VALIDADA, sin NOT VALID. Merece explicacion
-- porque el intento anterior hizo lo contrario y eso costo caro.
--
-- Aquel razonamiento era: el DEFAULT dejaba `periodo = ''` en las filas ya
-- escritas, '' no casa con el patron, y una constraint normal escanea la tabla
-- al crearse; sobre una base con UNA sola fila en la cola la migracion moria
-- con 23514. NOT VALID se salta ese escaneo, y la migracion pasaba en verde.
--
-- Lo que NOT VALID no hace es eximir a esas filas para siempre. Exime SOLO la
-- validacion retroactiva del momento de crear la constraint. PostgreSQL
-- comprueba un CHECK contra cada version NUEVA de la fila, asi que desde ese
-- ALTER cualquier UPDATE sobre una fila legada falla con 23514, sin importar
-- que columnas toque el UPDATE ni en que estado este la fila: hasta un
-- `UPDATE ... SET id = id` sobre una fila ya cerrada en `hecho` falla. Solo el
-- DELETE se escapa.
--
-- El defecto que se cerraba asi era mas barato que el que se abria. Una
-- migracion que muere es ruidosa y se atiende en el despliegue; una cola que
-- acepta la migracion y despues se niega a avanzar no avisa a nadie. `Tomar`
-- es un UPDATE que ordena por (disponible_en, id), y una fila legada lleva el
-- disponible_en mas viejo de la tabla: el worker la elige siempre, el UPDATE
-- revienta, la transaccion revierte y la fila se queda pendiente con los
-- mismos intentos. La cola entera bloqueada, en silencio, de forma permanente.
-- Y el alcance no se agotaba en `Tomar`: cualquier codigo futuro que
-- actualizara una fila legada -un archivado, una limpieza, una metrica- fallaba
-- igual, tambien sobre filas terminales en `hecho` o `fallido`.
--
-- Con el centinela del backfill de arriba nada de eso hace falta: todas las
-- filas, legadas y nuevas, cumplen el patron. La constraint se crea validada de
-- una vez, y su escaneo retroactivo pasa a ser la red de seguridad -si alguna
-- fila se hubiera quedado sin centinela, la migracion falla aqui, en el
-- despliegue y dentro de la transaccion, en vez de envenenar la fila-.
--
-- Aviso operativo sobre `cola_clave_natural`: una UNIQUE no admite NOT VALID, y
-- el centinela no cambia esa conclusion. Si al aplicar esta migracion hubiera
-- DOS filas anteriores del mismo `tipo`, las dos quedan en (tipo, '0000', 1) y
-- la creacion del indice falla con 23505. Ese sigue siendo el unico caso que
-- exige drenar la cola antes de desplegar. Falla en transaccion y con un
-- mensaje que nombra la clave duplicada, asi que no deja la base a medias.
ALTER TABLE cola_trabajos
  ADD CONSTRAINT cola_periodo_valido
    CHECK (periodo ~ '^[0-9]{4}(-[0-9]{2})?$'),
  ADD CONSTRAINT cola_corrida_positiva
    CHECK (corrida >= 1),
  ADD CONSTRAINT cola_clave_natural
    UNIQUE (tipo, periodo, corrida);
-- +goose StatementEnd

-- El indice de 00001 era (id) WHERE estado = 'pendiente'. La consulta de
-- `Tomar` filtra ademas por disponible_en y ordena por (disponible_en, id),
-- asi que el indice tiene que llevar las dos columnas en ese orden o el plan
-- vuelve a ordenar en memoria.
-- +goose StatementBegin
DROP INDEX cola_pendientes;
CREATE INDEX cola_pendientes
  ON cola_trabajos (disponible_en, id)
  WHERE estado = 'pendiente';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX cola_pendientes;
CREATE INDEX cola_pendientes ON cola_trabajos (id) WHERE estado = 'pendiente';

ALTER TABLE cola_trabajos
  DROP CONSTRAINT cola_clave_natural,
  DROP CONSTRAINT cola_corrida_positiva,
  DROP CONSTRAINT cola_periodo_valido,
  DROP COLUMN disponible_en,
  DROP COLUMN corrida,
  DROP COLUMN periodo;
-- +goose StatementEnd
