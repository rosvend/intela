-- Resolver una anomalia critica corrige el dato (#164, ADR 0021).
--
-- Hasta aqui `POST /alertas/{id}/resolver` exigia una nota y cerraba la
-- alerta, pero no tocaba nada de lo que pondera el reparto: un duplicado
-- resuelto seguia contando dos veces y un `tipo_obra` vacio seguia abortando
-- el motor. Esta migracion da donde escribir la correccion.
--
-- 1. 'duplicado' en usos.escalon
-- --------------------------------
--
-- Una persona decidio que la fila repite un hecho que ya cuenta otra fila (o
-- que su entrega entera repite los bytes de otra). La fila queda SIN obra y
-- con oni = FALSE, igual que 'excluido' y 'descartado': es la forma en que
-- todo lector de `usos` que filtra `obra_id IS NOT NULL` -- UsosDeCanal, el
-- detalle de ingresos -- la deja fuera sin cambiar una sola consulta. La obra
-- que tenia no se pierde: queda escrita en `evidencia` y en el asiento de la
-- correccion.
--
-- NO se reusa 'excluido', por el mismo motivo que 00022 no lo reuso para
-- 'descartado': `ResolverUsos` reprocesa las filas excluidas en CADA corrida,
-- porque la exclusion R-27 es configuracion. La fila duplicada volveria a la
-- cascada, se identificaria otra vez y el doble conteo reapareceria solo.
-- 'duplicado' no es reprocesable: es una decision humana, firmada y con nota,
-- y por eso entra en `manual_tiene_autor` y `resolucion_manual_tiene_nota`.
--
-- 2. reportes.excluida_por / excluida_en
-- ---------------------------------------
--
-- Excluir una entrega entera (`duplicado_archivo`) marca sus filas como
-- 'duplicado' y ademas deja la marca en la entrega: el detector de huella la
-- deja de contar como pata de una colision, y la alerta de la OTRA entrega
-- del par se autocierra en la siguiente pasada en vez de pedir una segunda
-- exclusion. Las dos columnas van juntas o ninguna.
--
-- 3. alertas.accion / accion_objetivo / resuelta_rol
-- ---------------------------------------------------
--
-- La correccion aplicada queda en la propia alerta, con el registro sobre el
-- que actuo (una fila, una entrega o el tipo asignado) y el rol de quien la
-- firmo. Solo una alerta cerrada por una PERSONA lleva accion; el autocierre
-- del sistema no corrige nada.
--
-- `aceptar_tal_cual` es la salida explicita para un falso positivo de un
-- detector de duplicados (P-21: la clave de cine es provisional): no corrige
-- el dato, la compuerta la deja pasar y la cuenta aparte en el asiento de la
-- transicion. Las criticas que una persona cerro ANTES de esta migracion se
-- marcan asi, porque eso es lo que significaba "resuelta" entonces (ADR 0021,
-- "Que significa resuelta para el dinero"): nadie corrigio el dato.
--
-- COORDINACION DE NUMERO: 00024 es el primero libre por encima de lo aplicado
-- en `main` (`00023_oni_publicacion.sql`). Dos ficheros con la misma version
-- no chocan en git y hacen que goose entre en panic con "duplicate version";
-- quien mergee segundo renumera. La guarda
-- `internal/infraestructura/migraciones/numeracion` lo comprueba en CI.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE usos DROP CONSTRAINT usos_escalon_check;
ALTER TABLE usos ADD CONSTRAINT usos_escalon_check
  CHECK (escalon IN ('pendiente','alias','id_global','difuso','manual','oni',
                     'excluido','descartado','duplicado'));

ALTER TABLE usos DROP CONSTRAINT uso_resuelto_tiene_obra;
ALTER TABLE usos ADD CONSTRAINT uso_resuelto_tiene_obra
  CHECK (
    (escalon IN ('excluido','descartado','duplicado') AND NOT oni AND obra_id IS NULL)
    OR (escalon NOT IN ('excluido','descartado','duplicado')
        AND ((oni AND obra_id IS NULL) OR (NOT oni AND obra_id IS NOT NULL)))
  );

ALTER TABLE usos DROP CONSTRAINT manual_tiene_autor;
ALTER TABLE usos ADD CONSTRAINT manual_tiene_autor
  CHECK (escalon NOT IN ('manual','descartado','duplicado')
         OR (resuelto_por IS NOT NULL AND resuelto_en IS NOT NULL));

ALTER TABLE usos DROP CONSTRAINT resolucion_manual_tiene_nota;
ALTER TABLE usos ADD CONSTRAINT resolucion_manual_tiene_nota
  CHECK (escalon NOT IN ('manual','descartado','duplicado') OR btrim(nota_resolucion) <> '');

ALTER TABLE reportes
  ADD COLUMN excluida_por TEXT REFERENCES usuarios(id),
  ADD COLUMN excluida_en  TIMESTAMPTZ,
  ADD CONSTRAINT reporte_exclusion_firmada
    CHECK ((excluida_por IS NULL) = (excluida_en IS NULL));

ALTER TABLE alertas
  ADD COLUMN accion          TEXT NOT NULL DEFAULT '',
  ADD COLUMN accion_objetivo TEXT NOT NULL DEFAULT '',
  ADD COLUMN resuelta_rol    TEXT NOT NULL DEFAULT '',
  ADD CONSTRAINT alerta_accion_valida
    CHECK (accion IN ('', 'excluir_uso', 'excluir_entrega', 'asignar_tipo_obra', 'aceptar_tal_cual')),
  -- Solo la cierra con accion una persona: ni una abierta ni una autocerrada corrigieron nada.
  ADD CONSTRAINT alerta_accion_solo_de_persona
    CHECK (accion = '' OR (resuelta AND NOT autocerrada)),
  -- Una correccion nombra el registro sobre el que actuo; aceptar no actua sobre ninguno.
  ADD CONSTRAINT alerta_accion_tiene_objetivo
    CHECK ((accion IN ('', 'aceptar_tal_cual')) = (btrim(accion_objetivo) = '')),
  ADD CONSTRAINT alerta_rol_solo_de_persona
    CHECK (resuelta_rol = '' OR (resuelta AND NOT autocerrada));

-- Las criticas cerradas por una persona antes de #164 no corrigieron el dato.
UPDATE alertas
   SET accion = 'aceptar_tal_cual'
 WHERE resuelta AND NOT autocerrada
   AND tipo IN ('duplicado_archivo', 'duplicado_registro', 'tipo_obra_sin_mapear');
-- +goose StatementEnd

-- Lo que la compuerta cuenta aparte: criticas aceptadas tal cual por periodo.
-- +goose StatementBegin
CREATE INDEX alertas_accion ON alertas (periodo, accion) WHERE accion <> '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS alertas_accion;

ALTER TABLE alertas
  DROP CONSTRAINT alerta_rol_solo_de_persona,
  DROP CONSTRAINT alerta_accion_tiene_objetivo,
  DROP CONSTRAINT alerta_accion_solo_de_persona,
  DROP CONSTRAINT alerta_accion_valida,
  DROP COLUMN resuelta_rol,
  DROP COLUMN accion_objetivo,
  DROP COLUMN accion;

ALTER TABLE reportes
  DROP CONSTRAINT reporte_exclusion_firmada,
  DROP COLUMN excluida_en,
  DROP COLUMN excluida_por;

-- Sin 'duplicado' en el vocabulario, la fila vuelve a pendiente para que la
-- cascada la mire otra vez, igual que el Down de 00007 con 'excluido'.
UPDATE usos SET escalon = 'pendiente', oni = TRUE, resuelto_por = NULL, resuelto_en = NULL,
                nota_resolucion = '', puntaje = 0
 WHERE escalon = 'duplicado';

ALTER TABLE usos DROP CONSTRAINT resolucion_manual_tiene_nota;
ALTER TABLE usos ADD CONSTRAINT resolucion_manual_tiene_nota
  CHECK (escalon NOT IN ('manual','descartado') OR btrim(nota_resolucion) <> '');

ALTER TABLE usos DROP CONSTRAINT manual_tiene_autor;
ALTER TABLE usos ADD CONSTRAINT manual_tiene_autor
  CHECK (escalon NOT IN ('manual','descartado')
         OR (resuelto_por IS NOT NULL AND resuelto_en IS NOT NULL));

ALTER TABLE usos DROP CONSTRAINT uso_resuelto_tiene_obra;
ALTER TABLE usos ADD CONSTRAINT uso_resuelto_tiene_obra
  CHECK (
    (escalon IN ('excluido','descartado') AND NOT oni AND obra_id IS NULL)
    OR (escalon NOT IN ('excluido','descartado')
        AND ((oni AND obra_id IS NULL) OR (NOT oni AND obra_id IS NOT NULL)))
  );

ALTER TABLE usos DROP CONSTRAINT usos_escalon_check;
ALTER TABLE usos ADD CONSTRAINT usos_escalon_check
  CHECK (escalon IN ('pendiente','alias','id_global','difuso','manual','oni','excluido','descartado'));
-- +goose StatementEnd
