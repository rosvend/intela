# Plan de traspaso: #175, resolución manual de identificación

Issue: https://github.com/rosvend/intela/issues/175, con el comentario de diseño
https://github.com/rosvend/intela/issues/175#issuecomment-5858319254.
Asignada a @Killgreck. La adelanta sanmemu09 en local.

Este plan es autocontenido: lo ejecuta otra conversación que no vio la sesión en la que se
preparó. Lo redactó Claude Code el 2026-09-27, después de leer el código de `main` en `96943d3`.

---

## 0. Estado y reglas no negociables

**Estado.**
- Worktree: `C:\Users\Usuario\Documents\REPOSITORIOS\Intela-wt\175`.
- Rama: `feature/175-resolucion-manual-identificacion`, creada desde `main` en `96943d3`.
- **No hay ningún cambio de código.** Este archivo es lo único nuevo y está sin commitear.

**Reglas.**
1. **Git solo local.** Commits en esta rama, **nunca push ni PR** (lo decide sanmemu09).
2. **No tocar el checkout principal** `C:\Users\Usuario\Documents\REPOSITORIOS\Intela`: tiene cuatro
   PNG sin commitear de #39. Todo el trabajo va en el worktree.
3. **Leer `CLAUDE.md`** en la raíz antes de empezar. Lo que más se rompe:
   - El dominio se nombra en español (`obra`, `uso`, `escalon`...). La infraestructura, en inglés.
   - Frontera de arquitectura (ADR 0012, `.golangci.yml`):
     - `internal/dominio` no importa `time`, `os`, `database/sql`, `pgx`, `chi`, etc.
     - `internal/aplicacion` no importa `infraestructura`, `pgx` ni `chi`.
   - El instante entra por el puerto `Reloj` (`internal/aplicacion/puertos.go:33`), nunca `time.Now()` en el núcleo.
   - El asiento de bitácora es parte de "hecho" (ADR 0006): si el asiento falla, el caso de uso no está hecho y se revierte todo.
   - YAGNI/KISS: el sistema mueve dinero de terceros.
   - Estilo de comentarios: español sin tildes, igual que el código existente.
4. **TDD.** En cada paso, primero la prueba en rojo.
5. **Este camino no escribe titulares, porcentajes ni importes** (R-02, R-03, ADR 0007). Tampoco
   medidas de ponderación en las respuestas: la respuesta de un caso no lleva rating, vistas ni taquilla.
6. Antes de usar una API de librería **que el repo no use ya**, consultar su documentación actual
   (el repo pide el CLI `ctx7`). Aquí casi todo es `pgx` y `chi`, ya en uso.
7. **Suite completa al terminar**, no un subconjunto. Ver §7: la regla es explícita del usuario.

---

## 1. Qué pide la issue

Un caso de uso y un `POST` por caso, **solo para administrador**, que:

1. **Asigna** un uso ONI pendiente a una obra, candidata o cualquiera del catálogo:
   `escalon='manual'`, `obra_id`, `oni=false`, `resuelto_por` (de la sesión), `resuelto_en` (del
   `Reloj`). O lo **descarta**.
2. **Aprende el alias** de los `ids_fuente` del uso con `quien` = el id del usuario (ADR 0007:
   resolver una vez, reutilizar siempre).
3. **Escribe el asiento** (ADR 0006). La asignación referencia la obra, para que
   `GET /auditoria/obra/{id}` la devuelva.

Criterios de aceptación de la issue:
- Asignar a una candidata deja `manual`, la obra, `oni=false`, `resuelto_por` y `resuelto_en`, y el
  caso deja de estar pendiente.
- Asignar a una obra buscada que no era candidata funciona igual, y el asiento dice que no era candidata.
- Descartar saca el caso de la lista de pendientes con el efecto acordado (D1), y queda registrado
  con usuario, fecha y nota.
- El alias se aprende: un reporte posterior de la misma `fuente` con el mismo id resuelve en el
  escalón 1, sin pasar por la cola.
- Cada resolución escribe un asiento con actor, instante, decisión y nota. El de una asignación lo
  devuelve `GET /auditoria/obra/{id}`.
- Sin nota, o con solo espacios: 400. Ya resuelto: 409. Caso desconocido: 404. Obra fuera del catálogo: 4xx.
- Nada escribe titulares, porcentajes ni importes.
- La frontera se mantiene: `golangci-lint run --enable-only=depguard ./...` en verde. `make verificar` en verde.

Del comentario de diseño (frames de #39):
- El asiento lleva `titulo`, `fuente`, `periodo` y `reporte_id` del uso **tal como estaban al decidir**.
- Hace falta el **nombre** del actor, no solo su id.
- La nota tiene tope (D5).
- El 409 ya está diseñado en el front: "Otra persona resolvió este caso antes" y "Recargar caso".

---

## 2. Decisiones tomadas (sanmemu09, 2026-09-27): no reabrir

**D1. "Descartar" es un escalón nuevo, `descartado`.**
- El uso queda **sin obra, `oni=false` y no pondera**. Queda fuera del listado público ONI, porque
  la vista `oni_publico` filtra `WHERE u.oni`.
- Va **firmado**: `resuelto_por`, `resuelto_en` y nota obligatorios.
- **La cascada no lo vuelve a tocar**: `reprocesable` devuelve `false`, igual que para `manual`.
- Efecto monetario: el mismo que una exclusión R-27. Su parte no existe y la bolsa se reparte entre
  las obras que sí ponderan.
- Citas: RD 7.1 (REDES solo representa autores de guion o libreto); RD 9.5 y R-27 (se excluye lo que
  no es catálogo de REDES); RD 13.8 es el régimen ONI que **no** aplica al descartado.
- **Por qué no se reusa `excluido`:** `ResolverUsos` reprocesa las filas `excluido` en cada corrida
  (`internal/aplicacion/identificacion.go:205-215`: la exclusión es configuración). Además
  `FueraDeRepertorio` está vacío en producción. La decisión de la persona se desharía sola en la
  corrida siguiente.
- Matiz para la documentación: hoy el motor **todavía no reserva la parte ONI** (R-18 pendiente,
  dueño #33/#34), así que hoy ONI y descartado pesan lo mismo, cero. La diferencia se vuelve real
  cuando llegue la reserva ONI.

**D2. Alerta `oni` de #37: se deja al autocierre.** El caso de uso **no toca `alertas`**.
- `anomalias.EsCritica` no incluye `oni`, así que esa alerta no bloquea la compuerta.
- La siguiente `Anomalias.Evaluar` ya no la detecta, porque `deteccionONI` filtra por `escalon = 'oni'`,
  y la autocierra con el actor de sistema.
- Una alerta autocerrada se reabre sola si el uso volviera a ONI. Una cerrada por una persona, no.

**D3. Se puede resolver siempre, aunque el periodo esté en reparto o ya distribuido.**
- La resolución se serializa con el **cerrojo de periodo** de #171: el mismo `pg_advisory_xact_lock`
  con la clave de `claveCerrojoAlertas(periodo)` (`internal/infraestructura/postgres/alertas.go`,
  helper `bloquearAlertasEn`). Así no se cuela entre la compuerta y la valorización.
- Una corrida ya valorizada no cambia: sus asientos `reparto.obra_valorizada` congelaron cómo se
  identificó cada uso (`OrigenDeUso`).
- El dinero de lo ya distribuido va por RD 13.8.5, que paga "en el siguiente proceso", cuando exista
  la reserva ONI. Eso no es de esta issue.

**D4. Corregir una decisión manual queda fuera de alcance.** Tanto reasignar como deshacer un descarte.

**D5. Dónde viven la nota y el nombre.**
- Nota: columna nueva `usos.nota_resolucion` (migración) **y** en el payload del asiento. Es el
  mismo patrón que `alertas.nota` junto con el asiento `alerta.resuelta` (ADR 0021).
- `GET /identificacion/casos` devuelve `nota`. El PR #176 ya anunció que #175 la añade.
- **Tope: 300 caracteres** (runas, no bytes), contados tras recortar espacios. En el contrato,
  `maxLength: 300`; más largo da 400. En la base, un CHECK con `char_length`.
- El nombre del actor va en el payload como `actor_nombre`, sacado de la **sesión** al decidir:
  `aplicacion.Usuario.Nombre`, `internal/aplicacion/modelos.go:27`.

**D6. Alias en conflicto: se rechaza sin escribir nada.** Pasa si el par canónico (fuente, tipo,
valor) del uso ya tiene alias hacia **otra** obra. Es un caso real: dos emisiones del mismo programa;
se resuelve una, se aprende el alias, y la otra sigue pendiente hasta la próxima corrida. Responde
409 con mensaje propio. En el repo no se usa 422 en ningún handler.
- Si el alias ya existe hacia la **misma** obra, no es error: no se aprende nada nuevo.
- Si el uso no tiene par local (sin `id_ficha`, `show_id`, etc.), no hay alias que aprender y la
  asignación sigue. Es lo mismo que hace la cascada.

**D7. Una sola ruta:** `POST /identificacion/casos/{id}/resolucion`, con cuerpo
`{ "decision": "asignar" | "descartar", "obra_id"?: string, "nota": string }`. Responde 200 con el
caso resuelto, con el mismo schema `CasoIdentificacion` que la lista.

**D8. Nombres de los hechos:** `identificacion.asignada`, con ref `("obra", obraID)`, e
`identificacion.descartada`, con ref `("uso", usoID)`. El front de auditoría ya agrupa el prefijo
`identificacion.` bajo "Identificación" (`web/src/auditoria/tipos.ts:76`).

---

## 3. Anclas verificadas en el código (main en 96943d3)

**Dominio**
- `internal/dominio/identificacion/cascada.go:24-32`: constantes de escalón. Falta `EscalonDescartado`.
- `internal/dominio/identificacion/tipos.go`: `Entrada`, `Candidato` (con `Puntaje` decimal y
  `TituloConsultado`) y `Resultado` (`ObraID`, `Escalon`, `Puntaje`, `ONI`, `Evidencia`, `Candidatos`).
- `internal/dominio/identificacion/metricas.go:26-52`: `Metricas` cuenta `Manuales` y `Excluidas`.
  No conoce `descartado`; hoy caería en `AONI`.

**Aplicación**
- `internal/aplicacion/identificacion.go`:
  - `:205-215`: `reprocesable()`. Añadir `EscalonDescartado` al caso `false`.
  - `:435-458`: `entradaDesdeUso()` saca el par canónico (TipoID, ValorID), el mismo que usa la cascada. **Reusarlo.**
  - `:464-480`: `parLocalCanonico`.
- `internal/aplicacion/casos_identificacion.go` (lectura de la cola, #174):
  - estados `pendiente` y `asignado`;
  - `escalonesDeEstado`;
  - `consultaDe()`: por defecto lista `[oni, manual]`;
  - `completarCaso()`: falla cerrado ante un escalón desconocido;
  - struct `CasoIdentificacion` (sin `Nota`).
- `internal/aplicacion/errores.go`:
  - `ErrNoEncontrado`;
  - `ErrActorAusente` y `exigirActor()` (`:464`);
  - `ErrNotaObligatoria` (la de alertas, con "alerta" en el mensaje: no reusarla aquí);
  - `ErrAlertaYaResuelta`, patrón para el 409.
- `internal/aplicacion/anomalias.go:502-563`: `Anomalias.Resolver`. Es **el patrón a copiar**: actor
  y nota validados antes de nada, una unidad de trabajo con el UPDATE y el asiento dentro, sentinelas
  que suben sin envolver, y `cableado()` en `:608`.
- `internal/aplicacion/puertos.go`:
  - `:399` `RepositorioIdentificacion` (`Alias`, `GuardarAlias` y otros);
  - `:627` `RepositorioCasosIdentificacion`;
  - `:1011` `BitacoraAuditoria` (`Asentar`, `De`...);
  - `:1064` `UnidadDeTrabajo`, que es reentrante en Postgres.
- `internal/aplicacion/catalogo.go:35`: `RefObra = "obra"`. No existe una constante `RefUso` en `aplicacion`.
- `internal/aplicacion/auditoria.go:50`: `HistorialDeObra` lee `Bitacora.De(ctx, "obra", obraID)`.
- `internal/aplicacion/modelos.go`:
  - `:222` `UsoPersistido`;
  - `:316` `ResumenUsosDeCanal` cuenta `Pendientes`, `ONI` y `Excluidos`, y le falta `Descartados`;
  - `:380` `Asiento`.

**Postgres**
- `internal/infraestructura/postgres/identificacion.go`:
  - `:34` `GuardarAlias`: `ON CONFLICT DO NOTHING`, y `quien` vacío se guarda como NULL.
  - `:110-132` `GuardarMatch`: UPDATE condicional `WHERE escalon = $6`, y el `CASE` que copia
    `obras.tipo` cuando `tipo_obra` está vacío (desde #169).
  - `:160` `CandidatosDeUso`.
  - **Comentario obsoleto** en `:89-109`: dice que `tipo_obra` NO se rellena, pero el SQL sí lo
    rellena. Lo reintrodujo el squash de #171. Ver §4.7.
- `internal/infraestructura/postgres/casos_identificacion.go`: `sqlCasosIdentificacion`, una sola
  sentencia para la página y el conteo de pendientes (`escalon = $4`, ONI).
- `internal/infraestructura/postgres/reparto.go:71-87`: `resumenExclusiones`, que cuenta con `FILTER` por escalón.
- `internal/infraestructura/postgres/alertas.go`: `BloquearAlertasDePeriodo` y `bloquearAlertasEn`
  (cerrojo de periodo), `claveCerrojoAlertas`.
- `internal/infraestructura/postgres/store.go`:
  - `:174` `txDe(ctx)`;
  - `:190` `EnUnidad`;
  - `:202` `ejecutorDe`;
  - `:214` `enTransaccionDe`.
- `internal/infraestructura/postgres/liquidacion.go:39`: `errFueraDeUnidad`.
- `internal/infraestructura/postgres/ingesta.go:27-36`: `columnasUso` y `escanearUso`.

**HTTP**
- `internal/infraestructura/httpapi/alertas.go:284-327`: `resolverAlerta`. **Patrón del handler**:
  `MaxBytesReader`, decode tolerante a EOF, `UsuarioDe`, y el `switch errors.Is`.
- `internal/infraestructura/httpapi/casos_identificacion.go`: `casoJSON`, `aCasoJSON` y `listarCasosIdentificacion`.
- `internal/infraestructura/httpapi/server.go:192-195`: grupo `/identificacion` con
  `requiereRol(aplicacion.RolAdministrador)`.
- `internal/infraestructura/httpapi/auth.go:44`: `UsuarioDe(ctx)` devuelve un `aplicacion.Usuario`
  con `ID`, `Nombre` y `Rol`.

**Cableado**
- `cmd/api/main.go:230` y `cmd/lambda/main.go:265`: `Identificacion: aplicacion.CasosIdentificacion{Repo: store}`.

**Esquema**
- `migrations/00001_init.sql:155-199`: tabla `usos`, con las restricciones `uso_resuelto_tiene_obra`
  y `manual_tiene_autor`, más `usos_pendientes` y `usos_oni`. La vista `oni_publico` está en `:209`.
- `migrations/00007_uso_excluido_no_es_oni.sql`: añadió `excluido` a `usos_escalon_check` y reescribió
  `uso_resuelto_tiene_obra`. **Es el precedente exacto** de la migración nueva, cabecera incluida.
- `main` llega a la migración **00021**.

**Contrato**
- `api/openapi.yaml`:
  - `:921` `/identificacion/casos`;
  - `:1147` `/auditoria/obra/{id}`;
  - `:3066` `/alertas/{id}/resolver`, patrón de la ruta;
  - `:4942` `ResolucionDeAlerta`;
  - `:5320` `Asiento`;
  - `:5569` `PaginaCasosIdentificacion`;
  - `:5582` `CasoIdentificacion`.

**Front**
- `web/package.json`:
  - `contrato` = `openapi-typescript ../api/openapi.yaml -o src/contrato.d.ts`;
  - `contrato:check`;
  - `test` = `npm run contrato:check && vitest run`;
  - `typecheck`.
- En `web/src` todavía nadie consume `CasoIdentificacion`: solo aparece en `contrato.d.ts`.

**Build**
- `Makefile`: `verificar: tidy build vet fmt-check test`.
- `go.mod`: `go 1.24.0`.

---

## 4. Diseño

### 4.1 Migración `migrations/00022_resolucion_manual_identificacion.sql`

**Número.** 00022 es la primera libre por encima de la versión aplicada (00021). **Ojo:** el PR
abierto #87 (listado público ONI) ya trae `migrations/00022_oni_publicacion.sql`.
- Regla del repo (cabeceras de 00006, 00007 y 00013): "el primero libre por encima de la versión
  APLICADA, y se reasigna al mergear". Quien mergee segundo renumera.
- Dos ficheros con la misma versión no chocan en git, pero hacen que goose entre en panic.
- La guarda `internal/infraestructura/migraciones/numeracion` compara contra el árbol de `main`.
- Documentar esta coordinación en la cabecera, como hicieron 00007 y 00013.

**Cabecera.** Explicar en prosa, en el estilo de 00007:
- qué es `descartado` y en qué se diferencia de `excluido` y de ONI (D1), con sus citas;
- por qué `manual_tiene_autor` se extiende a `descartado`;
- la nota y su tope (D5);
- que ninguna ruta escribía `manual` antes, y por eso el relleno defensivo.

```sql
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
```

Notas:
- Antes de escribirla, comprobar que 00010, 00011 y 00014 no renombraron estas restricciones (§8.1).
- La vista `oni_publico` y los índices `usos_oni` y `usos_pendientes` no se tocan: con `oni=false` y
  un escalón distinto de `pendiente`, la fila descartada queda fuera de los tres.

### 4.2 Dominio: `internal/dominio/identificacion/resolucion.go` (nuevo, puro)

- En `cascada.go`, añadir `EscalonDescartado = "descartado"` con su comentario. Actualizar también el
  comentario de `EscalonManual`: ahora la escribe la resolución manual de #175.
- En `resolucion.go`:

```go
const MaxNotaResolucion = 300 // runas, tras recortar (D5)

type Decision string

const (
    DecisionAsignar   Decision = "asignar"
    DecisionDescartar Decision = "descartar"
)

var (
    ErrNotaVacia          = errors.New("la nota es obligatoria para resolver un caso")
    ErrNotaDemasiadoLarga = errors.New("la nota no puede pasar de 300 caracteres")
    ErrDecisionInvalida   = errors.New("decision invalida")
    ErrCasoYaResuelto     = errors.New("otra persona ya resolvio este caso")  // manual o descartado
    ErrCasoNoPendiente    = errors.New("el caso ya no esta pendiente")        // cualquier otro escalon
)

// NormalizarNota recorta y valida: vacia o solo espacios -> ErrNotaVacia;
// mas de MaxNotaResolucion runas (utf8.RuneCountInString) -> ErrNotaDemasiadoLarga.
func NormalizarNota(nota string) (string, error)

// ValidarDecision es la forma del pedido, antes de leer nada:
// asignar exige obraID no vacio (tras TrimSpace); descartar exige obraID vacio;
// cualquier otra decision -> ErrDecisionInvalida (envuelto con el detalle).
func ValidarDecision(d Decision, obraID string) error

// ResolverCaso aplica la transicion sobre el escalon leido:
// - solo EscalonONI es resoluble;
// - EscalonManual / EscalonDescartado -> ErrCasoYaResuelto;
// - cualquier otro (pendiente, alias, id_global, difuso, excluido) -> ErrCasoNoPendiente
//   envuelto con el escalon actual.
// asignar  -> Resultado{ObraID, Escalon: EscalonManual, ONI: false,
//             Puntaje: el del candidato si obraID esta entre los candidatos, si no 0,
//             Evidencia determinista}
// descartar -> Resultado{Escalon: EscalonDescartado, ONI: false, Puntaje: 0, Evidencia}
func ResolverCaso(escalonActual string, d Decision, obraID string, candidatos []Candidato) (Resultado, error)
```

- Evidencia determinista, con el puntaje a cinco decimales mediante el `puntajeLegible` existente:
  - candidata: `manual: candidata obra-12 (0.52941) de 2 propuestas`;
  - buscada: `manual: obra obra-40 buscada en el catalogo, fuera de las 2 candidatas`;
  - descartado: `descartado a mano: no es un uso del repertorio`.
- Nombres de error distintos de `aplicacion.ErrNotaObligatoria` a propósito: aquella es de alertas.
- `metricas.go` (recomendado, es pequeño y cierra un hueco): añadir `Descartadas int` al `Reporte`.
  - Contarla **antes** de mirar la obra, como `Manuales`.
  - Sacarla del denominador de KR-2 igual que `Excluidas`, porque es "no es repertorio":
    `porcentaje(r.Automaticas, r.Total-r.Excluidas-r.Descartadas)`.
  - Actualizar la prueba que exige que las categorías cierren contra `Total`
    (`metricas_test.go`) y el `Printf` de `cmd/metricas-matching/main.go:185`.
  - La cascada nunca produce `descartado`. Esto es defensa, para que no caiga en `AONI`.

### 4.3 Aplicación

**a) `casos_identificacion.go` (lectura de la cola)**
- `EstadoCasoDescartado = "descartado"`, y su entrada en `escalonesDeEstado`.
- Sin `estado`, `consultaDe` lista `[oni, manual, descartado]`. El mensaje de error de un estado
  inválido nombra los tres.
- `completarCaso`: caso `EscalonDescartado`, que exige `ResueltoEn`; `Estado = descartado` y
  `UltimaActualizacion = *ResueltoEn`. Sin `ResueltoEn`, error, igual que manual.
- `CasoIdentificacion.Nota string`.
- Actualizar el comentario de la cabecera: "descartado llega con la escritura de #175".

**b) `identificacion.go`:** `reprocesable` devuelve `false` para `EscalonDescartado`. Actualizar el
comentario: "'manual' nunca" pasa a "'manual' ni 'descartado' nunca: una decisión humana no se pisa".

**c) `modelos.go`:** añadir `Descartados int` a `ResumenUsosDeCanal`, con su viñeta en el comentario:
"una persona decidió que no es un uso del repertorio (#175): no pondera y no es ONI".

**d) Nuevo `resolucion_identificacion.go`** (caso de uso más su puerto estrecho, declarado junto al
consumidor como `LecturaDeEntregas`):

```go
const (
    HechoIdentificacionAsignada   = "identificacion.asignada"
    HechoIdentificacionDescartada = "identificacion.descartada"
    RefUso                        = "uso"
)

// RepositorioResolucionIdentificacion: lo justo para resolver un caso. *postgres.Store lo satisface.
type RepositorioResolucionIdentificacion interface {
    PeriodoDeUso(ctx context.Context, usoID string) (string, error)               // ErrNoEncontrado
    BloquearPeriodoDeUsos(ctx context.Context, periodo string) error              // exige unidad abierta
    CasoParaResolver(ctx context.Context, usoID string) (CasoParaResolver, error) // SELECT ... FOR UPDATE
    TituloDeObra(ctx context.Context, obraID string) (string, error)              // ErrNoEncontrado
    Alias(ctx context.Context, fuente, tipo, valor string) (string, error)        // ya existe en *Store
    GuardarAlias(ctx context.Context, fuente, tipo, valor, obraID, quien string) error // ya existe
    GuardarResolucionManual(ctx context.Context, r ResolucionManual) error
    CasoIdentificacionPorID(ctx context.Context, usoID string) (CasoIdentificacion, error)
}

type CasoParaResolver struct {
    Uso        UsoPersistido
    Periodo    string
    Candidatos []identificacion.Candidato
}

type ResolucionManual struct {
    UsoID     string
    Resultado identificacion.Resultado
    ActorID   string
    Cuando    time.Time
    Nota      string
}

type SolicitudResolucion struct{ UsoID, Decision, ObraID, Nota string }

type ResolucionIdentificacion struct {
    Repo     RepositorioResolucionIdentificacion
    Bitacora BitacoraAuditoria
    Unidad   UnidadDeTrabajo
    Reloj    Reloj
}

func (r ResolucionIdentificacion) Resolver(ctx context.Context, s SolicitudResolucion,
    actorID, actorNombre string) (CasoIdentificacion, error)
```

Sentinelas nuevas en `errores.go`, cada una con su comentario del porqué, como las demás:
- `ErrObraInexistente`: "esa obra no esta en el catalogo". Da 400, por la convención de
  `ErrTitularInexistente`: un dato del cuerpo que señala una entidad que no existe.
- `ErrAliasEnConflicto`: da 409 (D6).

**Flujo de `Resolver`**, en este orden: las validaciones baratas antes de tocar la base.
1. `exigirActor(actorID, ...)`, que da `ErrActorAusente`.
2. `nota, err := identificacion.NormalizarNota(s.Nota)`.
3. `identificacion.ValidarDecision(Decision(s.Decision), strings.TrimSpace(s.ObraID))`.
4. `cableado()`: dependencias nil devuelven error **antes** de abrir la unidad.
5. `r.Unidad.EnUnidad(ctx, func(ctx) error { ... })`, pasando **el `ctx` de `fn`** a todos los puertos:
   1. `periodo := Repo.PeriodoDeUso(usoID)`. `ErrNoEncontrado` sube sin envolver y da 404.
   2. `Repo.BloquearPeriodoDeUsos(periodo)` (D3). El cerrojo va **antes** del bloqueo de la fila,
      el mismo orden que usan la valorización y la ingesta.
   3. `ahora := r.Reloj.Ahora()`, leído con el cerrojo tomado. Es el mismo criterio que el comentario
      de `Anomalias.Evaluar`, y sirve para `resuelto_en` y para `Asiento.Cuando`.
   4. `caso := Repo.CasoParaResolver(usoID)`, que bloquea la fila con `FOR UPDATE`.
   5. `res := identificacion.ResolverCaso(caso.Uso.Escalon, decision, obraID, caso.Candidatos)`.
      Devuelve 409 si el caso no está pendiente.
   6. Si es asignar: `titulo := Repo.TituloDeObra(obraID)`. Con `ErrNoEncontrado`, devolver
      `ErrObraInexistente` (400).
   7. Si es asignar: `e := entradaDesdeUso(caso.Uso)`. Si `e.TipoID != ""` y `e.ValorID != ""`:
      - `existente, err := Repo.Alias(e.Fuente, e.TipoID, e.ValorID)`;
      - encontrado y distinto de `obraID`: `ErrAliasEnConflicto`;
      - encontrado e igual: `alias.aprendido = false`;
      - `ErrNoEncontrado`: `Repo.GuardarAlias(..., obraID, actorID)` y **releer** `Alias`. Si ahora
        apunta a otra obra, es que perdimos una carrera contra `ON CONFLICT DO NOTHING`, y se devuelve
        `ErrAliasEnConflicto`. Si no, `alias.aprendido = true`;
      - cualquier otro error sube (D8 de la cascada: un fallo de base no es "no hay alias").
   8. `Repo.GuardarResolucionManual(ResolucionManual{usoID, res, actorID, ahora, nota})`.
   9. `Bitacora.Asentar(Asiento{...})` con el payload de abajo. Un error revierte **todo**.
   10. `caso := Repo.CasoIdentificacionPorID(usoID)`, luego `completarCaso(caso)`, que se devuelve.
       Va dentro de la unidad para que la respuesta sea atómica con la escritura.
6. Devolver el caso.

Los errores del repositorio y del dominio suben **sin envolver**, o envueltos con `%w`, para que el
handler los distinga con `errors.Is`.

**Payload del asiento** (JSON en snake_case, decimales como string, igual que `AsientoValorizacion`):

```json
{
  "uso_id": "uso-1",
  "decision": "asignar",
  "obra_id": "obra-12",
  "candidata": true,
  "puntaje": "0.52941",
  "candidatos_propuestos": 2,
  "titulo": "La Nina T3 E12",
  "titulo_original": "",
  "fuente": "caracol",
  "periodo": "2024-11",
  "reporte_id": "rep-1",
  "ids_fuente": "id_ficha=871732",
  "escalon_anterior": "oni",
  "evidencia_anterior": "banda ambigua: ...",
  "nota": "coincide la ficha tecnica con la declaracion",
  "actor_nombre": "Ana Perez",
  "alias": {"fuente": "caracol", "tipo_id": "id_ficha", "valor": "871732", "aprendido": true}
}
```

- Descartar: sin `obra_id`, `candidata` ni `puntaje` (`omitempty`), y con `"alias": null`.
- Obra buscada: `"candidata": false` y sin `puntaje`.
- Uso sin par local: `"alias": null`.
- Referencia: asignar usa `RefTipo = RefObra` y `RefID = obraID`; descartar usa `RefTipo = RefUso` y
  `RefID = usoID`. `ActorID = actorID` y `Cuando = ahora`.

### 4.4 Postgres: `internal/infraestructura/postgres/resolucion_identificacion.go` (nuevo)

- `var _ aplicacion.RepositorioResolucionIdentificacion = (*Store)(nil)`.
- `PeriodoDeUso`:
  `SELECT r.periodo FROM usos u JOIN reportes r ON r.id = u.reporte_id WHERE u.id = $1`.
  Sin filas, `traducirError` devuelve `ErrNoEncontrado`. Comprobar que lo hace así.
- `BloquearPeriodoDeUsos`: `tx, hay := txDe(ctx)`. Sin unidad, `errFueraDeUnidad`. Con ella,
  `bloquearAlertasEn(ctx, tx, periodo)`: **la misma clave** que usan `Evaluar`, la ingesta y la
  valorización de #171. En el comentario, decir que es a propósito.
- `CasoParaResolver`: `SELECT <columnasUso con prefijo u.>, r.periodo FROM usos u JOIN reportes r ...
  WHERE u.id = $1 FOR UPDATE OF u`, más los candidatos con el `CandidatosDeUso` existente. Reusar
  `escanearUso` si la proyección cuadra; si no, un escaneo propio (§8.5).
- `TituloDeObra`: `SELECT titulo FROM obras WHERE id = $1`.
- `GuardarResolucionManual`:
  ```sql
  UPDATE usos
     SET obra_id = NULLIF($2, ''), escalon = $3, evidencia = $4, puntaje = $5,
         oni = FALSE,
         resuelto_por = $6, resuelto_en = $7, nota_resolucion = $8,
         -- El mismo CASE que GuardarMatch (#165/#169): con obra y el campo vacio,
         -- el tipo sale del catalogo (lo exige el ADR 0021 para la asignacion manual).
         tipo_obra = CASE
           WHEN $2 = '' OR tipo_obra <> '' THEN tipo_obra
           ELSE COALESCE((SELECT tipo FROM obras WHERE id = $2), tipo_obra)
         END
   WHERE id = $1 AND escalon = 'oni'
  ```
  Con `RowsAffected() == 0`, devolver `identificacion.ErrCasoYaResuelto` envuelto. Es defensa, porque
  la fila ya está bloqueada. **No borrar `candidatos_match`**: es la evidencia de lo que se propuso, y
  la lectura de la cola la sigue mostrando.
- `CasoIdentificacionPorID` y la lectura de la cola:
  - Añadir `u.nota_resolucion` a `sqlCasosIdentificacion`, tanto en `base` como en el SELECT final.
  - Llevar la columna a `CasoIdentificacion.Nota`.
  - Para leer por id, reusar la sentencia con un filtro opcional `AND ($7 = '' OR u.id = $7)` y los
    escalones `[oni, manual, descartado]`, o escribir una consulta hermana.
  - Extraer el escaneo de una fila a un helper común.
  - Sin fila, devolver `ErrNoEncontrado`.
- `resumenExclusiones` (`reparto.go:71`): añadir `COUNT(*) FILTER (WHERE escalon = 'descartado')`,
  escaneado a `r.Descartados`, y actualizar el comentario de "los tres" a "los cuatro".

### 4.5 HTTP

En `internal/infraestructura/httpapi/`, en `casos_identificacion.go` o en un `resolucion_identificacion.go` nuevo:
- Interfaz `ResolucionIdentificacion { Resolver(ctx, aplicacion.SolicitudResolucion, actorID, actorNombre string) (aplicacion.CasoIdentificacion, error) }`,
  más `var _ ResolucionIdentificacion = aplicacion.ResolucionIdentificacion{}`.
- Un campo en `Casos` (el struct de `server.go`) y en `API`. Ruta, dentro del grupo `/identificacion`
  (`server.go:192`, que ya exige administrador): `ident.Post("/casos/{id}/resolucion", a.resolverCasoIdentificacion)`.
- Handler, copiando `resolverAlerta`:
  - 503 si el caso de uso no está cableado;
  - `http.MaxBytesReader` de 64 KiB;
  - decode de `{decision, obra_id, nota}`: un JSON roto (que no sea EOF) da 400;
  - `UsuarioDe` (sin sesión, 401) y `id := strings.TrimSpace(chi.URLParam(r, "id"))`. Los ids de uso
    son TEXT, no UUID: no validar UUID;
  - llamada con `usuario.ID` y `usuario.Nombre`. **Nunca** tomar el actor del cuerpo.
- Mapeo de errores:
  - 400: `identificacion.ErrNotaVacia`, `ErrNotaDemasiadoLarga`, `ErrDecisionInvalida` y `aplicacion.ErrObraInexistente`.
  - 404: `aplicacion.ErrNoEncontrado`, con "ese caso no existe".
  - 409: `identificacion.ErrCasoYaResuelto`, `identificacion.ErrCasoNoPendiente` (el mensaje nombra
    el estado actual) y `aplicacion.ErrAliasEnConflicto` (el mensaje nombra la obra del alias).
  - 401 por `aplicacion.ErrActorAusente`, como defensa.
  - Cualquier otro: 500, con log.
- 200 con `aCasoJSON(caso)`.
- `casoJSON` gana `Nota *string json:"nota"`: `null` si está vacía (caso pendiente) y el texto si no.
- Cablear en `cmd/api/main.go` (cerca de `:230`) y `cmd/lambda/main.go` (cerca de `:265`):
  `Resolucion: aplicacion.ResolucionIdentificacion{Repo: store, Bitacora: store, Unidad: store, Reloj: <el reloj que ya usan Anomalias/Procesos>}`.
  Ver §8.3 sobre `cmd/lambda/cableado_test.go`, que vigila el cableado sobre el AST.

### 4.6 Contrato y tipos del front

- En `api/openapi.yaml`, la ruta nueva `/identificacion/casos/{id}/resolucion` con `post`:
  - `tags: [identificacion]`, `operationId: resolverCasoIdentificacion`, `security: bearerAuth`,
    `x-required-roles: [administrador]`;
  - la descripción cuenta D1 a D3, D6 y que el actor sale de la sesión;
  - `requestBody` `ResolucionDeCaso`;
  - respuestas `200` (`CasoIdentificacion`, con ejemplos de asignar y descartar), `400`, `401`,
    `403`, `404` y `409`, este con las tres causas en la descripción.
- Schema nuevo `ResolucionDeCaso`:
  - `required: [decision, nota]`;
  - `decision` con `enum [asignar, descartar]`;
  - `obra_id` string, "obligatoria si decision=asignar; prohibida si descartar", validado por el servidor;
  - `nota` string con `minLength: 1` y `maxLength: 300`.
- `CasoIdentificacion`:
  - `estado` pasa a `enum [pendiente, asignado, descartado]`;
  - `nota` entra en `properties` y en `required` (el schema lista todos), como `nullable: true` y
    `maxLength: 300`;
  - la descripción de `descartado` dice: "una persona decidió que no es un uso del repertorio: no
    pondera y no sale en el listado ONI".
- `GET /identificacion/casos`: el parámetro `estado` añade `descartado`, y la descripción lo menciona.
- `Asiento`: su descripción añade los payloads de `identificacion.asignada` e `identificacion.descartada`.
- `npx @redocly/cli@1 lint --config api/redocly.yaml api/openapi.yaml` en verde.
- `npm --prefix web run contrato`, que regenera `web/src/contrato.d.ts`, y después `npm --prefix web test`
  (incluye `contrato:check`) y `npm --prefix web run typecheck`.

### 4.7 Documentación

- `docs/dominio/reglas-negocio.md`: regla nueva **R-36 "Resolución manual de un caso ONI"**.
  - Asignar y descartar; `descartado` no pondera y no es ONI; nota obligatoria de hasta 300
    caracteres; firmado; la cascada no lo pisa; alias con `quien` = usuario.
  - Estado: "Decidido por el equipo (sanmemu09, 2026-09-27); confirmar con el PO".
  - Fuente: RD 7.1, RD 9.5 (R-27), RD 13.8, ADR 0006 y ADR 0007.
  - Implementación: los módulos y pruebas nuevos.
  - Mencionar que R-18 sigue pendiente, así que hoy ONI y descartado pesan igual.
- `docs/dominio/matriz-reglas.md`: una fila de R-36 que enlace regla, artículo, módulo y pruebas, con
  los nombres reales de las pruebas.
- `docs/dominio/glosario.md`: las entradas `descartado` y, si falta, "caso de identificación".
- `docs/dominio/preguntas-cliente.md`: **P-22** "¿Qué significa descartar un caso ONI?", con la
  respuesta provisional (D1). Dueño: sanmemu09 y el PO. Estado: respondida provisionalmente.
- ADR: comprobar en `docs/decisiones/README.md` si una decisión con efecto monetario exige ADR (§8.14).
  Si lo exige, escribir `0022-descartar-caso-de-identificacion.md`. Hoy el último es el 0021.
- **Comentario obsoleto** en `postgres/identificacion.go:89-109`: reescribirlo para que diga lo que
  hace el SQL desde #169 (copia `obras.tipo` si `tipo_obra` viene vacío) y quitar la afirmación de
  "NO se rellena aquí". **Commit aparte** (`docs(identificacion): ...`), y mencionarlo en el reporte.

### 4.8 Fuera de alcance: no hacer

- Corregir o reasignar una decisión manual, o deshacer un descarte (D4).
- Resolución masiva, que la issue difiere.
- Tocar `alertas` desde este caso de uso (D2).
- Cambiar el detector `duplicado_registro` de #37 (§9).
- La reserva ONI y la liberación por RD 13.8.5 (#33/#34).
- El frontend de #39. Solo se regeneran los tipos.

---

## 5. Orden de ejecución (TDD) y commits sugeridos

1. **Migración con pruebas de esquema.** Siguiendo el estilo de `postgres/esquema_usos_test.go`, casos que la base rechaza:
   - `descartado` con obra;
   - `descartado` con `oni = true`;
   - `descartado` sin `resuelto_por`;
   - `manual` o `descartado` sin nota;
   - nota de 301 caracteres.

   Y casos que acepta: `descartado` válido y `manual` válido con nota. Actualizar los fixtures que
   insertan `manual` por SQL (§8.11).
2. **Dominio:** `resolucion.go` con pruebas de tabla, `EscalonDescartado` y `metricas.go`.
3. **Aplicación, lectura:** estados y nota en la cola, `reprocesable` y `ResumenUsosDeCanal.Descartados`, con sus pruebas.
4. **Aplicación:** el caso de uso `ResolucionIdentificacion` con dobles (§6.2).
5. **Postgres:** el adaptador y las pruebas de integración (§6.3).
6. **HTTP:** handler, ruta, cableado en `cmd/api` y `cmd/lambda`, y pruebas por el router (§6.4).
7. **Contrato:** OpenAPI, redocly, tipos del front y prueba de contrato (§6.5).
8. **Docs**, y el comentario obsoleto en commit aparte.
9. **Verificación completa** (§7) y los commits locales finales.

Estilo del mensaje, igual que el historial:
`feat(identificacion): resolucion manual de casos ONI, asignar y descartar (#175)`. Pueden ser uno o
varios commits.

---

## 6. Pruebas a escribir

### 6.1 Dominio (`resolucion_test.go`, de tabla)

- `oni` + asignar a una candidata: `manual`, obra, puntaje del candidato, `ONI=false` y la evidencia exacta.
- `oni` + asignar a una buscada: puntaje 0 y evidencia de "buscada".
- `oni` + descartar: `descartado`, sin obra y `ONI=false`.
- `manual` y `descartado`: `ErrCasoYaResuelto`.
- `pendiente`, `alias`, `id_global`, `difuso` y `excluido`: `ErrCasoNoPendiente`, y el error nombra el escalón.
- `ValidarDecision`: asignar sin obra (incluida una obra de solo espacios), descartar con obra, y una
  decisión desconocida o vacía dan `ErrDecisionInvalida`.
- `NormalizarNota`:
  - `""` y `"   "` dan `ErrNotaVacia`;
  - 300 runas con multibyte (por ejemplo `strings.Repeat("ñ", 300)`) pasan: prueba que se cuentan
    runas y no bytes;
  - 301 runas dan `ErrNotaDemasiadoLarga`;
  - los espacios de los extremos se recortan.
- Determinismo: dos llamadas iguales dan el mismo `Resultado`.
- `metricas_test.go`: `descartado` cuenta en `Descartadas`, fuera del denominador, y las categorías
  siguen cerrando contra `Total`.

### 6.2 Aplicación (con dobles)

**Dobles.**
- Repositorio en memoria.
- `UnidadDeTrabajo` que **emula el rollback**: copia el estado del doble al entrar y lo restaura si
  `fn` devuelve error.
- Bitácora que puede fallar.
- `Reloj` fijo.
- El doble del repositorio registra el **orden** de las llamadas.

**Casos de `ResolucionIdentificacion`.**
- Asignar a una candidata:
  - la escritura lleva el escalón `manual`, la obra, `ONI=false`, `ActorID`, `Cuando == reloj` y la nota recortada;
  - el alias se aprende con `quien = actorID`;
  - el asiento es `identificacion.asignada`, con ref `obra`/`obraID`, el actor, `Cuando` y el payload completo de §4.3;
  - la respuesta tiene `Estado=asignado`.
- Asignar a una buscada: payload con `candidata=false` y sin puntaje.
- Descartar:
  - el escalón `descartado` y sin obra;
  - **no** se llama a `Alias` ni a `GuardarAlias`;
  - el asiento es `identificacion.descartada`, con ref `uso`/`usoID`;
  - la respuesta tiene `Estado=descartado`.
- Alias:
  - ya existente hacia la misma obra: no se escribe y el payload lleva `aprendido=false`;
  - ya existente hacia otra obra: `ErrAliasEnConflicto` y **nada escrito**, ni UPDATE ni asiento;
  - carrera, donde `GuardarAlias` no escribe y la relectura da otra obra: `ErrAliasEnConflicto` y nada escrito;
  - uso sin par local: no hay alias y el payload lleva `alias: null`.
- Validación temprana, con cero llamadas al repositorio:
  - nota vacía o de solo espacios: `ErrNotaVacia`;
  - nota de 301 runas: `ErrNotaDemasiadoLarga`;
  - actor vacío o de solo espacios: `ErrActorAusente`;
  - decisión inválida.
- Estado del caso:
  - caso inexistente: `ErrNoEncontrado`;
  - caso `manual`: `ErrCasoYaResuelto`;
  - caso `alias`: `ErrCasoNoPendiente`.
- Obra inexistente: `ErrObraInexistente`, y nada escrito.
- **El asiento falla, así que nada está hecho:** error devuelto y el estado del doble igual que antes
  (ni UPDATE, ni alias, ni nota).
- Orden: `PeriodoDeUso`, luego `BloquearPeriodoDeUsos`, luego `CasoParaResolver`. El reloj se lee una
  sola vez, después del cerrojo.
- Mal cableado, con cualquier dependencia nil: error y ninguna unidad abierta.

**Otras pruebas de aplicación.**
- `identificacion_test.go`: `ResolverUsos` salta una fila `descartado` (ninguna llamada a
  `GuardarMatch`) y un escalón desconocido sigue fallando cerrado.
- `casos_identificacion_test.go`:
  - `estado=descartado` pide solo `[descartado]`;
  - por defecto pide los tres;
  - `completarCaso` con `descartado` fija el estado y `ultima_actualizacion = resuelto_en`;
  - `descartado` sin `resuelto_en` da error;
  - `Nota` viaja.

### 6.3 Integración con Postgres real (testcontainers, patrón de `postgres/main_test.go` y `testhelp/`)

- **Asignar a una candidata, con el `Store` real para todo:**
  - fila en `manual`, con obra, `oni=false`, `resuelto_*` y `nota_resolucion`;
  - `tipo_obra` copiado de `obras.tipo` cuando venía vacío;
  - `candidatos_match` intacto;
  - alias con `quien` = el id del usuario;
  - el asiento aparece en `Store.De("obra", obraID)` y en `Auditoria.HistorialDeObra`.
- **Asignar a una buscada:** lo mismo, con el payload `candidata=false`.
- **Descartar:**
  - fila en `descartado`, `oni=false`, sin obra, con `resuelto_*` y nota;
  - **fuera de `oni_publico`**;
  - no cuenta en `pendientes` de `ListarCasosIdentificacion` y aparece con `estado=descartado`;
  - no sale en `UsosDeCanal`;
  - cuenta en `resumenExclusiones.Descartados`.
- **Concurrencia:** dos goroutines resuelven el mismo caso, por ejemplo asignar contra descartar.
  Exactamente una gana, la otra recibe `ErrCasoYaResuelto`, y queda **un solo** asiento. Patrón:
  `procesos_concurrencia_integracion_test.go` y `alertas_cerrojo_test.go`.
- **Alias reutilizado:**
  - se asigna un uso de caracol con `id_ficha=X`;
  - llega un **reporte nuevo** con otro uso del mismo `id_ficha=X`;
  - `ResolverUsos(periodo)` lo resuelve en el escalón `alias` hacia la obra, sin ONI.
- **La cascada no pisa decisiones humanas:** `ResolverUsos` sobre un periodo con una fila `manual` y
  una `descartado` deja las dos intactas.
- **El asiento falla, así que nada está hecho, contra la base real:**
  - `Store` real para el repositorio y la unidad, con una **bitácora doble que falla** (el caso de uso
    la recibe por separado);
  - después, la fila sigue en `oni`, sin nota y sin `resuelto_*`, y no hay alias.
- **Cerrojo de periodo (D3):**
  - otra transacción sostiene `pg_advisory_xact_lock` del periodo;
  - la resolución espera, y termina cuando esa transacción suelta el cerrojo.
  - Patrón: `alertas_cerrojo_test.go`.
- **Migración:** si hay pruebas de goose up y down (`migraciones_test.go`), que sigan pasando; un
  `down` convierte un `descartado` en `oni`.

### 6.4 HTTP (router chi con dobles, patrón de `alertas_test.go` y `casos_identificacion_test.go` de httpapi)

- 200 al asignar: cuerpo con `estado=asignado`, `obra_asignada`, `resuelto_por {id, nombre}`,
  `resuelto_en` y `nota`. El doble recibió `actorID` y `actorNombre` **de la sesión**; un campo
  `actor` metido en el cuerpo se ignora.
- 200 al descartar: `estado=descartado` y `obra_asignada: null`.
- 400:
  - JSON roto;
  - nota ausente, vacía, de solo espacios o de más de 300;
  - decisión inválida;
  - asignar sin `obra_id`;
  - descartar con `obra_id`;
  - obra inexistente.
- 401 sin sesión. 403 con los roles `distribucion`, `auditor`, `contabilidad` y `titular`.
- 404 si el caso no existe.
- 409 por las tres causas, cada una con su mensaje.
- La respuesta no lleva campos de dinero ni medidas. Copiar la prueba que ya hizo #176.
- Si `rbac_test.go` lleva un inventario de rutas y roles, añadir la ruta nueva (§8.8).

### 6.5 Contrato

- Prueba Go de contrato, siguiendo `httpapi/alertas_contrato_test.go`: la respuesta real del handler
  cuadra con `CasoIdentificacion` del OpenAPI, y el cuerpo de la petición con `ResolucionDeCaso`.
- redocly lint en verde, `contrato.d.ts` regenerado y `contrato:check` en verde.

---

## 7. Verificación: comandos exactos y trampas del entorno

Máquina Windows. Regla explícita del usuario: al terminar **cada** cambio, correr la **suite
completa** con `-race`, sin subconjuntos "porque en Windows fallan".

**Trampa grave.** Docker Desktop puede servir una vista **cacheada y obsoleta** de un worktree recién
creado bajo `/mnt/c/...`. Ya dio un "0 issues" falso. La salida: **commitear en local**, clonar la
rama al sistema de archivos nativo de WSL y correr los contenedores desde ese clon.

```bash
# Desde el Bash de Windows. Una sola linea por comando, con comillas en los PATH.
wsl.exe -d Ubuntu -- bash -lc 'rm -rf ~/intela-175 && git clone --branch feature/175-resolucion-manual-identificacion /mnt/c/Users/Usuario/Documents/REPOSITORIOS/Intela ~/intela-175'

# Suite completa con -race, dentro de golang:<version de go.mod> (hoy go 1.24.0), con el socket de Docker para testcontainers.
# -p 1 evita "too many clients" cuando varios paquetes levantan su Postgres a la vez.
wsl.exe -d Ubuntu -- bash -lc 'cd ~/intela-175 && docker run --rm --network host -v "$PWD":/app -w /app -v /var/run/docker.sock:/var/run/docker.sock golang:1.24 sh -c "go test ./... -race -count=1 -p 1"'

# Lint y frontera. La version de golangci-lint es la fijada en CI (memoria: v2.13.1; confirmar en .github/workflows/ci.yml).
wsl.exe -d Ubuntu -- bash -lc 'cd ~/intela-175 && docker run --rm -v "$PWD":/app -w /app golangci/golangci-lint:v2.13.1 golangci-lint run ./...'
wsl.exe -d Ubuntu -- bash -lc 'cd ~/intela-175 && docker run --rm -v "$PWD":/app -w /app golangci/golangci-lint:v2.13.1 golangci-lint run --enable-only=depguard ./...'
```

- Antes de fiarse de un resultado, comprobar que el contenedor ve el código nuevo: un `cat` o `ls -la`
  de un fichero nuevo **desde dentro** del contenedor.
- `make verificar` (`tidy build vet fmt-check test`), correrlo en el clon nativo de WSL. Si el host no
  tiene Go, dentro del contenedor `golang:1.24` con el mismo montaje.
- Front, desde el worktree de Windows: `npm --prefix web run contrato`, `npm --prefix web test` y
  `npm --prefix web run typecheck`.
- `npx @redocly/cli@1 lint --config api/redocly.yaml api/openapi.yaml`.
- Tras cada commit nuevo, `git -C ~/intela-175 pull` (o volver a clonar) antes de repetir.

---

## 8. Lo que falta comprobar (no se verificó en la sesión de preparación)

1. **Nombres de las restricciones de `usos`.** Ver si 00010, 00011 o 00014 renombraron o recrearon
   `usos_escalon_check`, `uso_resuelto_tiene_obra` o `manual_tiene_autor`:
   `grep -n "CONSTRAINT\|CHECK" migrations/0001[0-4]*.sql`. Ajustar los `DROP` de la migración.
2. **Filas `manual` en producción.** Se esperan cero, porque ninguna ruta escribía `manual`. El
   relleno defensivo cubre el caso contrario. Sin acceso a producción, dejarlo dicho en la cabecera.
3. **Cableado en `cmd`:**
   - qué reloj reciben hoy `Anomalias` y `Procesos` en `cmd/api/main.go` y `cmd/lambda/main.go`;
   - qué vigila `cmd/lambda/cableado_test.go` sobre el AST. Si exige cada caso de uso en los dos
     binarios, añadir el nuevo a su lista.
4. **Colisión de nombres de métodos en `*postgres.Store`:** `PeriodoDeUso`, `BloquearPeriodoDeUsos`,
   `CasoParaResolver`, `TituloDeObra`, `GuardarResolucionManual` y `CasoIdentificacionPorID`.
   `grep -rn "func (s \*Store) <Nombre>" internal/infraestructura/postgres`. Si alguno existe con otra
   firma, cambiar el nombre.
5. **`columnasUso` y `escanearUso`** (`ingesta.go:27-36`): confirmar que la proyección trae `escalon`,
   `evidencia`, `ids_fuente`, `titulo_original` y `reporte_id`, y si se puede prefijar con `u.` para el JOIN.
6. **Infraestructura de pruebas de integración.** Cómo arrancan el contenedor y siembran usuarios,
   obras, reportes, usos y candidatos: `postgres/main_test.go`, `postgres/testhelp/`, y los helpers de
   `postgres/casos_identificacion_test.go` (#176) y `postgres/identificacion_test.go`. Reusar, no duplicar.
7. **Patrones de concurrencia y cerrojo:** `postgres/procesos_concurrencia_integracion_test.go` y
   `postgres/alertas_cerrojo_test.go`.
8. **`httpapi/rbac_test.go`:** ver si enumera todas las rutas con sus roles, y si hay una prueba que
   cruce rutas de chi contra el OpenAPI (`grep -rn openapi internal/infraestructura/httpapi/*_test.go`).
   Si existe, la ruta nueva tiene que entrar.
9. **Prueba de contrato:** el patrón exacto de `httpapi/alertas_contrato_test.go` y la configuración
   de reglas de `api/redocly.yaml`, que puede exigir ejemplos y `operationId`.
10. **El schema `Error` del OpenAPI:** ver si solo lleva `error: string`. Si pudiera llevar un código
    máquina, el front distinguiría las tres causas de 409. Si no, se quedan distinguidas por el mensaje (§9).
11. **Fixtures de prueba que insertan `escalon='manual'` por SQL.** El CHECK nuevo de nota los rompe;
    añadirles `nota_resolucion`:
    - `internal/infraestructura/postgres/casos_identificacion_test.go`;
    - `internal/infraestructura/postgres/identificacion_test.go`;
    - `internal/infraestructura/postgres/identificacion_unidad_test.go`;
    - `internal/infraestructura/postgres/origen_test.go`.

    Los de `aplicacion` y `dominio` usan dobles y el CHECK no los afecta, aunque también mencionan `manual`.
12. **Pruebas que comparan `ResumenUsosDeCanal` entero:** con el campo nuevo a cero siguen pasando,
    pero conviene añadir el caso `descartado` donde se cuentan los otros tres.
13. **Entorno:** la versión de Go en `go.mod` (hoy 1.24.0) y la de golangci-lint en `.github/workflows/ci.yml`,
    para elegir las imágenes. Los scripts de `web/package.json` ya se verificaron (§3).
14. **`docs/decisiones/README.md`:** si pide ADR para una decisión con efecto monetario (D1), escribir el 0022.
15. **PR #87** (`00022_oni_publicacion.sql`): ver si toca `usos` u `oni_publico`. Si #87 mergea antes,
    renumerar a 00023 (la guarda `internal/infraestructura/migraciones/numeracion` lo exige) y revisar
    que su vista siga excluyendo `descartado` (con `oni=false` debería).
16. **`traducirError`:** confirmar que convierte `pgx.ErrNoRows` en `aplicacion.ErrNoEncontrado`, y
    cómo trata las violaciones de CHECK, para que el `ON CONFLICT` y los CHECK no terminen en un 500 mudo.
17. **`explicar.go:313-318`** (`rangoDeCerteza`) no incluye `descartado`. Es correcto porque un
    descartado nunca pondera. Confirmar que ninguna ruta de Explicar lista usos sin obra.
18. **PR #181** (semilla de casos, abierto): no hay choque esperado, porque no trae migración y toca
    `semilla` y docs. Si mergea antes, sus casos sembrados sirven de datos para probar a mano de punta a punta.

---

## 9. Riesgos y seguimientos: reportar, no arreglar aquí

- **Duplicados que se descartan.** El detector `duplicado_registro` de #37 (`anomalias/detectar.go:107`)
  no mira el escalón. Descartar una fila duplicada **no apaga** su alerta crítica; hay que resolverla
  en `/alertas`. Con `excluido` pasa lo mismo hoy. Candidato a issue.
- **La reserva ONI (R-18) sigue pendiente.** Hoy descartado y ONI pesan igual, cero. La diferencia
  llega con #33/#34.
- **Hermanos con el mismo id.** Resolver un uso aprende el alias, pero los demás usos pendientes con
  el mismo id siguen en la bandeja hasta la siguiente `ResolverUsos`. No se propaga en el acto, porque
  la resolución masiva está diferida. Avisar a #39.
- **Tres causas de 409** distinguidas solo por el mensaje: ya resuelto por una persona, ya no
  pendiente, y alias en conflicto. Avisar a #39, que ya diseñó "Recargar caso".
- **Choque de migración 00022 con #87** (§4.1).
- **El comentario obsoleto** de `GuardarMatch` (§4.7).

---

## 10. Definición de hecho y reporte final

**Hecho cuando:**
- Los criterios de aceptación de §1 se cumplen con D1 a D8.
- La suite completa con `-race` pasa en verde (§7). `golangci-lint` y `--enable-only=depguard` dan 0 issues.
- `make verificar` pasa en verde.
- redocly en verde y `contrato.d.ts` regenerado, con `npm --prefix web test` y `typecheck` en verde.
- La documentación de §4.7 está al día.
- Hay commits **locales** en `feature/175-resolucion-manual-identificacion`, **sin push**.

**Reporte final** para sanmemu09, a trasladar a @Killgreck:
- Qué se hizo, con los ficheros.
- Las decisiones D1 a D8 y lo provisional: D1 está pendiente de confirmar con el PO.
- La evidencia de las pruebas: los comandos y sus salidas resumidas.
- Los seguimientos de §9.
- Lo que haya quedado de §8 sin comprobar, dicho explícitamente.

**Revisión:** cuando termine, sanmemu09 puede pedir en Claude Code la revisión final con el agente
`revisor-intela` (Sonnet, esfuerzo alto), indicándole que **no** haga push.
