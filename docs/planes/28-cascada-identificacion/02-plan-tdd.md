# 02 — Plan de ejecución TDD: cascada de identificación (#28)

> Complementa a `01-design.md` (el diseño, las decisiones D1–D9 y los límites de alcance). Este
> documento fija el **orden de trabajo**, los **tests** (qué se prueba con dobles y qué contra
> PostgreSQL real) y el **orden de commits**. Todo identificador de código en inglés; prosa en
> español. Rama: `feature/28-identification-cascade` desde `main` (46aa720).

## Convenciones del repo que rigen cada archivo nuevo

- Dominio: `internal/dominio/identificacion/` no importa nada externo (depguard): sin `time`, sin
  pgx, sin `aplicacion`. `shopspring/decimal` sí está permitido (ya lo usa `tipos.go`).
- Aplicación: orquesta y declara; nada de infraestructura. Los errores viajan con `errors.Is` y
  envoltura `%w`.
- Adaptador postgres: patrón de `internal/infraestructura/postgres/doc.go` (aserción
  `var _ aplicacion.X = (*Store)(nil)` antes del SQL, `traducirError`, `EnTransaccion` solo si el
  puerto lo pide — aquí no).
- Tests del adaptador: contra PostgreSQL real con `testhelp.Pool(t)`, siembra por SQL directo
  (`ejecutar(...)`, estilo `semilla_test.go`), **nunca** `t.Parallel()` con `Pool`.
- Comentarios en Go: los del repo mezclan español para lo explicativo; mantener el mismo tono
  (los comentarios de dominio explican el POR QUÉ, en español, sin tildes según estilo del repo —
  mirar los ficheros vecinos y copiar su tono).
- `gofmt`, `go vet`, `golangci-lint` corren solos; no pelearse con el formato.

---

## Fase 1 — Dominio puro + tests (red → green → refactor)

### 1.1 `internal/dominio/identificacion/cascada.go` (nuevo)

Contenido exacto en `01-design.md` §5.1: constantes `EscalonAlias`, `EscalonIDGlobal`,
`EscalonExcluido`; tipo `IDGlobal` con `IDA/EIDR/IMDB` y `OrdenIDGlobal`; tipo
`FuentesExcluidas` con `Excluye`; tipo `Consulta`; función `Resolver(e Entrada, c Consulta,
excluidas FuentesExcluidas) Resultado`.

Casos puros que DOMINIO ya resuelve, en la semántica de D3:
- `Resolver` no llama a nada externo y no puede fallar: devuelve `Resultado`, nunca error.

### 1.2 `internal/dominio/identificacion/cascada_test.go` (nuevo) — tabla

**Fixture**: `entradaValida()` → `Entrada{Fuente: "caracol", TipoID: "id_ficha", ValorID:
"871732", IDA: "", EIDR: "", IMDB: "tt0100001", Titulo: "La Casa de las Dos Palmas"}`; `obra :=
"obra-45"`; helpers para ajustar.

Casos (tabla, nombre → mutación):

| # | Caso | Entrada/Consulta | Resultado esperado |
| --- | --- | --- | --- |
| 1 | **alias hit → escalón 1, confianza máxima** | entrada con par; `Consulta{AliasObraID: "obra-45"}` | `Escalon == "alias"`, `ObraID == "obra-45"`, `Puntaje == 1`, `ONI == false`, `Evidencia` contiene `"alias caracol id_ficha=871732 -> obra-45"` |
| 2 | **id global hit → escalón 2** | entrada con `IMDB` poblado; `Consulta{IDGlobalObraID: "obra-45", IDGlobalCual: IMDB}` | `Escalon == "id_global"`, `ObraID == "obra-45"`, `Puntaje == 1`, `ONI == false`, `Evidencia` contiene `"imdb tt0100001 -> obra-45"` |
| 3 | **id poblado sin match → no resuelto (cae al siguiente escalón)** | `Consulta{IDGlobalObraID: "", IDGlobalCual: IMDB}` | `Resultado{}`: `ObraID == ""`, `Escalon == ""`, `ONI == false` |
| 4 | **el alias manda sobre el id global** (ambos responderían: no puede pasar en producción —el caso de uso no sondea el 2 si el 1 pegó—, pero el dominio define el orden) | `Consulta{AliasObraID: "obra-45", IDGlobalObraID: "obra-99", IDGlobalCual: IDA}` | escalón `alias`, obra `obra-45` |
| 5 | **filtro de repertorio excluye** | entrada de `Fuente: "noticias-24h"`, `FuentesExcluidas{"noticias-24h"}` | `Escalon == "excluido"`, `ObraID == ""`, `ONI == false`, `Evidencia` contiene `"fuera de repertorio"`; **sin importar** lo que traiga `Consulta` (caso con alias pegado también excluye) |
| 6 | **fuente dentro del repertorio no se excluye** | misma entrada con set que no la contiene | no excluida (si alias pega → escalón 1) |
| 7 | **entrada sin datos** (sin par, sin globales, `Consulta` vacía) | `Entrada{Fuente:"caracol", Titulo:"X"}` | `Resultado{}` |
| 8 | **evidencia y puntaje deterministas** | repetir caso 1 dos veces | mismo string y mismo decimal (comparar con `Equal`, no `==`; igualdad estructural del `Resultado`) |

Aserciones: comparar campos con `if`, como los tests vecinos (`obra_test.go`), o `reflect.DeepEqual`
donde el `Resultado` entero sea estable — no usar `==` con `decimal`.

### 1.3 Comprobación de la fase

```bash
go test ./internal/dominio/identificacion/
gofmt -l internal/dominio/identificacion/   # vacío
go vet ./internal/dominio/identificacion/
```

---

## Fase 2 — Adaptador `RepositorioIdentificacion` + tests (PostgreSQL real)

> Orden del encargo: dominio → adaptador → caso de uso. El adaptador no depende del caso de uso y
> así cada fase se cierra con verde.

### 2.1 `internal/infraestructura/postgres/identificacion.go` (nuevo)

Cuatro métodos sobre `*Store`, SQL exacto en `01-design.md` §5.5. Puntos de atención:
- `var _ aplicacion.RepositorioIdentificacion = (*Store)(nil)` justo tras los imports.
- `Alias` y `ObraPorIDGlobal` con `QueryRow` + `traducirError` (contexto que nombre la consulta).
- `ObraPorIDGlobal`: fast-path los tres vacíos → `ErrNoEncontrado` envuelto; `ORDER BY id LIMIT 1`.
- `GuardarAlias`: `ON CONFLICT DO NOTHING`; `quien == ""` → `nil`.
- `GuardarMatch`: `UPDATE` con `NULLIF` y `oni = ($2 <> '')`; comprobar `RowsAffected() == 0` →
  `ErrNoEncontrado` envuelto (patrón de `Store.Actualizar` en `catalogo.go`).
- `puntaje` viaja como `decimal.Decimal` directo a pgx (mecanismo verificado en #72).

### 2.2 `internal/infraestructura/postgres/identificacion_test.go` (nuevo) — contra PostgreSQL real

Helpers locales al fichero (estilo de `semilla_test.go`, comentarios del porqué de cada valor):

```go
func sembrarIdentificacion(t *testing.T) (*Store, *pgxpool.Pool) // Pool + SQL directo:
//   obras: obra-45 (imdb='tt0100001'), obra-12 (ida='IDA-000001'), obra-77 (ida='', eidr='', imdb='')
//          (INSERT con genero/anio/tipo válidos, como semilla_test.go)
//   reporte: rep-1 (fuente 'caracol', periodo '2024', sha256 64 hex, clave_objeto, nbytes>0)
//   usos: u-1 y u-2 pendientes con ids_fuente 'id_ficha=871732\nimdb=tt0100001', u-3 pendiente sin ids
//   (los INSERT de usos llevan todos los NOT NULL: id, reporte_id, fuente, titulo, modalidad)
func nuevaObraGlobal(t *testing.T, id string, ajustar ...func(*repertorio.Metadatos)) repertorio.Obra
func insertarUsoSQL(t *testing.T, pool *pgxpool.Pool, u aplicacion.UsoPersistido) // SQL directo con
//   NULLIF('',''), escalon 'pendiente', oni TRUE, puntaje 0, emisiones 1 — los valores que pone #72
```

Casos del adaptador (cada uno con `s := sembrarIdentificacion(t)`; asserts por SELECT directo
cuando el método no devuelva lo que hay que mirar):

**Alias**
- A1. sin fila → error `errors.Is(err, aplicacion.ErrNoEncontrado)` **y** el mensaje nombra la
  consulta (`"alias de ..."`).
- A2. con fila sembrada en `alias_obra` (`('caracol','id_ficha','871732','obra-45',NULL)`) →
  devuelve `obra-45`.

**GuardarAlias**
- G1. inserta y se lee de vuelta (SELECT `fuente,tipo_id,valor,obra_id,quien`).
- G2. repetir el mismo par → **sin error** (idempotente) y una sola fila.
- G3. repetir el mismo par con OTRA obra → sin error y la fila **sigue apuntando a la obra
  original** (DO NOTHING no pisa: criterio de D5).
- G4. `valor` vacío → error (CHECK `btrim(valor) <> ''`) **envuelto**, sin centinela nuevo: solo
  `err != nil` y el mensaje trae la causa.
- G5. `quien == ""` → columna NULL (SELECT con `quien IS NULL`).

**ObraPorIDGlobal**
- O1. match por `ida` → `obra-12`; por `imdb` → `obra-45`; por `eidr` idem con obra sembrada.
- O2. sin match → `ErrNoEncontrado` con contexto.
- O3. los tres vacíos → `ErrNoEncontrado` (sin tocar la base; el comportamiento se ve aquí).
- O4. dos obras con el mismo `imdb` sembradas → devuelve la de menor `id` (`ORDER BY id`):
  determinismo (D9).
- O5. combinación: llamar con `ida` de obra-12 **y** `imdb` de obra-45 → devuelve la que cumpla
  cualquiera (una sola fila; si ambas casan, `ORDER BY id` decide).

**GuardarMatch**
- M1. UPDATE feliz: sembrar `u-1` pendiente; `GuardarMatch("u-1", resultado alias)` → SELECT de la
  fila: `obra_id='obra-45'`, `escalon='alias'`, `evidencia` exacta, `puntaje = 1` (comparar con
  `Equal`), `oni=false`, `resuelto_por IS NULL`, `resuelto_en IS NULL`.
- M2. resultado con `ObraID == ""` y `Escalon == "oni"` (lo usará #32) → fila queda `oni=true`,
  `obra_id IS NULL` — el CHECK `uso_resuelto_tiene_obra` lo admite.
- M3. `usoID` inexistente → `ErrNoEncontrado` (0 filas afectadas no es silencio).
- M4. resultado con escalón fuera del CHECK (p. ej. `Escalon == "excluido"`, valor que el dominio
  nunca persiste) → error de constraint envuelto (`err != nil`, mensaje con causa) — prueba que el
  adaptador no traga estados que el esquema no conoce.
- M5. `Resultado{Escalon:"manual", ObraID:"obra-45"}` sin autor/instante → error de constraint
  (`manual_tiene_autor`): la resolución manual no entra por este puerto en este issue.
- M6. `GuardarMatch` con `obra_id` inexistente (`ObraID: "obra-inexistente"`) → error de FK
  envuelto.

### 2.3 Comprobación de la fase

```bash
go test -race -count=1 ./internal/infraestructura/postgres/   # con Docker (testhelp)
go test -short ./internal/infraestructura/postgres/           # sin Docker: debe saltar, no fallar
```

---

## Fase 3 — Caso de uso `ResolverUsos` + tests (dobles) + integración

### 3.1 `internal/aplicacion/identificacion.go` (nuevo)

Contenido exacto en `01-design.md` §§5.2–5.4: `quienCascada`, struct `ResolverUsos` (campos
`Usos RepositorioIngesta`, `Identificacion RepositorioIdentificacion`, `FueraDeRepertorio
identificacion.FuentesExcluidas`), método `ResolverUsos(ctx, periodo) (int, error)`, privados
`resolverFila`, `entradaDesdeUso`, helpers de parser y de selección de par canónico
(`parCanonicoPorFuente`), `valorGlobal`/`idaEidrImdb`.

Importa `internal/dominio/identificacion` y `errors`/`fmt`/`strings`/`slices` (stdlib; permitido en
aplicación). **No** importa `time`.

### 3.2 `internal/aplicacion/identificacion_test.go` (nuevo) — unidad con dobles

Dobles locales (estilo `catalogoFalso`, en el mismo fichero):

```go
// ingestaFalsa: devuelve la lista que se le dé y cuenta llamadas.
type ingestaFalsa struct {
    usos   []UsoPersistido      // lo que devuelve UsosDePeriodo
    usosLlamadas []string       // periodos pedidos
}
// identificacionFalsa: mapa de alias, mapa de globales, y registros de llamadas
// para poder afirmar orden y contenido de GuardarAlias/GuardarMatch.
type identificacionFalsa struct {
    alias   map[string]string        // clave "fuente|tipo|valor" → obraID
    porIDGlobal map[string]string    // clave "ida|eidr|imdb|VALOR" → obraID (o ausente = ErrNoEncontrado)
    err     error                    // si va, todos los sondeos fallan con esto (fallo de red)
    guardadosAlias []struct{...}
    guardadosMatch []struct{ UsoID string; R identificacion.Resultado }
}
```

Los cinco métodos de `RepositorioIngesta` que exige la interfaz (`GuardarReporte`, `GuardarUsos`,
`UsosSinResolver`, `UsosDePeriodo`, `UsoPorID`) se implementan en el doble; solo `UsosDePeriodo`
tiene comportamiento (los demás devuelven cero/sin error — nunca se llaman en este caso de uso).

Helpers: `usoPendiente(id, fuente, idsFuente string) UsoPersistido` (escalon `"pendiente"`,
`ONI true`, `Modalidad: reparto.TV`, medidas a cero, emisiones 1 — los valores de ingesta);
`correr(t, ing, idf, excluidas, usos...) (int, error)`.

Casos (unidad; **sin PostgreSQL**):

| # | Caso | Setup | Esperado |
| --- | --- | --- | --- |
| U1 | **pendientes del periodo se resuelven por alias** | alias `caracol\|id_ficha\|871732 → obra-45`; un uso pendiente | `ResolverUsos` → 1; `guardadosMatch` = 1 con `Resultado` escalón `alias`/obra `obra-45`; `guardadosAlias` vacío (el escalón 1 no aprende, D5) |
| U2 | **resuelve por id global y aprende el alias antes del match** | sin alias; obra por `imdb`; uso con `ids_fuente "id_ficha=871732\nimdb=tt0100001"` | 1; `guardadosAlias` = 1 `("caracol","id_ficha","871732","obra-45","cascada")` y **ocurre antes** que el `GuardarMatch` (el doble registra orden); match con escalón `id_global` y evidencia que nombra `imdb` |
| U3 | **el alias aprendido cortocircuita la próxima vez** | igual que U2 pero el doble de alias ya contiene el par (efecto de la corrida anterior) | escalón `alias`; **cero** llamadas a `ObraPorIDGlobal` (el doble lo cuenta) |
| U4 | **fila sin datos (sin par ni globales)** | uso con `ids_fuente ""` | 0 resueltas; sin alias, sin matches; la fila no se toca |
| U5 | **no-repertorio excluida: ni sondeos ni escrituras** | `FueraDeRepertorio{"canal-deportes"}`; uso de esa fuente que SÍ casaría por alias | 0; ni `Alias` ni `GuardarMatch` ni `GuardarAlias` llamados (el doble cuenta sondeos) |
| U6 | **ya resueltas/no pendientes se ignoran** | usos con `escalon` `alias`, `oni`/`manual`, y una pendiente | solo se procesa la pendiente |
| U7 | **fallo de red en Alias aborta la corrida** | `identificacionFalsa.err = errors.New("red caida")` | error devuelto envuelto; **cero** escrituras; nada marcado (D8) |
| U8 | **fallo de red en ObraPorIDGlobal aborta** | sin alias, error en el sondeo de global | idem |
| U9 | **sin alias y sin globales que casen → no resuelta** | entrada con par e `imdb` poblado, sin match | 0; sin escrituras; la fila queda como estaba (para #32) |
| U10 | **`GuardarMatch` falla → propaga** | `identificacionFalsa` con error solo en el match | error; el número devuelto no cuenta la fila |
| U11 | **se procesa el periodo pedido** | `ingestaFalsa` registra el periodo | `UsosDePeriodo` llamado con el mismo string; resueltas correctas |
| U12 | **parser de ids_fuente** (tabla): `"id_ficha=871732\nimdb=tt0100001"` → par `(id_ficha,871732)` + `IMDB`; `"show_id=80141259\nseries_id=123\nnetflix_id=456"` con fuente `netflix` → par `(show_id,80141259)`; clave desconocida `"foo=bar"` ignorada; línea rota `"solo-texto"` ignorada; `"k="` y `"=v"` ignoradas; repetida `"a=1\na=2"` → gana `a=2`; `""` → sin par y sin globales; fuente sin mapeo con una sola clave local → esa; con dos (`"x=1\ny=2"`, fuente `"otra"`) → la alfabética (`x`) | vía `entradaDesdeUso` (o helper expuesto solo para tests si se prefiere) |

### 3.3 Integración de `ResolverUsos` contra PostgreSQL real

**Dónde**: `internal/infraestructura/postgres/identificacion_test.go` (el paquete postgres puede
importar `aplicacion`; `aplicacion` no puede importar infraestructura — por eso la integración vive
aquí, con el `Store` real de `RepositorioIdentificacion`).

**Por qué un doble para la lectura**: `RepositorioIngesta` no tiene adaptador en main (#72 abierta);
este issue no lo implementa (límite duro). La siembra de `usos` es SQL directo; la lectura del
periodo la hace un doble local mínimo (`ingestaDePrueba` en el fichero de test, devuelve lo que la
siembra insertó). Todo lo que es **escritura y sondeo** de este issue corre contra PostgreSQL real.

Casos de integración:

| # | Caso | Esperado |
| --- | --- | --- |
| I1 | **criterio 1 de la issue**: sembrar catálogo + `alias_obra` + usos pendientes → `ResolverUsos("2024")` → la fila con alias queda `escalon='alias'`, `obra_id` correcto, `oni=false`, `puntaje=1`, evidencia exacta (SELECT directo) |
| I2 | **criterio 2**: fila sin alias pero con `imdb` que casa → queda `escalon='id_global'` con su evidencia **y** `alias_obra` ganó una fila `(fuente,tipo,valor,obra,quien='cascada')` (SELECT) |
| I3 | **criterio 3 (cortocircuito)**: tras I2, sembrar un segundo uso con el MISMO par de fuente (otra emisión) y correr de nuevo → el nuevo uso queda `escalon='alias'` (el alias aprendido cortocircuitó); el uso viejo no se re-procesa (sigue `id_global`) |
| I4 | **criterio 4 (repertorio)**: uso de fuente en `FueraDeRepertorio` → tras la corrida la fila sigue `escalon='pendiente'`, `oni=true`, sin obra, y `alias_obra` no ganó filas de esa fuente (excluida ≠ ONI: la cascada no la marcó; tampoco la resolvió) |
| I5 | **criterio 5 (lo no resuelto se queda)**: uso con par e `imdb` sin match en catálogo → sigue `escalon='pendiente'` (insumo de #32), sin escrituras |
| I6 | **idempotencia**: correr I1 dos veces → mismas filas finales; `alias_obra` sin duplicados; segundo `ResolverUsos` devuelve 0 (no hay pendientes nuevas) |

Aserciones siempre por SELECT directo contra `pool` (las filas finales son la verdad), con `Equal`
para `puntaje`.

### 3.4 Comprobación de la fase

```bash
go test ./internal/aplicacion/
go test -race -count=1 ./...          # con Docker: toda la suite (dominio+aplicacion+postgres)
go test -short ./...                  # sin Docker: solo unidad
```

---

## Fase 4 — Limpieza, documentación y verificación final

Contenido del último commit (o del PR si el revisor lo prefiere en varios):
- **Corregir la skill** `.claude/skills/matching-de-obras/SKILL.md`: la sección «Los IDs de fuente
  si sirven como alias» describe `alias_obra(fuente, tipo_id, valor_id, obra_id, confianza,
  resuelto_por, resuelto_en)`; el esquema real (00001) es `alias_obra(fuente, tipo_id, valor,
  obra_id, quien, aprendido)`. Ajustar a la realidad y anotar que `quien`/`aprendido` son el rastro
  (la confianza máxima de los escalones 1–2 se guarda como `usos.puntaje`, no en `alias_obra`).
- Si la revisión lo pide (y solo entonces): nota en la página 2 del `.drawio` sobre la arista del
  módulo Identificación. No tocarlo por iniciativa propia (ADR 0012: no se verifica; las ediciones
  cosméticas ya rompieron cosas).
- `docs/planes/28-cascada-identificacion/`: **no tocar** (es el plan; queda en la rama como
  registro). Tampoco se toca `.gitignore` (modificación local de tooling).

Verificación completa (la puerta de CI + lo que pide la issue):

```bash
go mod tidy                                  # debe quedar sin diff (go.mod/go.sum intactos)
gofmt -l .                                   # vacío
go vet ./...
go build ./...
go test -race -count=1 ./...                 # con Docker (testhelp)
golangci-lint run ./...                      # v2.13.1 (la que fija lint-go.yml)
golangci-lint run --enable-only=depguard ./...   # la frontera, aislada
make verificar
```

---

## Orden de commits (rama `feature/28-identification-cascade`)

Mensajes en inglés, estilo del repo (`feat(ámbito): ...`, `docs(...)`). Un commit por fase; cada
uno deja el árbol en verde:

1. `feat(identificacion): cascade escalon 1-2 pure logic with repertoire filter` — `cascada.go` +
   `cascada_test.go` (Fase 1).
2. `feat(postgres): RepositorioIdentificacion adapter (alias, guardar alias, id global, match)` —
   `identificacion.go` + `identificacion_test.go` (Fase 2: adaptador puro con sus casos A/G/O/M).
3. `feat(aplicacion): ResolverUsos use case with alias learning` — `identificacion.go` +
   `identificacion_test.go` en `aplicacion` (Fase 3, unidad U1–U12) **y** los casos de integración
   I1–I6 (viven en el paquete postgres porque combinan el `Store` real con el caso de uso; entran
   aquí, con el commit 3, porque prueban `ResolverUsos`, no el adaptador).
4. `docs: align matching skill alias table with real schema` — la corrección de la skill (Fase 4).

Regla: no mezclar fases en un commit; si un test de una fase posterior obliga a retocar código de
una anterior, el retoque viaja en el commit de la fase que lo necesita y se explica en el mensaje.

## Qué se prueba con dobles y qué contra PostgreSQL real (resumen)

| Nivel | Archivo | Contra qué |
| --- | --- | --- |
| Dominio puro (tabla) | `dominio/identificacion/cascada_test.go` | nada: funciones puras |
| Aplicación (caso de uso + parser) | `aplicacion/identificacion_test.go` | dobles locales de `RepositorioIngesta` y `RepositorioIdentificacion` |
| Adaptador (A/G/O/M) | `postgres/identificacion_test.go` | PostgreSQL real (testhelp + SQL directo) |
| Caso de uso + adaptador (I1–I6) | `postgres/identificacion_test.go` | PostgreSQL real para todo lo escribible/sondeable; doble local solo para la lectura de `usos` (pertenece a #72) |
