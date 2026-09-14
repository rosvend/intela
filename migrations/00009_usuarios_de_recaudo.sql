-- El pagador del recaudo pasa a ser una fila, y `bolsas.usuario_id` a apuntar
-- a ella (#27).
--
-- `bolsas.usuario_id` era TEXT NOT NULL sin clave foranea, porque no habia
-- tabla a la que apuntar: la que se llama `usuarios` es la de CUENTAS que
-- inician sesion -rol, password_hash, titular_id- y no tiene nada que ver. El
-- "usuario" del reglamento es quien explota el repertorio y PAGA por ello: un
-- canal, una sala de cine, un hotel, una plataforma OTT (`RT 2`, glosario).
--
-- De ahi el nombre `usuarios_recaudo`. Renombrar la tabla de cuentas para
-- liberar `usuarios` habria sido mas fiel al glosario y arrastra sesiones,
-- provision, RBAC y el frontend; no es el alcance de este PR.
--
-- Sin la FK, un typo en `usuario_id` crea un pagador fantasma y el reparto le
-- atribuye dinero que alguien pago de verdad. Y sin el UNIQUE, cargar dos
-- veces el reporte de recaudo de un periodo duplica la bolsa, que aguas abajo
-- es repartir dos veces el mismo dinero.
--
-- ALCANCE (P-08): Intela RECIBE lo cobrado, por usuario y periodo. No liquida
-- tarifas y no factura. Por eso aqui no hay tabla `tarifas` ni `convenios`: la
-- tabla `T-01` a `T-11` del Reglamento de Tarifas es documentacion de
-- referencia. Las columnas `convenio`, `tarifa` y `factura` que `bolsas` ya
-- tiene desde 00001 siguen siendo PROCEDENCIA -la pregunta 1 del ADR 0006, de
-- donde salio este dinero- y se quedan opcionales a proposito: `T-11` dice que
-- la tarifa publicada es el valor por defecto CUANDO NO HAY convenio, asi que
-- exigir un convenio afirmaria algo que el reglamento no afirma.
--
-- NUMERO: 00009, el primero libre por encima de la version que produccion ya
-- aplico (00008). Colisiona con el `00009_afiliaciones.sql` de la PR #88, que
-- sigue abierta: dos archivos con nombres distintos y la misma version no dan
-- conflicto en git y hacen que `goose` entre en panic con "duplicate version 9"
-- en pleno despliegue. Quien mergee primero se queda el numero y el otro sube
-- al siguiente libre. Por la misma razon, las PR #80 (00007) y #87 (00008)
-- tendran que renumerar: sus numeros ya los ocupan migraciones mergeadas. La
-- regla utilizable es la que dejo escrita 00007_uso_excluido_no_es_oni.sql --
-- "el primero libre por encima de la version APLICADA, y se reasigna al
-- mergear"-- porque `goose` corre con allowMissing = false y un hueco por
-- debajo de la version aplicada bloquea el despliegue entero.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE usuarios_recaudo (
  id     TEXT PRIMARY KEY,
  nombre TEXT NOT NULL CHECK (btrim(nombre) <> ''),

  -- Opcional: el circuito internacional lo alimentan sociedades hermanas, que
  -- no tienen NIT colombiano.
  nit TEXT NOT NULL DEFAULT '',

  -- Las diez categorias de la tabla resumen `RT 4`, que es la operativa
  -- -`RT 3` anuncia seis tipos de usuario y enumera siete; la discrepancia
  -- esta en el documento fuente-. Mismo orden que recaudo.CategoriasUsuario().
  --
  -- Cerrado no por la tarifa -Intela no factura- sino porque la categoria
  -- decide QUE FORMULA de reparto aplica aguas abajo: television por puntos
  -- (RD 9.1.1), cine y teatro proporcional a taquilla (RD 9.2, RD 9.3), OTT
  -- por la ecuacion de RD 9.7, hoteles por remision a RD 9.5.
  categoria TEXT NOT NULL CHECK (categoria IN (
    'tv_abierta', 'tv_cerrada', 'cine',
    'transporte_aereo', 'transporte_terrestre', 'transporte_fluvial',
    'hotel', 'salud', 'medios_digitales', 'otros_establecimientos',
    'sin_clasificar'
  ))
);
-- +goose StatementEnd

-- El backfill va ANTES de la FK, como en 00008: `bolsas` ya cita pagadores que
-- todavia no tienen fila -- los del sembrador -- y anadir la restriccion sin
-- darlos de alta la haria fallar sobre una base que ya existe.
--
-- La categoria de esos no se puede deducir de una fila de `bolsas`, asi que
-- entra 'sin_clasificar' y no una categoria comoda. El ADR 0004 pide justo eso:
-- que el hueco se vea. Un 'otros_establecimientos' inventado aqui seria una
-- clasificacion con consecuencias de calculo que nadie decidio.
-- +goose StatementBegin
INSERT INTO usuarios_recaudo (id, nombre, categoria)
SELECT DISTINCT usuario_id, usuario_id, 'sin_clasificar'
  FROM bolsas
ON CONFLICT (id) DO NOTHING;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE bolsas
  ADD CONSTRAINT bolsas_usuario_id_fkey
  FOREIGN KEY (usuario_id) REFERENCES usuarios_recaudo(id);
-- +goose StatementEnd

-- Una bolsa por usuario, periodo y circuito.
--
-- El circuito entra en la clave a proposito: el nacional y el internacional del
-- mismo usuario y periodo son dos bolsas legitimas y separadas (`RD 10.3`,
-- R-35), no una etiqueta sobre el mismo dinero -- se invierten por separado, la
-- reserva del 5% solo aplica al nacional (R-07) y las fechas de corte no
-- coinciden (R-34)-.
--
-- Esto sube al esquema el invariante que hasta ahora solo comprobaba
-- semilla/dataset_test.go sobre la fixture.
--
-- Si la base ya tuviera una repeticion, este ALTER FALLA y goose revierte la
-- migracion entera. Es el comportamiento que se quiere y no se le pone remedio
-- automatico: cada fila de `bolsas` es dinero, asi que elegir por su cuenta cual
-- de dos bolsas repetidas sobra es precisamente lo que un despliegue no debe
-- hacer. Se reconcilia a mano -- comparando contra la cuenta de cobro-- y se
-- vuelve a desplegar. Para saber si hay algo que reconciliar, antes de desplegar:
--
--     SELECT usuario_id, periodo, circuito, count(*), sum(bruto)
--       FROM bolsas GROUP BY 1,2,3 HAVING count(*) > 1;
-- +goose StatementBegin
ALTER TABLE bolsas
  ADD CONSTRAINT bolsas_usuario_periodo_circuito_key
  UNIQUE (usuario_id, periodo, circuito);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE bolsas DROP CONSTRAINT bolsas_usuario_periodo_circuito_key;
ALTER TABLE bolsas DROP CONSTRAINT bolsas_usuario_id_fkey;
-- +goose StatementEnd

-- Sin DROP ... CASCADE: si algo mas hubiera acabado apuntando a esta tabla,
-- que la bajada falle en voz alta es mejor que perder esa referencia en
-- silencio. Las bolsas se quedan con su `usuario_id` como texto libre, que es
-- exactamente como estaban antes de esta migracion.
-- +goose StatementBegin
DROP TABLE usuarios_recaudo;
-- +goose StatementEnd
