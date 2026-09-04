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
  ADD COLUMN periodo       TEXT        NOT NULL DEFAULT '',
  ADD COLUMN corrida       INT         NOT NULL DEFAULT 1,
  ADD COLUMN disponible_en TIMESTAMPTZ NOT NULL DEFAULT now();

-- El DEFAULT solo existe para poder anadir la columna como NOT NULL sobre
-- filas ya escritas. A partir de aqui el periodo lo pone quien encola: una
-- cadena vacia por descuido compartiria clave con cualquier otro trabajo sin
-- periodo del mismo tipo.
ALTER TABLE cola_trabajos ALTER COLUMN periodo DROP DEFAULT;

-- `cola_periodo_valido` entra NOT VALID a proposito.
--
-- El DEFAULT de arriba deja `periodo = ''` en las filas que ya existieran, y
-- '' no casa con el patron. Una constraint normal escanea la tabla entera al
-- crearse, asi que sobre una base con UNA sola fila en la cola la migracion
-- muere con 23514 y el despliegue se queda a medias. NOT VALID se salta ese
-- escaneo retroactivo, que es lo unico que sobra.
--
-- Lo que NO se debilita, que es la garantia que el resto del PR necesita:
-- NOT VALID exime solo a las filas que ya estaban. Desde este ALTER, todo
-- INSERT y todo UPDATE se comprueba, asi que el periodo de una fila NUEVA
-- tiene formato AAAA o AAAA-MM o la insercion falla.
--
-- Y no hay VALIDATE CONSTRAINT despues, a proposito: volveria a escanear esas
-- mismas filas y a fallar con el mismo 23514. Las filas anteriores conservan
-- `periodo = ''` de forma permanente. No se rellenan con un valor inventado
-- porque no existe ninguno correcto: `cola_trabajos` no tenia de donde
-- sacarlo, y derivarlo de `creado` fabricaria una instruccion de pago
-- plausible y falsa -el nucleo lee esta columna para saber que periodo
-- repartir-. '' no es un periodo y no puede confundirse con uno: significa
-- "fila anterior a la clave natural".
--
-- Aviso operativo sobre `cola_clave_natural`: una UNIQUE no admite NOT VALID.
-- Si al aplicar esta migracion hubiera DOS filas anteriores del mismo `tipo`,
-- las dos quedan en (tipo, '', 1) y la creacion del indice falla con 23505.
-- Eso si exige drenar la cola antes de desplegar. Falla en transaccion y con
-- un mensaje que nombra la clave duplicada, asi que no deja la base a medias.
ALTER TABLE cola_trabajos
  ADD CONSTRAINT cola_periodo_valido
    CHECK (periodo ~ '^[0-9]{4}(-[0-9]{2})?$') NOT VALID,
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
