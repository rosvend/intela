-- La bandeja de anomalias de un periodo (#37, OE-5 / KR-4).
--
-- COORDINACION DE NUMERO: en `origin/main` (09034e24) existen 00001, 00002 y
-- 00005 a 00015; 00003 y 00004 nunca existieron. Numero: 00016, el primero
-- libre por encima de 00015, que es la version ya aplicada. Nunca un hueco
-- por debajo de ella (goose allowMissing=false, ver 00006).
--
-- ESTE FICHERO NACIO COMO 00015 Y SE MOVIO. Mientras esta PR estaba abierta,
-- `00015_bolsas_accesorias.sql` mergeo a main en la PR #144 (rama
-- feature/121-bolsas-accesorios, commit de merge af029b5) y se quedo con el
-- numero. La regla del repo es que RENUMERA EL QUE MERGEE SEGUNDO, y aqui el
-- segundo somos nosotros; la etapa `Migration versions` de CI (#110) es la
-- que cazo el choque.
--
-- 00016 TAMBIEN ESTA DISPUTADO. Al renumerar lo reclaman otras tres ramas, y
-- ninguna esta mergeada:
--
--   feature/36-...liquidacion      -> 00016_liquidacion.sql
--   feature/50-...afiliaciones     -> 00016_afiliaciones.sql
--   feature/52-...oni-publicacion  -> 00016_oni_publicacion.sql
--
-- No se renumera por encima de ellas para adelantarse: renumerar sin saber
-- quien entra primero cambia el numero dos veces y deja el segundo cambio sin
-- revisar. Vuelve a mandar la misma regla — el segundo en mergear se mueve.
-- Renumerar esta migracion es un `git mv` y nada mas: no la referencia ningun
-- codigo (`migrations/embed.go` usa `//go:embed *.sql`) y no depende de
-- ninguna migracion posterior a 00002.
--
-- EL CUERPO DEL ISSUE SE EQUIVOCA. Dice que "the `alertas` table exists"; no
-- existe. `grep -ri alerta migrations/` sobre main no devuelve una sola
-- coincidencia. El puerto `RepositorioAlertas` estaba declarado en
-- `internal/aplicacion/puertos.go` sin tabla, sin adaptador y sin caso de uso.
-- Por eso esta migracion va DENTRO de esta PR y no en una issue aparte: una
-- migracion suelta reclama numero y choca con las ramas abiertas por debajo.
--
-- ---------------------------------------------------------------------------
-- Por que una tabla propia y no `usos_rechazados`
-- ---------------------------------------------------------------------------
--
-- `usos_rechazados` ya admite tipo='anomalia' desde la migracion 00010, y a
-- primera vista cabria aqui. No cabe, por tres razones de forma:
--
--   1. Su clave y sus FK son de una FILA DE REPORTE (`id`, `reporte_id NOT
--      NULL REFERENCES reportes`). Cuatro de las seis anomalias no se cuelgan
--      de una fila de reporte: `duplicado_archivo` se cuelga de una ENTREGA,
--      y `titular_sin_porcentaje` y `reserva_declaracion_incompleta` de una
--      OBRA, que ni siquiera tiene por que venir de un reporte concreto.
--   2. No tiene estado. `ResolverAlerta` (criterio del issue) necesita saber
--      si alguien ya se hizo cargo, quien y cuando; el log de rechazos es una
--      lista de filas apartadas, no una bandeja de trabajo.
--   3. No tiene periodo. La misma obra puede quedar retenida en dos periodos
--      seguidos, y cada uno tiene su propia alerta que perseguir.
--
-- Meterlo ahi a la fuerza obligaria a un `reporte_id` inventado y a tres
-- columnas nuevas que solo usaria la mitad de las filas. ADR 0016 separo el
-- log de rechazos de `usos` por el mismo tipo de razon.
--
-- ---------------------------------------------------------------------------
-- La referencia al registro ofensor
-- ---------------------------------------------------------------------------
--
-- Par (ref_tipo, ref_id) TEXT con indice, que es el patron que este repo ya
-- bendijo para lo mismo en `asientos` (00001): la referencia apunta a tablas
-- distintas segun el caso, asi que no puede ser una FK. Aqui ademas HACE FALTA
-- que no lo sea: `ref_id` de una alerta de ONI apunta a un `usos.id` que la
-- resolucion de #39 puede llegar a borrar, y una FK con CASCADE se llevaria
-- por delante la alerta -- y con ella el rastro de que aquello paso.
--
-- ref_titular es la SEGUNDA coordenada, y solo de `titular_sin_porcentaje`. El
-- registro que falta ahi es una fila de `declaraciones`, cuya identidad es
-- (obra, titular): con la obra sola, dos coautores ausentes de la MISMA obra
-- colapsarian en una sola alerta y uno de los dos nombres se perderia. El
-- CHECK de abajo deja escrito que ninguna otra anomalia la puede usar.
--
-- ---------------------------------------------------------------------------
-- La clave natural, que es lo que hace idempotente a EvaluarAnomalias
-- ---------------------------------------------------------------------------
--
-- UNIQUE (periodo, tipo, ref_tipo, ref_id, ref_titular).
--
-- La evaluacion se corre varias veces sobre el mismo periodo -- al cerrar la
-- ingesta, otra vez cuando un autor declara, otra antes de la compuerta -- y
-- las tres pasadas ven las mismas anomalias. Sin esta clave, la tercera
-- triplica el tablero y el contador de criticas abiertas deja de significar
-- nada. `detalle` NO entra en la clave a proposito: es prosa, y reescribir una
-- frase duplicaria la fila.
--
-- Es la misma disciplina que la clave natural de `cola_trabajos` (00005).
--
-- ---------------------------------------------------------------------------
-- No es append-only, y no puede serlo
-- ---------------------------------------------------------------------------
--
-- `asientos` y `notificaciones` llevan triggers que rechazan UPDATE y DELETE
-- (ADR 0006). Esta tabla no, y la diferencia no es de rigor: un asiento
-- registra que algo PASO y una alerta registra que algo SIGUE pasando.
-- Resolverla es cambiar ese estado. El rastro de quien lo cambio va donde
-- tiene que ir: a un asiento, escrito en la MISMA transaccion que el UPDATE
-- (ver aplicacion.Anomalias.Resolver).

-- +goose Up

-- +goose StatementBegin
CREATE TABLE alertas (
  -- UUID de la base, como `asientos` y por lo mismo: un id derivado del
  -- hallazgo ('al-' || tipo || ref_id) seria una segunda clave natural que
  -- habria que mantener de acuerdo con la de abajo, y la primera vez que se
  -- separaran una alerta se escribiria dos veces.
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),

  -- El mismo patron que `reportes.periodo` y `bolsas.periodo`. El nucleo lo
  -- valida ademas con recaudo.ValidarPeriodo, que es MAS estricto (rechaza el
  -- mes 00 y el 13); estrechar el CHECK aqui pide tocar cuatro tablas y no es
  -- el alcance de esta PR.
  periodo      TEXT NOT NULL CHECK (periodo ~ '^[0-9]{4}(-[0-9]{2})?$'),

  -- Los cinco primeros son el contrato que ya fijo el tablero de la #104
  -- (web/src/reparto/tipos.ts). El sexto lo pide la revision del issue: una
  -- obra fuera de las cuatro categorias de `RD 9.1.1` no tiene ponderacion
  -- que aplicar.
  tipo         TEXT NOT NULL CHECK (tipo IN (
                 'oni',
                 'duplicado_archivo',
                 'duplicado_registro',
                 'titular_sin_porcentaje',
                 'reserva_declaracion_incompleta',
                 'tipo_obra_sin_mapear'
               )),

  -- Sin FK: ver la cabecera. El CHECK cierra el vocabulario para que un
  -- ref_tipo con otra grafia no deje una alerta que ninguna pantalla resuelve.
  ref_tipo     TEXT NOT NULL CHECK (ref_tipo IN ('uso', 'reporte', 'obra')),
  ref_id       TEXT NOT NULL CHECK (btrim(ref_id) <> ''),
  ref_titular  TEXT NOT NULL DEFAULT '',

  detalle      TEXT NOT NULL CHECK (btrim(detalle) <> ''),
  detectada    TIMESTAMPTZ NOT NULL,

  -- La decision humana (#39). resuelta_por es FK a `usuarios` igual que
  -- `asientos.actor_id`: quien cierra una alerta es una cuenta del sistema, y
  -- un id que no resuelve a nadie deja una decision sin dueno.
  resuelta     BOOLEAN NOT NULL DEFAULT FALSE,
  resuelta_por TEXT REFERENCES usuarios(id),
  resuelta_en  TIMESTAMPTZ,
  nota         TEXT NOT NULL DEFAULT '',
  -- Cerrada por el sistema porque una reevaluacion ya no la detecto (ADR 0021).
  autocerrada  BOOLEAN NOT NULL DEFAULT FALSE,

  -- Una alerta resuelta dice quien y cuando: una persona (resuelta_por) o el
  -- sistema (autocerrada), nunca las dos ni ninguna. Sin resolver, nada.
  CONSTRAINT alerta_resuelta_tiene_firma
    CHECK (
      (resuelta AND resuelta_en IS NOT NULL
        AND ((resuelta_por IS NOT NULL AND NOT autocerrada)
          OR (resuelta_por IS NULL AND autocerrada)))
      OR (NOT resuelta AND NOT autocerrada AND resuelta_por IS NULL AND resuelta_en IS NULL)
    ),

  -- Resolver exige nota: es la justificacion auditable (ADR 0021).
  CONSTRAINT alerta_resuelta_tiene_nota
    CHECK (NOT resuelta OR btrim(nota) <> ''),

  -- ref_titular es exclusiva de una anomalia. Deja escrito en el esquema lo
  -- que si no seria una convencion: que la segunda coordenada pertenece al
  -- detector de titulares y a ningun otro.
  CONSTRAINT alerta_ref_titular_solo_de_titulares
    CHECK (ref_titular = '' OR tipo = 'titular_sin_porcentaje'),

  -- La clave natural. Ver la cabecera.
  CONSTRAINT alerta_unica_por_hallazgo
    UNIQUE (periodo, tipo, ref_tipo, ref_id, ref_titular)
);
-- +goose StatementEnd

-- El listado del tablero es "las de este periodo, primero las mas recientes".
-- Sin indice recorre la tabla entera, que crece con cada pasada de cada
-- periodo.
-- +goose StatementBegin
CREATE INDEX alertas_periodo ON alertas (periodo, detectada DESC, id);
-- +goose StatementEnd

-- La compuerta de #34 pregunta exactamente esto: cuantas criticas siguen
-- abiertas en un periodo. Parcial porque las resueltas no se cuentan nunca y
-- son las que se acumulan con el tiempo.
-- +goose StatementBegin
CREATE INDEX alertas_abiertas ON alertas (periodo, tipo) WHERE NOT resuelta;
-- +goose StatementEnd

-- Ir de un registro ofensor a sus alertas: lo que hara la bandeja de
-- resolucion de #39 al abrir un uso o una obra.
-- +goose StatementBegin
CREATE INDEX alertas_ref ON alertas (ref_tipo, ref_id);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP INDEX IF EXISTS alertas_ref;
DROP INDEX IF EXISTS alertas_abiertas;
DROP INDEX IF EXISTS alertas_periodo;
DROP TABLE IF EXISTS alertas;
-- +goose StatementEnd
