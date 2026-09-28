-- Resolucion manual de un caso ONI (#175): asignar o descartar, firmado y con nota.
--
-- Que es 'descartado' y en que se diferencia de 'excluido' y de ONI
-- ------------------------------------------------------------------
--
-- El escalon nuevo registra la decision de una PERSONA sobre un caso que la
-- cascada dejo en ONI (RD 13.8): "esto no es un uso del repertorio de REDES
-- SGC". La fila queda SIN obra, con oni = FALSE y sin ponderar, igual que una
-- exclusion R-27 (RD 9.5): su parte no existe y la bolsa se reparte entre las
-- obras que si ponderan. RD 7.1 es la razon de fondo -- REDES solo representa
-- autores de guion o libreto --, y por eso el descartado no es el regimen ONI
-- de RD 13.8: no es una obra sin identificar, es una que no es del repertorio.
--
-- NO se reusa 'excluido' a proposito: `ResolverUsos` reprocesa las filas
-- excluidas en CADA corrida, porque la exclusion es configuracion
-- (FuentesExcluidas) y tiene que poder cambiar. Una decision humana guardada
-- ahi se desharia sola en la corrida siguiente. `reprocesable()` devuelve
-- false para 'manual' y para 'descartado': una decision humana no se pisa.
--
-- La vista `oni_publico` y los indices parciales `usos_pendientes` y `usos_oni`
-- no se tocan: con oni = FALSE y escalon <> 'pendiente' la fila descartada
-- queda fuera de los tres.
--
-- Por que manual_tiene_autor se extiende a 'descartado'
-- ----------------------------------------------------
--
-- Un descarte es una decision humana sobre a quien se le paga, y el ADR 0006
-- pide actor e instante para ese caso. `manual_tiene_autor` reservaba la firma
-- a 'manual' -la cola de resolucion de #39-; el descarte se firma igual, asi
-- que la misma restriccion cubre los dos.
--
-- La nota y su tope (D5)
-- ---------------------
--
-- `usos.nota_resolucion` es la justificacion auditable de la decision, el
-- mismo patron que `alertas.nota` junto al asiento `alerta.resuelta`
-- (ADR 0021). Es obligatoria -- no basta con la firma: hay que decir POR QUE -
-- y su tope son 300 caracteres, contados en runas por Postgres (`char_length`
-- cuenta caracteres, no bytes) y tras recortar los espacios de los extremos.
--
-- El relleno defensivo
-- -------------------
--
-- Ninguna ruta escribia 'manual' antes de esta migracion, asi que se esperan
-- cero filas. Pero el CHECK de nota rechazaria cualquiera que existiera y el
-- despliegue fallaria entero; el UPDATE de abajo las deja pasar diciendo, en
-- la propia nota, que no la escribio nadie.
--
-- COORDINACION DE NUMERO: 00022 es el primero libre por encima de la version
-- aplicada (00021). La PR #87 (listado publico ONI) trae su propia
-- `00022_oni_publicacion.sql`: dos archivos con la misma version no chocan en
-- git y hacen que goose entre en panic con "duplicate version 22" en pleno
-- despliegue. Quien mergee segundo renumera, como ya paso con 00007 y 00013.
-- La guarda `internal/infraestructura/migraciones/numeracion` es la que lo
-- comprueba en CI.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE usos ADD COLUMN nota_resolucion TEXT NOT NULL DEFAULT '';

-- Relleno defensivo: ninguna ruta escribia 'manual' antes de esta migracion, pero si alguna
-- fila lo tiene, el CHECK de nota la rechazaria y el despliegue fallaria.
UPDATE usos SET nota_resolucion = 'sin nota: resuelto antes de la migracion 00022'
 WHERE escalon = 'manual' AND btrim(nota_resolucion) = '';

ALTER TABLE usos DROP CONSTRAINT usos_escalon_check;
ALTER TABLE usos ADD CONSTRAINT usos_escalon_check
  CHECK (escalon IN ('pendiente','alias','id_global','difuso','manual','oni','excluido','descartado'));

-- La rama de 'excluido' de 00007 se abre a 'descartado': los dos son "sin obra
-- y sin ONI", y siguen siendo los dos unicos.
ALTER TABLE usos DROP CONSTRAINT uso_resuelto_tiene_obra;
ALTER TABLE usos ADD CONSTRAINT uso_resuelto_tiene_obra
  CHECK (
    (escalon IN ('excluido','descartado') AND NOT oni AND obra_id IS NULL)
    OR (escalon NOT IN ('excluido','descartado')
        AND ((oni AND obra_id IS NULL) OR (NOT oni AND obra_id IS NOT NULL)))
  );

ALTER TABLE usos DROP CONSTRAINT manual_tiene_autor;
ALTER TABLE usos ADD CONSTRAINT manual_tiene_autor
  CHECK (escalon NOT IN ('manual','descartado')
         OR (resuelto_por IS NOT NULL AND resuelto_en IS NOT NULL));

ALTER TABLE usos ADD CONSTRAINT resolucion_manual_tiene_nota
  CHECK (escalon NOT IN ('manual','descartado') OR btrim(nota_resolucion) <> '');
ALTER TABLE usos ADD CONSTRAINT nota_resolucion_tope
  CHECK (char_length(nota_resolucion) <= 300);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Sin 'descartado' en el vocabulario, un descarte vuelve a caso pendiente (ONI).
UPDATE usos SET escalon = 'oni', oni = TRUE, resuelto_por = NULL, resuelto_en = NULL, puntaje = 0
 WHERE escalon = 'descartado';

ALTER TABLE usos DROP CONSTRAINT nota_resolucion_tope;
ALTER TABLE usos DROP CONSTRAINT resolucion_manual_tiene_nota;

ALTER TABLE usos DROP CONSTRAINT manual_tiene_autor;
ALTER TABLE usos ADD CONSTRAINT manual_tiene_autor
  CHECK (escalon <> 'manual' OR (resuelto_por IS NOT NULL AND resuelto_en IS NOT NULL));

ALTER TABLE usos DROP CONSTRAINT uso_resuelto_tiene_obra;
ALTER TABLE usos ADD CONSTRAINT uso_resuelto_tiene_obra
  CHECK (
    (escalon = 'excluido' AND NOT oni AND obra_id IS NULL)
    OR (escalon <> 'excluido'
        AND ((oni AND obra_id IS NULL) OR (NOT oni AND obra_id IS NOT NULL)))
  );

ALTER TABLE usos DROP CONSTRAINT usos_escalon_check;
ALTER TABLE usos ADD CONSTRAINT usos_escalon_check
  CHECK (escalon IN ('pendiente','alias','id_global','difuso','manual','oni','excluido'));

ALTER TABLE usos DROP COLUMN nota_resolucion;
-- +goose StatementEnd
