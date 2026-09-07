# 01 — Diseño: cascada de identificación, escalones 1–2 + filtro de repertorio (#28)

> Documento de planificación. Lo implementa un agente (Claude) que no ve la conversación que lo
> originó: todo lo que necesita saber está en este directorio y en el repo. Este fichero decide el
> **diseño**; `02-plan-tdd.md` decide el **orden de trabajo**; `03-brief-para-claude.md` es el
> **brief de ejecución** autocontenido.

- Issue: #28 — "[Backend] Identification cascade — escalon 1 (alias) + escalon 2 (IDA/EIDR/IMDB) + repertoire filter".
- Rama de trabajo: `feature/28-identification-cascade`, creada desde `main` en `46aa720` (head real del remoto verificado por API el día de la planificación).
- Fuentes normativas: ADR 0007 (cascada), `docs/dominio/identificadores.md` (cubo y radios), regla R-27 / `RD 9.5` (filtro de repertorio), ADR 0005 (determinismo), ADR 0006 (trazabilidad), ADR 0003 (módulos), skill `.claude/skills/matching-de-obras/SKILL.md`.
- Skill: la cascada y el vocabulario (alias, ONI, repertorio) están en `.claude/skills/matching-de-obras/SKILL.md` y en el ADR 0007. **Ojo**: la skill describe `alias_obra` con columnas que no existen en el esquema (ver §10.2).

---

## 1. Resumen

Construir los dos primeros escalones de la cascada del ADR 0007 contra el catálogo maestro, más el
filtro de repertorio (R-27) que corre **antes** de la cascada:

1. **Escalón 1 — alias**: la fila trae un id de su fuente que ya está en `alias_obra` → resuelve
   con certeza máxima (puntaje 1), sin más trabajo.
2. **Escalón 2 — identificador global**: si la fila trae IDA, EIDR o IMDB poblados y el catálogo
   tiene una obra con ese identificador → resuelve, **y aprende el alias** (fuente, tipo, valor) para
   que la próxima fila con ese id de fuente entre por el escalón 1.
3. **Filtro de repertorio (escalón 0)**: una fila de una fuente excluida no entra a la cascada:
   queda excluida, **no se marca ONI** y no se le escribe nada.
4. **Lo no resuelto se queda como está** (`escalon = 'pendiente'`): es el insumo del difuso (#32),
   que es otro issue. La cola manual/ONI y su UI son #39/#37.

`aplicacion` gana el caso de uso `ResolverUsos(periodo)`: lee las filas pendientes del periodo por
el puerto declarado `RepositorioIngesta`, corre la cascada por fila y persiste los aciertos con
`GuardarMatch` y los alias aprendidos con `GuardarAlias`. El adaptador `RepositorioIdentificacion`
se implementa en `internal/infraestructura/postgres/identificacion.go`.

**No hace falta ninguna migración**: `usos` ya tiene `obra_id`, `escalon`, `evidencia`, `puntaje`,
`oni`, `resuelto_por`, `resuelto_en` (00001) y `alias_obra` ya existe con la forma que la cascada
necesita. El esquema se tocó en 00001 pensando en esta cascada; este issue solo la implementa.

---

## 2. Estado verificado de main y de #72 (hechos, no supuestos)

- `main` local está en `46aa720` (merge de #86) y el head real del remoto, verificado por la API de
  GitHub, es `46aa72030aef9bf705395c22b8ef37441d9be2bc`. Sincronizado; no hay nada que pull.
- **PR #72 está OPEN** (rama `feature/18-vault-reportes-e-ingesta`, sin merge). Consecuencia para
  este issue:
  - En `main`, `internal/aplicacion/puertos.go` **ya declara** `RepositorioIngesta` con
    `UsosSinResolver`, `UsosDePeriodo`, `UsoPorID`, `GuardarReporte`, `GuardarUsos`; **pero no hay
    adaptador**: `internal/infraestructura/postgres/` no tiene `ingesta.go`.
  - **Este issue NO implementa lectura de usos** (eso pertenece a #72). `ResolverUsos` se construye
    contra el puerto declarado; en los tests de integración las filas de `usos` se siembran por SQL
    directo y la lectura del periodo se hace con un doble local del puerto (ver `02-plan-tdd.md`).
  - En `main`, `UsoPersistido` (modelos.go) no tiene `RechazoMotivo` ni `Periodo`. El código de este
    issue compila contra el tipo tal como está en main y **no lo modifica**. Cuando #72 mergee, sus
    cambios son aditivos y no chocan.
- Migraciones en `main`: `00001_init.sql` y **`00002_catalogo_obras.sql`** (la migración del
  catálogo se renombró 00003→00002 dentro de #86; en main **no existe** `00003_catalogo_obras.sql`).
- Numeración reclamada por PRs abiertos hoy: `00003_cola_clave_natural.sql` (#85) y
  `00003_oni_publicacion.sql` (#87) reclaman la **misma** versión 00003; `00004_log_de_rechazos.sql`
  (#72 y #78) reclama 00004. La convención del repo (comentario cabecera de `00002_catalogo_obras.sql`):
  dos ficheros con la misma versión son error duro de goose en cada arranque; un hueco no lo es; el
  orden de merge decide y quien pierde renumerará con `git mv`. **Este issue no añade migración**, así
  que no entra en la disputa; si la revisión pidiera una, el número a usar sería `00003` sabiendo que
  hay dos PRs abiertos que lo reclaman.
- La skill `matching-de-obras` describe `alias_obra(fuente, tipo_id, valor_id, obra_id, confianza,
  resuelto_por, resuelto_en)`. **El esquema real (00001) es `alias_obra(fuente, tipo_id, valor,
  obra_id, quien, aprendido)`**: no existen `valor_id`, `confianza`, `resuelto_por` ni
  `resuelto_en` en esa tabla. Manda el esquema. El plan incluye corregir la skill (commit 4).

---

## 3. Alcance y no-alcance

### Hace
- Lógica pura de los escalones 1–2 y del filtro de repertorio en `internal/dominio/identificacion`.
- Caso de uso `ResolverUsos` en `internal/aplicacion` contra los puertos **ya declarados**
  (`RepositorioIdentificacion`, `RepositorioIngesta`). Ningún cambio en `puertos.go`.
- Adaptador `RepositorioIdentificacion` en `internal/infraestructura/postgres/identificacion.go`
  (`Alias`, `GuardarAlias`, `ObraPorIDGlobal`, `GuardarMatch`), con el patrón de `doc.go`.
- Aprendizaje de alias automático cuando una fila resuelve por escalón 2.
- Tests: unidad de dominio (tabla), unidad de aplicación (dobles), integración contra PostgreSQL
  real (testhelp + siembra por SQL directo), incluyendo el criterio "el alias aprendido
  cortocircuita la próxima corrida".

### NO hace (límites duros)
- **No implementa lectura de usos** (métodos de `RepositorioIngesta` en postgres): es #72, abierta.
- **No toca `puertos.go`, `errores.go`, `modelos.go` ni ninguna migración.**
- No es el difuso (#32): no hay `Similitud`, ni umbral, ni normalización de títulos, ni `puntaje`
  variable; las filas no resueltas quedan `pendiente` para #32.
- No es ONI/manual (#37, #39): este issue no escribe `escalon='oni'`, no emite alertas, no marca
  nada como ONI salvo respetar el estado en que las filas ya están. "Excluido" no es ONI.
- No es la cola manual: `GuardarAlias` con `quien` de un usuario llegará cuando #39 resuelva filas a
  mano; aquí el único aprendizaje es automático (`quien = "cascada"`).
- No hay contrato HTTP (los resultados afloran vía #39), no se cablea nada en `cmd/api` (no hay
  disparador todavía: la cola/scheduler es #85), no se generan tipos del frontend.
- **No toca dinero**: el módulo clasifica usos, no los valora (ADR 0003). Ningún importe entra ni
  sale por esta cascada; los `decimal` que aparecen son el `puntaje` de confianza.

---

## 4. Decisiones de diseño

Cada decisión con su alternativa descartada, en el estilo de los ADR del repo (aquí no se escribe un
ADR nuevo: el ADR 0007 ya decidió la cascada; esto es diseño de implementación).

### D1 — El contrato del formato de `usos.ids_fuente` lo fija este issue

**Problema**: el escalón 1 consulta `alias_obra` por `(fuente, tipo_id, valor)` y el escalón 2
necesita IDA/EIDR/IMDB, pero una fila de `usos` solo tiene **una** columna `ids_fuente TEXT`, y ni
el esquema ni #72 ni ningún doc definen su formato. `Entrada` (tipos.go) espera un único par
`(TipoID, ValorID)` más tres globales.

**Decisión**: `ids_fuente` es una serialización canónica de pares `clave=valor`:

- Un par por línea, separador `\n`, sin espacios alrededor del `=`.
- Claves en minúsculas con guion bajo (snake_case). Vocabulario **reservado** para identificadores
  globales: `ida`, `eidr`, `imdb`. El resto de claves son ids locales de la fuente.
- Contrato de escritura (para #25/#72, que aún no escriben nada): las claves locales son el nombre
  canónico de la columna id del archivo (`id_ficha` para Caracol; `show_id`, `series_id`,
  `netflix_id` para Netflix); los globales entran bajo su clave reservada; pares ordenados por clave,
  sin duplicados; solo pares con valor no vacío.
- El lector (este issue) es **tolerante**: línea sin `=`, clave desconocida o valor vacío se ignora;
  ante clave repetida gana la última (determinista). Un `ids_fuente` vacío es válido: la fila solo
  podrá resolver por lo que traiga en otras columnas (título → #32).

Consecuencia para la cascada: cada fila aporta **un** par local para el escalón 1 — el «par
canónico» — elegido con una precedencia por fuente (D2), y hasta tres globales para el escalón 2.

**Alternativa descartada**: dejar el formato sin fijar «hasta que #25 escriba» — rechazada: #28 es
hoy el único consumidor y sin contrato no hay nada que implementar ni probar; el contrato queda
documentado en el código del parser y en este documento para que el escritor futuro lo cumpla.

### D2 — Un solo par local canónico por fila (precedencia por fuente)

**Decisión**: el par que se sondea contra `alias_obra` es el de la clave local **preferida de la
fuente**: `netflix → show_id`, `caracol → id_ficha`. Fuente sin mapeo: si trae una sola clave
local, esa; si trae varias, la primera en orden alfabético de clave (determinista). La tabla de
preferencias vive en `aplicacion` (construcción de `Entrada`), no en el dominio.

Razón de fondo: la obra de REDES es el **programa/show**, no el capítulo (`identificadores.md`,
granularidades; la parrilla de Caracol trae los campos de episodio vacíos al 100%). Elegir
`netflix_id` (único por episodio) haría que cada episodio de un mismo show necesitara su propio
alias y su propio viaje por la cola; con `show_id`, aprender una vez resuelve los episodios del
show. Los alias que algún día existan a granularidad de temporada/episodio no se consultan ni se
aprenden en este issue (quedan fuera; si el negocio decide identificar capítulos, será una decisión
de granularidad nueva con su issue).

**Alternativa descartada**: sondear todos los pares locales de la fila en orden y quedarse con el
primer alias que pegue. Es más tolerante pero rompe el modelo de `Entrada` (un par) del andamiaje,
multiplica consultas por fila y reintroduce la granularidad ambigua que la muestra no permite
decidir. Se documenta como punto de extensión si el dato de capítulos llega.

### D3 — La cascada: decisión pura en dominio, sondeos puntuales en el caso de uso

**Decisión**: la función pura `identificacion.Resolver(entrada, consulta, excluidas) -> Resultado`
decide el escalón y construye el `Resultado` (escalón, puntaje, evidencia) **sin hacer E/S**. El
caso de uso hace los sondeos —uno de alias y, solo si hace falta, hasta tres de identificador
global en orden fijo `ida → eidr → imdb`— y le pasa a `Resolver` lo que respondieron, en
`identificacion.Consulta`. Los **índices reales** del escalón son los de la base: la PK de
`alias_obra (fuente, tipo_id, valor)` y los índices parciales `obras_ida/eidr/imdb` que 00001 creó
explícitamente para el escalón 2.

Razones: el puerto `RepositorioIdentificacion` solo expone sondeos puntuales (`Alias(...)`,
`ObraPorIDGlobal(...)`), no listados; materializar en memoria un «catalogoIndex/aliasIndex» exigiría
añadir métodos de listado a un puerto recién declarado **sin ningún consumidor**, y la cultura del
repo (ADR 0003, doc.go de aplicacion) rechaza ensanchar contratos sin necesidad. El fall-through
es orquestación de E/S de pocas líneas; la regla (qué cuenta como acierto, qué evidencia se
registra, cuándo una entrada «no resuelve») queda en funciones puras con tests de tabla. #32
extenderá esta misma pieza con el escalón 3 (difuso), que es exactamente el punto donde el ADR 0007
lo prevé.

**Alternativa descartada**: el `resolver(entrada, catalogoIndex, aliasIndex)` literal de la
propuesta del issue, con índices en memoria construidos al abrir la corrida — ver arriba: no hay
puerto de listado y no debe inventarse. La variante «sondeo por fila y cero dominio» se descarta
porque dejaría la regla de clasificación y la evidencia en el caso de uso, sin tests de tabla de
dominio y sin punto único para que #32 se cuelgue.

### D4 — Filtro de repertorio: escalón 0 puro, excluido ≠ ONI, y sin estado persistido (por ahora)

**Decisión**: el filtro es parte de `Resolver` (escalón 0, antes del alias): la entrada es de
repertorio salvo que su `Fuente` esté en `FuentesExcluidas` (conjunto que entra como parámetro del
dominio; en el caso de uso es un campo de configuración de `ResolverUsos`). Si está excluida,
`Resolver` devuelve `Resultado{Escalon: "excluido", ONI: false, ObraID: "", Evidencia: ...}` y el
caso de uso **no escribe nada**: ni `GuardarMatch`, ni `GuardarAlias`, ni marca ONI, ni cambia el
`escalon` de la fila.

Qué significa esto en el esquema y por qué no se persiste la exclusión:

- El CHECK de `usos.escalon` solo admite `pendiente|alias|id_global|difuso|manual|oni`; **no existe
  un estado «excluido»** y la fila excluida tampoco puede ponerse `oni=false` (el CHECK
  `uso_resuelto_tiene_obra` exige obra_id cuando no es ONI). Representar la exclusión como estado
  exigiría una migración que este issue no necesita.
- La exclusión es un **predicado determinista sobre la fila**: una re-corrida excluye otra vez la
  misma fila sin escribir nada, así que no hay trabajo repetido que valga la pena persistir hoy, y
  no se pierde trazabilidad (nada se decidió sobre la fila; decidir «excluida» no es un hecho de
  matching). Cuando #32/#37 necesiten saber qué filas quedaron fuera de la cascada (p. ej. para no
  listarlas en ONI), será el momento de decidir el estado (probablemente una migración aditiva con
  un valor `excluido` en el CHECK de `usos.escalon` o una tabla/log propio) — se deja anotado como
  pregunta abierta (P3), fuera de este issue.

El dato de política (qué fuentes/canales están fuera) **no existe en el esquema ni llega por ningún
puerto**: R-27/RD 9.5 es una clasificación de operadores de cable para el reparto, y el equivalente
a nivel de programa (noticieros/magazines) depende del mapeo de géneros que sigue siendo una
pregunta abierta con el cliente (fuentes-datos.md, pregunta 5). Por eso el conjunto entra inyectado
y en producción arranca **vacío** (nada se excluye hasta que exista el dato), sesgo conservador: lo
dudoso se matchea o va a la cola, nunca se descarta en silencio — el mismo sesgo que ADR 0007 fija
para el umbral difuso. El mecanismo queda completo y probado; la política entra por configuración
sin tocar la cascada.

**Alternativas descartadas**: (a) dejar que lo no-repertorio caiga a ONI — rechazada por R-27 y por
la propia issue: ensucia la cola manual con lo que nunca se va a pagar; (b) inventar ahora un
registro de canales o un mapeo de géneros — rechazada: sería fabricar datos normativos que piden
cita al reglamento/cliente (ADR 0004, fuentes-datos.md).

### D5 — Aprendizaje de alias: solo al resolver por escalón ≥ 2, y antes que el match

**Decisión**: `ResolverUsos` aprende el alias cuando una fila resuelve por **escalón 2** (id
global): `GuardarAlias(fuente, tipo_id, valor, obraID, quien="cascada")`. No se aprende al resolver
por escalón 1 (el alias ya existe: es lo que resolvió). La resolución manual futura (#39) usará el
mismo puerto con `quien` = id del usuario.

Orden dentro de la fila: **primero `GuardarAlias`, después `GuardarMatch`**. Si la corrida muere
entre los dos, el reintento re-resuelve la fila por escalón 1 (el alias ya está) y converge; si el
orden fuera inverso, la fila quedaría resuelta sin alias y el conocimiento se perdería para siempre.

`GuardarAlias` es **idempotente** (`ON CONFLICT (fuente, tipo_id, valor) DO NOTHING`): re-corridas y
carreras no duplican ni pisan. **No** pisa un alias existente con otra obra: un alias que ya apunta
a una obra distinta solo puede venir de una decisión anterior (humana o de una corrida previa), y
una igualdad de id global no debe revertirla en silencio.

**Alternativa descartada**: aprender en cada acierto (incluido escalón 1) — redundante; y
`ON CONFLICT DO UPDATE` (que la última decisión gane) — rechazada: un match automático no debe poder
deshacer una corrección previa.

### D6 — Trazabilidad: el `Resultado` habla el vocabulario del CHECK de `usos`

**Decisión**: `Resultado.Escalon` (string) usa los valores del CHECK de `usos.escalon` que ya
existen: `"alias"` e `"id_global"`, más `"excluido"` que **solo** vive en memoria (no se persiste,
ver D4). `GuardarMatch` traduce el `Resultado` al `UPDATE` de la fila sin reinterpretar nada:

- `obra_id = NULLIF($2,'')`, `escalon = $3`, `evidencia = $4`, `puntaje = $5`,
  `oni = (obra_id vacío)` — consistente con el CHECK `uso_resuelto_tiene_obra` por construcción.
- Los aciertos de alias/id_global llevan `puntaje = 1` (certeza máxima por igualdad exacta) y
  `oni = false`; `resuelto_por` y `resuelto_en` quedan `NULL` (el CHECK `manual_tiene_autor` los
  reserva a la resolución manual; el instante de la resolución automática quedará en el asiento de
  bitácora cuando #29 cablee `BitacoraAuditoria`, no es columna de `usos`).
- La `Evidencia` es texto estable y determinista que nombra **cómo** se reconoció, no solo por qué
  escalón pasó (pregunta 3 del ADR 0006):
  - escalón 1: `alias caracol id_ficha=871732 -> obra-45`
  - escalón 2: `ida IDA-000001 -> obra-45` (nombra cuál de los tres identificadores casó)
  - excluida: `fuera de repertorio: <fuente>`
  Formato fijado en el dominio (constructores), probado en la tabla.

`GuardarMatch` también queda preparado para #32/#37 sin cambios: un `Resultado` con `ObraID == ""`
y `Escalon == "oni"` escribe `oni = true` con obra NULL, que el CHECK admite; un `Resultado` vacío
o inconsistente revienta en un CHECK y el error sube envuelto (nunca se traga).

**Alternativa descartada**: dar formato a la evidencia en el adaptador o en el caso de uso —
rechazada: la evidencia es vocabulario de dominio (qué prueba un acierto) y debe vivir donde vive
la regla, testeada en tabla.

### D7 — Anti-N+1: una sola lectura del periodo; sondeos de una fila, indexados y acotados

**Decisión**:

- `ResolverUsos` hace **una** lectura: `RepositorioIngesta.UsosDePeriodo(ctx, periodo)` (método ya
  declarado en main). El filtro `escalon == "pendiente"` se aplica en memoria: filas ya resueltas o
  ya clasificadas de corridas anteriores no se tocan (idempotencia). El puerto debe devolver orden
  explícito por `id` (contrato del patrón postgres); el caso de uso no depende del orden para sus
  escrituras (son por fila e independientes), pero los tests del doble lo respetan.
- Por fila pendiente: **1** sondeo de alias (PK de `alias_obra`, indexado) si trae par local; si no
  hay alias, hasta **3** sondeos de id global, uno por identificador poblado, en orden fijo
  `ida → eidr → imdb`, parando en el primero que case (cada uno usa su índice parcial de `obras`).
  Una fila sin id de fuente ni globales no sondea nada y queda para #32.
- Nunca se lee `usos` fila a fila, nunca se hace join por fila, no hay listados dentro del bucle.

**Alternativa descartada**: snapshot en memoria de todo `alias_obra`/`obras` al abrir la corrida
para cero sondeos — ver D3: exige puertos de listado que no existen; además la muestra es diminuta
(decenas de filas por periodo), los sondeos son igualdades con índice y el N+1 que hay que evitar
es el de lecturas de `usos`, no los sondeos de escalón.

### D8 — Errores: `ErrNoEncontrado` sigue al siguiente escalón; cualquier otro error aborta la corrida

**Decisión** (ya escrita como doctrina en `doc.go` del paquete identificacion y en
`aplicacion/errores.go`):

- `ErrNoEncontrado` del sondeo de alias o de id global significa «la consulta fue bien y no hay
  fila» → se cae al siguiente escalón o la fila queda sin resolver. Es el único error que la
  cascada interpreta como no-match.
- **Cualquier otro error aborta la corrida** con contexto (`errors.Is` intacto): tragarse un fallo
  de red como «no hay alias» reclasificaría filas en silencio y es el error caro que el repo declara
  por escrito. No se «marca ONI» ni se salta la fila: falla la corrida.
- `GuardarMatch` contra un `usoID` inexistente (0 filas afectadas) devuelve `ErrNoEncontrado`
  envuelto: con la lectura de pendientes como única fuente de `usoID`, solo puede pasar por
  concurrencia y debe sonar, no callar.
- Violaciones de constraint (CHECK de `escalon`, `uso_resuelto_tiene_obra`, FK de `obra_id`) suben
  envueltas con contexto; son bugs de llamada o estados que el dominio no debió producir, y se ven
  en los tests del adaptador. **No se añaden centinelas nuevos a `aplicacion/errores.go`**: este
  issue no introduce vocabulario de error que los casos de uso necesiten distinguir (a diferencia
  de #72, que sí añadió los suyos). Si la implementación descubre una necesidad real, se anota en
  la PR, no se inventa aquí.

**Alternativa descartada**: devolver `ErrNoEncontrado` «a lo ancho» en el adaptador para todo lo
que no sea fila — es exactamente lo que `traducirError` está diseñado para no hacer.

### D9 — Determinismo y re-ejecución (ADR 0005)

- La cascada no calcula cifras (clasifica filas), pero aun así: orden de sondeo fijo
  (`ida → eidr → imdb`), par canónico por precedencia fija por fuente, formato de evidencia fijo,
  parser de `ids_fuente` con reglas de desempate fijas (última clave repetida gana; primera clave
  alfabética si la fuente no tiene mapeo y hay varias).
- `ObraPorIDGlobal` con más de una obra que comparte el mismo identificador global (posible: no hay
  UNIQUE en `obras`) devuelve la de menor `id` (`ORDER BY id LIMIT 1`): determinista y documentado.
  El catálogo real no debería tener ese duplicado (es dato sucio de carga), pero la corrida no
  puede depender del orden físico.
- Re-ejecución: una corrida sobre un periodo ya corrido no re-resuelve nada (las filas ya no están
  `pendiente`) y no duplica alias (`ON CONFLICT DO NOTHING`). Una corrida interrumpida converge al
  reintentarse (D5).

---

## 5. Forma del código

### 5.1 `internal/dominio/identificacion/cascada.go` (nuevo, puro, sin E/S)

```go
package identificacion

// Escalones del vocabulario del CHECK de usos.escalon. "excluido" solo vive en
// Resultado: no se persiste en este issue (ver D4 de docs/planes/28-.../01-design.md).
const (
    EscalonAlias     = "alias"
    EscalonIDGlobal  = "id_global"
    EscalonExcluido  = "excluido"
)

// IDGlobal identifica cuál de los tres identificadores globales casó.
type IDGlobal string

const (
    IDA  IDGlobal = "ida"
    EIDR IDGlobal = "eidr"
    IMDB IDGlobal = "imdb"
)

// OrdenIDGlobal es el orden fijo en que se sondea el escalón 2 (D9).
var OrdenIDGlobal = []IDGlobal{IDA, EIDR, IMDB}

// FuentesExcluidas es el conjunto de fuentes fuera de repertorio (R-27).
// Vacío = nada se excluye.
type FuentesExcluidas []string

func (f FuentesExcluidas) Excluye(fuente string) bool

// Consulta trae lo que respondieron los sondeos de datos. Resolver no hace E/S:
// el caso de uso consulta los puertos y rellena esto.
type Consulta struct {
    AliasObraID    string   // lo que devolvió Alias(); "" = sin alias
    IDGlobalObraID string   // obra del primer id global que casó; "" = ninguno
    IDGlobalCual   IDGlobal // cuál de los tres casó; "" si no se sondeó
}

// Resolver aplica la cascada (escalones 0-2 del ADR 0007) y devuelve el
// Resultado. Un Resultado con ObraID == "" y Escalon == "" significa "no
// resuelto": la fila queda para el difuso (#32).
func Resolver(e Entrada, c Consulta, excluidas FuentesExcluidas) Resultado
```

Semántica de `Resolver`, en orden:
1. Si `excluidas.Excluye(e.Fuente)` → `Resultado{Escalon: EscalonExcluido, Evidencia:
   "fuera de repertorio: <fuente>"}` (ONI false, ObraID "", Puntaje cero).
2. Si `c.AliasObraID != ""` → `Resultado{Escalon: EscalonAlias, ObraID: c.AliasObraID,
   Puntaje: 1, Evidencia: "alias <fuente> <tipo>=<valor> -> <obraID>"}`.
3. Si `c.IDGlobalObraID != "" && c.IDGlobalCual != ""` → `Resultado{Escalon: EscalonIDGlobal,
   ObraID: c.IDGlobalObraID, Puntaje: 1, Evidencia: "<cual> <valor> -> <obraID>"}`.
4. Si no → `Resultado{}` (no resuelto; ONI false, no toca la fila).

No hay más tipos nuevos en el dominio. `Entrada`, `Resultado` y `Candidato` (tipos.go) no cambian
de forma.

### 5.2 `internal/aplicacion/identificacion.go` (nuevo)

```go
package aplicacion

// quienCascada identifica el aprendizaje automático en alias_obra.quien.
const quienCascada = "cascada"

// ResolverUsos corre la cascada sobre las filas pendientes de un periodo.
type ResolverUsos struct {
    Usos           RepositorioIngesta
    Identificacion RepositorioIdentificacion
    // FueraDeRepertorio: fuentes excluidas por R-27. Vacío en producción hasta
    // que exista el dato (ver D4).
    FueraDeRepertorio identificacion.FuentesExcluidas
}

// ResolverUsos resuelve las filas pendientes del periodo y devuelve cuántas
// resolvió. Las no resueltas quedan escalon='pendiente' para el difuso (#32).
func (r ResolverUsos) ResolverUsos(ctx context.Context, periodo string) (int, error)
```

Lógica (con los errores según D8):

```go
usos, err := r.Usos.UsosDePeriodo(ctx, periodo)          // 1 lectura (D7)
// por cada u con u.Escalon == "pendiente":
//   e := entradaDesdeUso(u)                              // 5.4, puro
//   res, err := r.resolverFila(ctx, u, e)                // 5.3
//   if res.ObraID == "" { continue }                     // excluida o no resuelta: nada
//   if res.Escalon == identificacion.EscalonIDGlobal {   // aprendizaje (D5)
//       r.Identificacion.GuardarAlias(ctx, e.Fuente, e.TipoID, e.ValorID,
//           res.ObraID, quienCascada)                    // ANTES que el match
//   }
//   r.Identificacion.GuardarMatch(ctx, u.ID, res)
//   resueltas++
```

Las filas que no estén `pendiente` (ya resueltas, ya ONI, manuales) se ignoran: la corrida es
idempotente sobre un periodo ya corrido (D9). Ninguna otra escritura: las excluidas no se tocan,
las no resueltas no se tocan.

### 5.3 `resolverFila` (privado, en el mismo fichero)

```go
// resolverFila sondea los escalones en orden y deja que Resolver decida.
func (r ResolverUsos) resolverFila(ctx context.Context, u UsoPersistido, e identificacion.Entrada) (identificacion.Resultado, error) {
    c := identificacion.Consulta{}
    if e.TipoID != "" && e.ValorID != "" {
        obra, err := r.Identificacion.Alias(ctx, e.Fuente, e.TipoID, e.ValorID)
        switch {
        case err == nil:
            c.AliasObraID = obra
        case errors.Is(err, ErrNoEncontrado):
            // sin alias: cae al siguiente escalón
        default:
            return identificacion.Resultado{}, fmt.Errorf("escalon 1 de %q (%s=%s): %w",
                u.ID, e.TipoID, e.ValorID, err)
        }
    }
    if c.AliasObraID == "" {
        for _, g := range identificacion.OrdenIDGlobal {          // ida → eidr → imdb
            valor := valorGlobal(e, g)                             // helper privado
            if valor == "" { continue }                            // escalón 2 no casa sin dato
            obra, err := r.Identificacion.ObraPorIDGlobal(ctx, idaEidrImdb(e, g)) // solo g poblado
            switch {
            case err == nil:
                c.IDGlobalObraID, c.IDGlobalCual = obra, g
            case errors.Is(err, ErrNoEncontrado):
                // sigue con el siguiente identificador
            default:
                return identificacion.Resultado{}, fmt.Errorf("escalon 2 de %q (%s): %w",
                    u.ID, g, err)
            }
            if c.IDGlobalObraID != "" { break }
        }
    }
    return identificacion.Resolver(e, c, r.FueraDeRepertorio), nil
}
```

Notas: `ObraPorIDGlobal` se llama con un solo identificador poblado y los otros dos vacíos (el
adaptador neutraliza los vacíos en SQL); así el caso de uso sabe **cuál** casó y la evidencia lo
nombra (D6). No hay `time`, no hay estado mutable, no hay transacción explícita: cada sondeo y cada
escritura son operaciones independientes del puerto, y la convergencia ante fallo a mitad la da el
orden alias→match (D5) más la idempotencia (D9).

### 5.4 `entradaDesdeUso` (privado, mismo fichero): construcción de `Entrada` y parser de `ids_fuente`

```go
// parCanonicoPorFuente: clave local preferida para el escalón 1 (D2).
var parCanonicoPorFuente = map[string]string{
    "caracol": "id_ficha",
    "netflix": "show_id",
}

func entradaDesdeUso(u UsoPersistido) identificacion.Entrada
```

Pasos:
1. `e.Fuente = u.Fuente`, `e.Titulo = u.Titulo`. `TituloOrig` queda `""`: `usos` no tiene columna
   para el título original (lo necesitará el difuso #32; si hace falta, será decisión de esquema de
   ese issue).
2. Parsear `u.IDsFuente` por líneas (`\n`): cada línea `clave=valor` → si `clave` ∈
   `{ida, eidr, imdb}` y `valor != ""`, llenar el global correspondiente; si no, acumular en un mapa
   ordenado de locales. Líneas rotas (sin `=`, clave vacía, valor vacío) se ignoran; clave repetida:
   gana la última.
3. Elegir el par local: clave del mapa `parCanonicoPorFuente[u.Fuente]`; si no hay mapeo para la
   fuente y hay una sola clave local, esa; si hay varias, la primera en orden alfabético. Ese par va
   a `e.TipoID` / `e.ValorID`.

### 5.5 `internal/infraestructura/postgres/identificacion.go` (nuevo)

Un fichero por puerto, aserción de compilación antes del SQL, todo error por `traducirError`,
patrón de `doc.go`:

```go
var _ aplicacion.RepositorioIdentificacion = (*Store)(nil)
```

SQL de las cuatro operaciones (firmas: las de `puertos.go`, sin cambios):

- `Alias(ctx, fuente, tipo, valor) (obraID string, err error)`
  `SELECT obra_id FROM alias_obra WHERE fuente=$1 AND tipo_id=$2 AND valor=$3` con `QueryRow` +
  `traducirError(err, "alias de %q (%s=%s)", fuente, tipo, valor)` → `ErrNoEncontrado` si no hay
  fila (la PK garantiza una sola).
- `GuardarAlias(ctx, fuente, tipo, valor, obraID, quien string) error`
  `INSERT INTO alias_obra (fuente, tipo_id, valor, obra_id, quien) VALUES ($1,$2,$3,$4,$5)
   ON CONFLICT (fuente, tipo_id, valor) DO NOTHING`; `quien` vacío → `NULL` (columna nullable).
  Idempotente por diseño (D5); un CHECK (`btrim(valor) <> ''`) o la FK de `obra_id` que fallen
  suben envueltos (D8).
- `ObraPorIDGlobal(ctx, ida, eidr, imdb string) (obraID string, err error)`
  Contrato del puerto (`puertos.go`): si los tres identificadores llegan vacíos devuelve
  `ErrNoEncontrado`, sin tocar la base — la cascada nunca la llama así (ver 5.3), pero una llamada
  sin datos no puede inventar un match. Si al menos uno viene poblado:
  `SELECT id FROM obras WHERE ($1 <> '' AND ida = $1) OR ($2 <> '' AND eidr = $2) OR ($3 <> '' AND
  imdb = $3) ORDER BY id LIMIT 1` — usa los índices parciales de 00001, es determinista ante
  duplicados de id global (D9) y no distingue cuál columna casó (no lo necesita: la cascada llama
  con uno solo poblado).
- `GuardarMatch(ctx, usoID string, r identificacion.Resultado) error`
  `UPDATE usos SET obra_id = NULLIF($2,''), escalon = $3, evidencia = $4, puntaje = $5,
   oni = ($2 = '') WHERE id = $1`; si `RowsAffected() == 0` →
  `traducirError`-style wrap de `ErrNoEncontrado` («el uso X no existe» — patrón del `Actualizar` de
  `catalogo.go`); los CHECK (`escalon`, `uso_resuelto_tiene_obra`, `manual_tiene_autor`, FK de
  `obra_id`) que fallen suben envueltos. El `puntaje` viaja como `decimal.Decimal` directo a pgx
  (mismo mecanismo que #72 con las medidas de `usos`).

Sin migración, sin tocar `store.go` ni `puertos.go`, sin cableado en `cmd/`.

---

## 6. Ejemplo completo de una corrida (contra el esquema real)

Catálogo sembrado: obra `obra-45` con `ida=''`, `eidr=''`, `imdb='tt0100001'`; obra `obra-12` con
`ida='IDA-000001'`.

- Reporte `rep-caracol-1` (fuente `caracol`, periodo `2024`) → dos filas:
  - `u-1`: `ids_fuente = "id_ficha=871732\nimdb=tt0100001"`, título cualquiera.
    - Entrada: par `(caracol, id_ficha, 871732)`; globales: `imdb=tt0100001`.
    - Corrida 1, sin alias previo: escalón 1 → `ErrNoEncontrado`; escalón 2 sondea `ida` (vacío,
      se salta), `eidr` (vacío, se salta), `imdb` → `obra-45`. `Resolver` → escalón `id_global`,
      puntaje 1, evidencia `imdb tt0100001 -> obra-45`. `GuardarAlias(caracol, id_ficha, 871732,
      obra-45, "cascada")`; `GuardarMatch(u-1, ...)` → la fila queda `escalon='id_global'`,
      `oni=false`, `obra_id='obra-45'`.
  - `u-2`: `ids_fuente = "id_ficha=871732"` (misma obra, otra emisión).
    - Corrida 1 (misma corrida, u-1 va antes por `ORDER BY id`): el alias se aprende **dentro** de
      la corrida; u-2 resuelve por escalón 1 (evidencia `alias caracol id_ficha=871732 ->
      obra-45`). Corrida 2 (periodo ya corrido): u-1/u-2 ya no están `pendiente`, no se tocan.
- Reporte de una fuente excluida (`fuente="canal-deportes"` con `FueraDeRepertorio` que la
  contiene): la fila no se sondea, no se escribe, queda `escalon='pendiente'` y `oni=true` tal como
  entró — **no marcada ONI por la cascada**, no aprendida, no resuelta.

---

## 7. Coherencia con el esquema (mapeo columna a columna)

| Necesidad del issue | Dónde vive en el esquema real (00001/00002) |
| --- | --- |
| Escalón 1: `(fuente, tipo_id, valor) → obra` | `alias_obra(fuente, tipo_id, valor, obra_id, quien, aprendido)`, PK `(fuente, tipo_id, valor)` |
| Escalón 2: igualdad por IDA/EIDR/IMDB | `obras(ida, eidr, imdb)` + índices parciales `obras_ida/eidr/imdb` (00001, creados para esto) |
| GuardarMatch: qué escalón y qué evidencia | `usos(obra_id, escalon, evidencia, puntaje, oni)` — columnas y CHECK ya en 00001 |
| «Un uso resuelto tiene obra; uno en ONI no» | CHECK `uso_resuelto_tiene_obra`; `GuardarMatch` lo respeta por construcción (`oni = obra ausente`) |
| Actor/instante de la resolución | `usos.resuelto_por/resuelto_en` + CHECK `manual_tiene_autor`: reservados a `escalon='manual'` (#39) |
| Puntaje de confianza | `usos.puntaje NUMERIC(6,5) CHECK (0..1)`; alias/id_global = 1 |
| Ids de fuente de la fila | `usos.ids_fuente TEXT` (formato: D1) |

---

## 8. Qué NO hace este issue (para que el implementador no se salga del alcance)

- **Nada de difuso (#32)**: ni `Similitud`, ni umbral, ni normalización, ni puntajes por similitud,
  ni `escalon='difuso'`. Las filas sin resolver quedan `pendiente`. No «adelantar» trabajo de #32.
- **Nada de ONI/anomalías (#37)**: no hay detección, no se escriben alertas, no se cambia el
  `oni` de ninguna fila; las pendientes ya vienen `oni=true` de ingesta (DEFAULT/valor estampado
  por #72) y así se quedan hasta que la cascada o #32 decida.
- **Nada de UI/cola manual (#39)**: GuardarAlias con `quien` humano no tiene llamador todavía.
- **Nada de lectura de usos (#72)**: `ResolverUsos` consume el puerto declarado; el adaptador de
  `RepositorioIngesta` lo trae #72. Los tests de integración usan doble local + SQL directo.
- **No se toca el esquema**: cero migraciones. Si la revisión demuestra que algo necesita esquema
  (p. ej. persistir la exclusión, D4/P3), se escribe la migración con su número negociado (§2), no
  en silencio.
- **No hay dinero**: ninguna medida ni importe entra en la cascada; el `puntaje` es confianza de
  matching, no valor.
- **No se re-planea**: este documento y sus compañeros son el plan. Desviaciones de implementación
  se anotan en la PR; cambios de alcance se consultan.

---

## 9. Errores tipados, resumen

| Situación | Error | Quién lo ve |
| --- | --- | --- |
| Alias no existe | `ErrNoEncontrado` (envuelto con contexto) | caso de uso → siguiente escalón |
| Id global no casa | `ErrNoEncontrado` (envuelto) | caso de uso → siguiente id o fila sin resolver |
| Los tres globales vacíos en `ObraPorIDGlobal` | `ErrNoEncontrado` | llamada directa (la cascada no llama así) |
| Red/timeout/constraint en un sondeo | error original envuelto, **no** `ErrNoEncontrado` | caso de uso → aborta la corrida |
| `GuardarMatch` sobre uso inexistente | `ErrNoEncontrado` («el uso X no existe») | caso de uso → aborta |
| CHECK/FK violados en escrituras | error de constraint envuelto | caso de uso → aborta; tests del adaptador |

---

## 10. Discrepancias detectadas y preguntas abiertas

### 10.1 Discrepancias entre el encargo y la realidad del repo (ya resueltas en este diseño)
1. El encargo hablaba de `migrations/00003_catalogo_obras.sql`; en main la migración del catálogo
   es `00002_catalogo_obras.sql` (renombrada dentro de #86). No afecta al diseño (no hay migración).
2. La skill describe columnas de `alias_obra` (`valor_id`, `confianza`, `resuelto_por`,
   `resuelto_en`) que el esquema no tiene (`valor`, `quien`, `aprendido`). El diseño usa el esquema;
   la corrección de la skill va en el commit 4 del plan.
3. La propuesta del issue dice `resolver(entrada, catalogoIndex, aliasIndex)`; el diseño lo
   reinterpreta (D3) porque los puertos declarados no exponen listados.

### 10.2 Preguntas abiertas para el implementador, el revisor o el cliente (no bloquean; se anotan en la PR)
- **P1 (implementador/revisor)** — Formato `ids_fuente` (D1): este issue lo fija porque es su único
  consumidor, pero quien lo escriba será #25 (adaptadores de formato). ¿Se valida el contrato en
  esta PR (doc en el código) o se espera a #25 para fijar también el lado escritor? Recomendado:
  documentar aquí y exigir el cumplimiento en la review de #25.
- **P2 (cliente)** — Política de repertorio (D4): R-27 opera sobre operadores de cable (RD 9.5) y
  el repo la reinterpreta a nivel de programa (noticieros/magazines); el dato (canales/programas
  fuera + mapeo de géneros, pregunta 5 de `fuentes-datos.md`) no existe. Mientras tanto el set
  inyectado queda vacío en producción. ¿Alguien más conoce una fuente de este dato?
- **P3 (revisor)** — Si #32/#52 necesitan que las filas excluidas no aparezcan como pendientes/ONI
  (la vista `oni_publico` lista `WHERE oni`, y las pendientes vienen `oni=true` desde ingesta),
  hará falta representar «excluido» en `usos` (migración aditiva al CHECK de `escalon`, o log
  propio). No es de este issue; dejarlo anotado para #32/#37.
- **P4 (revisor)** — `GuardarMatch` no registra el instante de la resolución automática
  (`resuelto_en` queda NULL; el CHECK lo reserva a `manual`). El «cuándo» quedará en el asiento de
  bitácora cuando #29 conecte `BitacoraAuditoria`. ¿Se acepta así o este issue debe abrir la
  discusión del asiento de «fila identificada»?
- **P5 (implementador)** — `ObraPorIDGlobal` con duplicados de id global en `obras` resuelve por
  `ORDER BY id` (D9). Si el seed de #78 o los datos reales muestran duplicados, conviene avisar:
  puede indicar carga sucia del catálogo.
- **P6 (implementador/revisor)** — `resolverFila` no usa transacción por fila ni por corrida (cada
  operación del puerto es independiente; convergencia por idempotencia, D5/D9). Si el revisor
  prefiere corrida transaccional, habría que cambiar la forma del puerto (recibir `pgx.Tx`) — se
  descarta por ahora porque los puertos ya están declarados sin ella, igual que #72.
