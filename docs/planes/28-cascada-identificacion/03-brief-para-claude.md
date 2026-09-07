# 03 — Brief de ejecución para Claude (issue #28)

> Esto es TODO lo que necesitas para implementar. Léelo entero, luego lee los dos documentos de
> diseño que cita — viven en el repo, en la rama donde estás — y ejecuta. No re-planifiques: el
> diseño y el plan TDD están decididos (D1–D9 y fases 1–4 en los ficheros citados). Si algo del
> diseño no se sostiene al implementar, **no cambies alcance ni esquema por tu cuenta**: anótalo en
> el cuerpo de la PR y pregunta.

## 0. Qué estás haciendo

Implementas el issue **#28** — cascada de identificación de obras, escalones 1 (alias) y 2
(IDA/EIDR/IMDB) + filtro de repertorio (R-27) — en el backend Go de Intela (REDES SGC). Estás en la
rama `feature/28-identification-cascade`, creada desde `main` en `46aa720` (head real del remoto).

Documentos que mandan (en `docs/planes/28-cascada-identificacion/`, **no los edites**):
- `01-design.md` — el diseño completo: decisiones D1–D9 con alternativas descartadas, forma exacta
  del código (firmas y SQL), contrato del formato de `ids_fuente`, coherencia con el esquema,
  límites de alcance y preguntas abiertas P1–P6.
- `02-plan-tdd.md` — orden de trabajo por archivo con tests caso a caso, qué se prueba con dobles y
  qué contra PostgreSQL real, y el orden de commits (4 commits).

## 1. Contexto mínimo del repo

- Go 1.24, módulo `github.com/rosvend/intela`; PostgreSQL 16, `pgx/v5` a mano (sqlc aplazado),
  migraciones `goose` (00001 y 00002 en main), tests de adaptadores con `testcontainers-go`
  (`internal/infraestructura/postgres/testhelp`).
- Clean Architecture con frontera `depguard` (`.golangci.yml`):
  - `internal/dominio/` puro: **no** importa `time`, pgx, `aplicacion`, infraestructura. El instante
    entra como parámetro; aquí no hace falta ninguno.
  - `internal/aplicacion/` orquesta y declara los puertos (`puertos.go`); no importa
    infraestructura.
  - `internal/infraestructura/postgres/` adapta: un fichero por puerto, aserción de compilación
    `var _ aplicacion.X = (*Store)(nil)`, errores por `traducirError`, patrón completo en
    `postgres/doc.go`.
- Vocabulario de dominio en español (obra, titular, declaracion, usos, alias); el resto del código,
  infraestructura incluida, en inglés.
- Invariantes que NO se tocan: el dinero no llega por fila; porcentajes solo de la Declaracion;
  declaracion incompleta = nunca reparto parcial; solo personas naturales cobran. **La cascada no
  toca dinero**: clasifica usos (escribe obra/escalon/evidencia/puntaje), no los valora.
- Determinismo (ADR 0005): órdenes fijos donde haya iteraciones (ida → eidr → imdb; par canónico
  por fuente; evidencia con formato fijo).
- Estado del entorno: **PR #72 abierta** → `RepositorioIngesta` está declarado en `puertos.go`
  pero **no tiene adaptador en main**. **No implementes lectura de usos** (es #72). Los tests de
  integración siembran `usos` por SQL directo y leen con un doble local. `UsoPersistido` es el de
  main (sin `RechazoMotivo`); no lo modifiques.

## 2. Checklist por archivo (firmas y casos en 01/02)

**Fase 1 — dominio puro.**
- [ ] Crear `internal/dominio/identificacion/cascada.go`: constantes `EscalonAlias = "alias"`,
  `EscalonIDGlobal = "id_global"`, `EscalonExcluido = "excluido"` (solo en memoria, no se persiste);
  `type IDGlobal string` con `IDA/EIDR/IMDB` y `OrdenIDGlobal`; `type FuentesExcluidas []string`
  con `Excluye(fuente) bool`; `type Consulta { AliasObraID string; IDGlobalObraID string;
  IDGlobalCual IDGlobal }`; `func Resolver(e Entrada, c Consulta, excluidas FuentesExcluidas)
  Resultado`. Semántica y evidencia exactas: §5.1 de 01-design.
- [ ] Crear `cascada_test.go` con la tabla de 8 casos: alias hit → escalón 1 puntaje 1; id global
  hit → escalón 2; id poblado sin match → `Resultado{}`; alias manda; repertorio excluye (aunque el
  alias pegara); fuente no excluida; entrada sin datos; evidencia determinista.

**Fase 2 — adaptador.**
- [ ] Crear `internal/infraestructura/postgres/identificacion.go` con los 4 métodos de
  `RepositorioIdentificacion` (SQL en §5.5 de 01-design; aserción `var _` primero; `Alias` y
  `ObraPorIDGlobal` con `QueryRow` + `traducirError`; `ObraPorIDGlobal` con fast-path de tres
  vacíos → `ErrNoEncontrado` y `ORDER BY id LIMIT 1`; `GuardarAlias` con `ON CONFLICT DO NOTHING`
  y `quien` vacío → NULL; `GuardarMatch` con `NULLIF`/`oni = ($2 <> '')` y chequeo de
  `RowsAffected() == 0` → `ErrNoEncontrado`).
- [ ] Crear `identificacion_test.go` (PostgreSQL real, helpers `sembrarIdentificacion`,
  `nuevaObraGlobal`, `insertarUsoSQL`): casos A1–A2, G1–G5, O1–O5, M1–M6 (lista en 02-plan §2.2).

**Fase 3 — caso de uso.**
- [ ] Crear `internal/aplicacion/identificacion.go`: `const quienCascada = "cascada"`;
  `type ResolverUsos struct { Usos RepositorioIngesta; Identificacion
  RepositorioIdentificacion; FueraDeRepertorio identificacion.FuentesExcluidas }`;
  `func (r ResolverUsos) ResolverUsos(ctx context.Context, periodo string) (int, error)`; privados
  `resolverFila` (sondeos en orden, `ErrNoEncontrado` = siguiente escalón, cualquier otro error
  aborta), `entradaDesdeUso` (parser de `ids_fuente` + par canónico por fuente: `caracol →
  id_ficha`, `netflix → show_id`). Forma completa en §§5.2–5.4 de 01-design.
- [ ] Crear `internal/aplicacion/identificacion_test.go` con dobles `ingestaFalsa` /
  `identificacionFalsa` (que registran llamadas y orden): casos U1–U12 de 02-plan §3.2.
- [ ] Añadir al `identificacion_test.go` de postgres los casos de integración I1–I6 (02-plan §3.3):
  `ResolverUsos` con `Store` real para identificación + doble local para la lectura + siembra SQL;
  asserts por SELECT directo; el alias aprendido cortocircuita la corrida siguiente; excluida no se
  escribe; re-ejecución idempotente.

**Fase 4 — limpieza.**
- [ ] Corregir `.claude/skills/matching-de-obras/SKILL.md`: la tabla de alias dice columnas que no
  existen; la real es `alias_obra(fuente, tipo_id, valor, obra_id, quien, aprendido)` (ver
  02-plan §4).
- [ ] NO toques: migraciones, `puertos.go`, `errores.go`, `modelos.go`, `cmd/`, `docs/planes/`,
  `.gitignore` (tiene una modificación local de tooling ajena a tu PR), el `.drawio`.

## 3. Criterios de aceptación de la issue → pruebas

| Criterio (issue #28) | Dónde se prueba |
| --- | --- |
| Fila cuyo id de fuente ya está en `alias_obra` resuelve en escalón 1 con confianza máxima, sin trabajo difuso | U1, A2/M1, I1 |
| Fila con IDA/EIDR/IMDB poblado que casa con una obra del catálogo resuelve en escalón 2 | U2, O1, I2 |
| Resolver una entrada persiste un alias para que el mismo id de fuente pegue en escalón 1 la próxima vez | U2/U3, G1–G3, I2/I3 |
| Canal/programa no-repertorio se excluye antes de la cascada y no se marca ONI | U5, I4 (+ caso 5 de la tabla de dominio) |
| Filas no resueltas quedan intactas para el difuso (nunca mal asignadas) | U4/U9, I5 (+ casos 3 y 7 de dominio) |
| Frontera + `make verificar` en verde | comandos abajo |

## 4. Comandos de verificación

```bash
go mod tidy                                        # debe quedar SIN diff en go.mod/go.sum
gofmt -l .                                         # vacío
go vet ./...
go build ./...
go test -race -count=1 ./...                       # requiere Docker (testhelp/testcontainers)
golangci-lint run ./...                            # v2.13.1, la que fija .github/workflows/lint-go.yml
golangci-lint run --enable-only=depguard ./...     # la frontera aislada
make verificar
go test -short ./...                               # bucle rápido sin Docker (se salta integración)
```

Los hooks locales (`lefthook.yml`, `lefthook install`) corren lo mismo que CI; si no los tienes
instalados, los comandos de arriba son la puerta.

## 5. Convenciones de commit y PR

- Rama ya creada: `feature/28-identification-cascade` (cumple la regex de CI
  `^(feature|fix|hotfix|docs|chore|refactor)/[a-z0-9._/-]+$`).
- 4 commits, en orden, cada uno en verde (mensajes de 02-plan):
  1. `feat(identificacion): cascade escalon 1-2 pure logic with repertoire filter`
  2. `feat(postgres): RepositorioIdentificacion adapter (alias, guardar alias, id global, match)`
  3. `feat(aplicacion): ResolverUsos use case with alias learning`
  4. `docs: align matching skill alias table with real schema`
- PR contra `main` con título tipo `feat(identificacion): identification cascade escalones 1-2 + repertoire filter (#28)`, `Closes #28` en el cuerpo, y una sección con las preguntas abiertas P1–P6 de
  01-design que te hayan quedado vivas (sobre todo P1 formato de `ids_fuente`, P3 exclusión sin
  estado, P4 instante de la resolución automática).
- Estilo de los comentarios y commits: mira los PRs #62/#86/#72 (ya en el repo o en GitHub) — el
  cuerpo de una PR de este repo explica las decisiones que no se leen solas en el código.

## 6. Recordatorios duros (no negociables)

1. **No** implementes lectura de usos (#72 abierta). Usas el puerto declarado y un doble en tests.
2. **No** migración, **no** esquema, **no** `puertos.go`/`errores.go`/`modelos.go`, **no** HTTP,
   **no** cableado en `cmd/`. El esquema de `usos` y `alias_obra` ya soporta todo el issue.
3. **No** toques dinero ni porcentajes; el `puntaje` que escribes es confianza de matching (1 para
   igualdad exacta), no un valor.
4. **No** uses `time` en dominio. No marques ONI nada: las no resueltas quedan `pendiente`.
5. Un error que no sea `ErrNoEncontrado` en un sondeo **aborta la corrida** — nunca lo trates como
   «no hay match».
6. No re-planifiques ni amplíes alcance. Dudas reales → pregunta en la PR (y si es bloqueante,
   para y consulta antes de seguir).
7. Al terminar, ejecuta la batería completa del §4 y pega su salida en el cuerpo de la PR, como
   hacen los PRs del repo.
