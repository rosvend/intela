-- Parametros normativos de valor textual (#194, P-18).
--
-- `parametros.valor` es NUMERIC(18,6), y no todo parametro normativo es una
-- cifra. La base de ponderacion de cine y teatro (`RD 9.2`, `RD 9.3`) es una
-- ELECCION entre dos medidas -`taquilla` o `espectadores`- y el reglamento se
-- contradice: el cuerpo de `RD 9.2` dice "ingresos de taquilla" y su ejemplo
-- calcula sobre espectadores (P-18, docs/dominio/preguntas-cliente.md). El
-- motor ya la recibe como `Snapshot.BaseCineTeatro`, pero ninguna clausula del
-- snapshot la llenaba: toda corrida de cine fallaba al valorizar con
-- "parametro normativo ausente: base_cine_teatro", y el API lo servia como un
-- 500 generico (#194).
--
-- Una columna aparte y no un codigo numerico ("1 = taquilla"): el valor tiene
-- que leerse igual en `parametros`, en el snapshot congelado y en el mensaje
-- de un auditor, sin tabla de traduccion (ADR 0004, ADR 0006).
--
-- COORDINACION DE NUMERO: 00024, primero libre por encima de lo que `main` ya
-- aplico (`00023_oni_publicacion.sql`). goose corre con allowMissing = false:
-- si otra PR toma el 00024 antes de mergear esta, esta sube al siguiente
-- libre, sin dejar huecos.

-- +goose Up

-- Exactamente uno de los dos valores. El charset del texto es el mismo que el
-- de un segmento de clave y deja fuera '=' y el salto de linea por la misma
-- razon que en 00012: `idDeSnapshot` hashea el preimagen "clave=valor\n", y un
-- valor que los contuviera podria fabricar la preimagen de otro conjunto.
-- +goose StatementBegin
ALTER TABLE parametros
  ALTER COLUMN valor DROP NOT NULL,
  ADD COLUMN valor_texto TEXT,
  ADD CONSTRAINT parametros_un_solo_valor CHECK ((valor IS NULL) <> (valor_texto IS NULL)),
  ADD CONSTRAINT parametros_valor_texto_charset CHECK (valor_texto ~ '^[a-z][a-z0-9_]*$');
-- +goose StatementEnd

-- `snapshots_parametros` es append-only (trigger de 00012), pero ADD COLUMN
-- sin DEFAULT no reescribe filas: no dispara el trigger y los snapshots ya
-- congelados quedan con valor_texto NULL, que es exactamente lo que eran.
-- +goose StatementBegin
ALTER TABLE snapshots_parametros
  ALTER COLUMN valor DROP NOT NULL,
  ADD COLUMN valor_texto TEXT,
  ADD CONSTRAINT snapshots_parametros_un_solo_valor CHECK ((valor IS NULL) <> (valor_texto IS NULL)),
  ADD CONSTRAINT snapshots_parametros_valor_texto_charset CHECK (valor_texto ~ '^[a-z][a-z0-9_]*$');
-- +goose StatementEnd

-- El snapshot pasa a exigir `cine_teatro.base` (version 2 del conjunto de
-- clausulas, postgres/parametros.go). Una base ya sembrada con el dataset
-- sintetico -la de la demo- la recibe aqui, con la misma marca sintetica que
-- el resto de sus parametros: sin ella, ninguna corrida nueva se podria abrir
-- hasta resembrar, y resembrar se niega en cuanto hay asientos.
--
-- Solo si la base tiene los parametros del dataset sintetico: una instalacion
-- con parametros reales no recibe un valor inventado. Alli la clausula falta y
-- abrir una corrida responde nombrandola (ADR 0004: ausente, no por defecto)
-- hasta que se cargue la fila que decida el organo competente.
--
-- `taquilla` porque es lo que prescribe el cuerpo de `RD 9.2` (y `RD 9.3` por
-- remision), y es la unica medida de cine que el adaptador de ingesta exige
-- (`MapaCine`: `taquilla` es requerida, `espectadores` no). Provisional hasta
-- que REDES responda P-18.
-- +goose StatementBegin
INSERT INTO parametros (clave, valor_texto, vigente_desde, organo, reglamento)
SELECT 'cine_teatro.base', 'taquilla', MIN(vigente_desde), 'sintetico', 'RD-IX-seed-sintetico'
  FROM parametros
 WHERE organo = 'sintetico'
   AND reglamento = 'RD-IX-seed-sintetico'
HAVING COUNT(*) > 0
   AND NOT EXISTS (SELECT 1 FROM parametros WHERE clave = 'cine_teatro.base');
-- +goose StatementEnd

-- +goose Down

-- Un snapshot congelado con un valor textual no cabe en el esquema de 00023 y
-- no se puede borrar (ADR 0005): el down se niega en vez de dejar un snapshot
-- que ya no hashea a su id.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM snapshots_parametros WHERE valor_texto IS NOT NULL) THEN
    RAISE EXCEPTION
      'ADR 0005: hay snapshots congelados con parametros textuales; revertir 00024 los dejaria sin su valor';
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM parametros WHERE valor_texto IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE snapshots_parametros
  DROP CONSTRAINT IF EXISTS snapshots_parametros_valor_texto_charset,
  DROP CONSTRAINT IF EXISTS snapshots_parametros_un_solo_valor,
  DROP COLUMN IF EXISTS valor_texto,
  ALTER COLUMN valor SET NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE parametros
  DROP CONSTRAINT IF EXISTS parametros_valor_texto_charset,
  DROP CONSTRAINT IF EXISTS parametros_un_solo_valor,
  DROP COLUMN IF EXISTS valor_texto,
  ALTER COLUMN valor SET NOT NULL;
-- +goose StatementEnd
