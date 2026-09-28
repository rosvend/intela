# Plano de #39: bandeja de identificacion, lista ONI e historial de resoluciones

Frontend (`web/`). Issue: <https://github.com/rosvend/intela/issues/39>. Rama
`feature/39-matching-review-inbox-oni`, worktree `Intela-wt/39`, base `main` (`96943d3`).

## 1. Alcance

Dentro:

- **Bandeja de identificacion** (`/identificacion`): los casos ONI pendientes con la entrada del
  reporte, la evidencia, los IDs de fuente y las obras candidatas con su puntaje. Tres acciones por
  caso: asignar a una candidata, buscar otra obra en el catalogo y asignarla, o descartar el registro.
  Todas abren un panel lateral con la nota obligatoria.
- **Lista ONI** (`/lista-oni`): todos los casos (pendientes, asignados y descartados) con estado,
  responsable y ultima actualizacion, filtros por estado, fuente y periodo, y el historial de cada
  registro.
- **Historial de resoluciones manuales**: seccion al final del detalle de obra
  (`web/src/catalogo/DetalleObra.tsx`), desde la bitacora de la obra.
- **Navegacion**: "Identificacion" y "Lista ONI" como items propios entre "Distribucion" y
  "Anomalias", solo administrador; badge con el conteo de pendientes.
- Las cuatro capturas del Make v9 en `docs/mockups/` y su README.

Fuera: las demas alertas (#40, PR #104, #164); resolver en lote (diferido por el issue); corregir o
deshacer una decision (fuera de alcance de #175); enlazar las alertas `oni` de `/anomalias` a la
bandeja (seguimiento).

## 2. Contrato

Tipos solo generados (`npm run contrato`), nunca a mano.

| Endpoint | Origen | Uso |
| --- | --- | --- |
| `GET /identificacion/casos` | main (#174, PR #176) | bandeja (`estado=pendiente`), lista ONI, conteo del badge (`limite=1`) |
| `POST /identificacion/casos/{id}/resolucion` | **provisional**, rama de #175 (`51ede63`) | panel de resolucion |
| `GET /obras?titulo=` | main | "Buscar otra obra" |
| `GET /auditoria/obra/{id}` | main | historial de resoluciones (asientos `identificacion.asignada`) |

El commit `d88edde` trae SOLO `api/openapi.yaml` de la rama de #175 y los tipos regenerados. Aporta:
`ResolucionDeCaso {decision: asignar|descartar, obra_id?, nota 1..300}`, el 200 con el
`CasoIdentificacion` resuelto, el estado `descartado`, el campo `nota` del caso, y el payload del
asiento `identificacion.asignada` (snake_case: `uso_id`, `titulo`, `fuente`, `periodo`,
`reporte_id`, `nota`, `actor_nombre`, entre otros).

Todo lo que depende de #175 vive en `web/src/identificacion/resolucion.ts` y en la lectura del
payload (`leerResolucionAsentada` en `tipos.ts`).

Hechos del contrato que el front respeta:

- La nota: Go la recorta y cuenta runas (tope 300). El `maxLength` del navegador cuenta unidades
  UTF-16: para el espanol es lo mismo y para un emoji es mas, asi que el cliente nunca deja pasar
  una nota que el servidor rechace. Se envia recortada.
- El 409 tiene tres causas. Hoy solo las distingue el mensaje; se pidio un `codigo` a #175. La
  clasificacion vive en UNA funcion (`causaDelConflicto`) y va por prefijo del mensaje de los
  ejemplos del contrato hasta que llegue el `codigo`.
- No hay `GET` de un caso por id: "Recargar caso" recarga la bandeja.
- El descarte (`descartado`) sigue pendiente de confirmar con el PO; el texto del panel dice lo que
  el contrato promete (no pondera, no sale en el listado publico de ONI).

## 3. Decisiones

Ya tomadas (no se reabren): solo casos ONI; ninguna candidata preseleccionada ni "recomendada"
(ADR 0007); sin montos en ninguna vista; puntaje como decimal es-CO (`0,71`), nunca porcentaje;
botones "Asignar a esta obra" / "Buscar otra obra" / "Descartar registro", nada de "Cerrar"; nota
obligatoria con contador `n/300` (el contrato fija `maxLength: 300`); los controles del prototipo
("Con datos/Cargando/Error/Vacio", "Ver estados del prototipo") no son producto; remocion optimista y
restauracion si falla; un 409 de "otra persona" muestra "Otra persona resolvio este caso antes" con
"Recargar caso".

Tomadas en esta sesion:

- **D1. Sin react-query.** El badge y la bandeja comparten el conteo por `useOutletContext` de
  react-router: `Layout` pide el conteo al montar (solo administrador) y la bandeja le empuja el suyo
  cada vez que cambia. Decision del usuario.
- **D2. La bandeja siempre lee desde el principio de la cola** (`estado=pendiente`,
  `limite=25`, sin desplazamiento). Paginar por desplazamiento sobre una cola que se vacia al
  resolver salta casos; "Cargar los siguientes casos" recarga desde arriba.
- **D3. Remocion optimista en la lista, estados en el panel.** Al enviar, el caso sale de la lista
  (y del conteo) y el panel sigue abierto con "Guardando...". Exito: se cierra el panel y sale el
  aviso. Error: el caso vuelve a su sitio y el panel muestra el error con la nota intacta. 409 de
  "ya resuelto" o "no pendiente": el caso sigue fuera (el servidor dice que ya no esta pendiente).
  409 de alias en conflicto: el caso vuelve y el panel lo explica. Mientras se envia, el panel no se
  cierra.
- **D4. Lista ONI con filtros en la URL** (`?estado=&fuente=&periodo=&desde=`), aplicados por el
  servidor. Las opciones de fuente y periodo son las vistas en las paginas cargadas mas la
  seleccionada; el estado es el enum del contrato. Pagina de 50 con `Paginador`.
- **D5. Historial del registro en la lista ONI desde la propia fila**: quien, cuando, la decision,
  la nota y, si se asigno, el enlace a la obra, donde vive el historial completo.
- **D6. Panel y modal con un primitivo propio `Dialogo`** (`role="dialog"`, `aria-modal`,
  foco atrapado, Escape, clic en el fondo, foco devuelto al cerrar). jsdom no implementa
  `HTMLDialogElement.showModal`, asi que el `<dialog>` nativo no se puede probar.
- **D7. "Buscar otra obra" pide solo con texto** (`GET /obras?titulo=&limite=5`, con
  `useValorDiferido`): con el campo vacio no lista obras al azar del catalogo.
- **D8. Tokens nuevos del Make v9**: `--acento-borde: #f5c6d0` y `--texto-medio: #5f5f70`.
- **D9. `iniciales()` sale de `Layout.tsx`** a `web/src/iniciales.ts`: el historial es su segundo
  consumidor.

## 4. Archivos

Nuevos, en `web/src/identificacion/`:

| Archivo | Responsabilidad |
| --- | --- |
| `tipos.ts` | alias de tipos generados, rutas atadas a `keyof paths`, guardas (`esCaso`, `esPaginaDeCasos`), formato (`formatearPuntaje`, `idsDeFuente`, etiquetas), lectura del asiento (`leerResolucionAsentada`) |
| `resolucion.ts` | `resolverCaso`, `causaDelConflicto`, `MAX_NOTA`. Lo unico que depende del contrato de #175 |
| `pendientes.ts` | `usePendientesDeIdentificacion` (Layout) y `useFijarPendientes` (bandeja) sobre el contexto del Outlet |
| `Dialogo.tsx` | primitivo de panel lateral y modal centrado |
| `BandejaIdentificacion.tsx` | pantalla, lista, tarjeta del caso, candidatas, aviso |
| `PanelResolucion.tsx` | panel lateral: tres modos, busqueda, nota, estados de envio |
| `ListaOni.tsx` | tabla, filtros, paginacion, historial del registro |
| `HistorialResoluciones.tsx` | seccion del detalle de obra |

Modificados: `web/src/navegacion.ts`, `web/src/Layout.tsx`, `web/src/App.tsx`,
`web/src/catalogo/DetalleObra.tsx`, `web/src/styles.css`, `web/src/iniciales.ts` (nuevo, sale de
Layout). Tests existentes a adaptar, sin perder asercion: `navegacion.test.ts` (el auditor tampoco ve
las dos rutas nuevas; titulos con el numero de rutas), `Layout.test.tsx` (once enlaces; el logout
encolaba respuestas por orden y la del badge le robaba el 204: pasa a responder por URL y busca el
DELETE por metodo y ruta), `catalogo/DetalleObra.test.tsx` (el simulador responde la bitacora).

## 5. Comportamiento y textos

### Navegacion

`RUTAS`: `{to: "/identificacion", label: "Identificación"}` y `{to: "/lista-oni", label: "Lista ONI"}`,
`roles: ["administrador"]` (el servidor protege `/identificacion/casos` con
`x-required-roles: [administrador]`), seccion principal, entre `/distribucion` y `/anomalias`.
Iconos `PencilSquareIcon` y `QueueListIcon`. El badge sale de `pendientes`; con 0 o sin dato no se
pinta. El badge lleva " pendientes" solo para lectores de pantalla.

### Bandeja

- Cabecera: "Bandeja de identificación", pastilla "N casos pendientes" (singular "1 caso
  pendiente"), "Revisa la evidencia y decide la obra correcta. Intela nunca asigna un registro a
  ciegas." y el enlace "Ver lista ONI".
- Tarjeta (expandida por defecto, plegable con `aria-expanded`): titulo, modalidad, id; "fuente ·
  periodo · Entrega reporte_id"; "Entrada del reporte" (titulo emitido, titulo original o "—",
  fuente); "Evidencia del sistema" (IDs de fuente como fichas y el texto de evidencia); "Obras
  candidatas" con "Ordenadas por puntaje. Ninguna se asigna automáticamente." y "N coincidencias".
  Cada candidata: titulo, puntaje, "anio · genero · Comparado con “titulo_consultado”" y
  "Asignar a esta obra". Sin candidatas: "No encontramos obras candidatas" / "Busca manualmente en
  el catálogo o descarta el registro.". Pie: "Buscar otra obra" y "Descartar registro".
- Estados: cargando; error ("No pudimos cargar los casos" + mensaje + "Intentar de nuevo");
  ilegible; vacia ("No hay casos pendientes" + "La cascada de identificación procesó todos los
  registros disponibles." o, si los resolvio quien mira, "Todas las entradas fueron asignadas o
  descartadas con trazabilidad."); mas pendientes que visibles ("Se muestran X de N casos
  pendientes, en orden de llegada del reporte." + "Cargar los siguientes casos").
- Aviso de exito (`role="status"`, 4 s): "Decisión registrada" + "Registro asignado a “T”" o
  "Registro “T” descartado".

### Panel de resolucion

"Panel de resolución" + titulo por modo ("Asignar obra", "Buscar otra obra", "Descartar
registro"); "Entrada original" (id, titulo, fuente · periodo · IDs); segun el modo, "Obra elegida",
la busqueda ("Buscar en el catálogo", "Título de la obra") o el aviso del descarte ("Este registro no
se asignará a ninguna obra" + "Queda como descartado: no pondera en el reparto ni sale en el listado
público de ONI. La decisión y tu nota quedan en la bitácora."); "Nota *" con `n/300` y "La nota es
obligatoria para dejar trazabilidad de la decisión."; pie "Cancelar" y el boton principal
("Guardando…", "Descartar registro", "Asignar a T" o "Selecciona una obra").

Errores: red o 5xx: "No pudimos guardar la resolución. Revisa tu conexión e inténtalo de nuevo; tu
nota sigue aquí."; otro error de la API: "No pudimos guardar la resolución: MENSAJE. Tu nota sigue
aquí."; 409 ya resuelto: "Otra persona resolvió este caso antes" + "Recarga el caso para consultar la
decisión más reciente." + "Recargar caso"; 409 no pendiente: "Este caso ya no está pendiente" + "La
cascada de identificación lo resolvió mientras lo revisabas." + "Recargar caso"; 409 desconocido: "El
caso cambió mientras lo revisabas" + mensaje + "Recargar caso"; 409 de alias: "Ese identificador ya
apunta a otra obra" + mensaje + "Asigna el caso a esa obra o descártalo: corregir una decisión
anterior no está disponible.".

Cuerpos: candidata o busqueda `{decision: "asignar", obra_id, nota}`; descarte
`{decision: "descartar", nota}`.

### Lista ONI

"Lista ONI", "Registros que requirieron revisión en la cascada de identificación.", "Ir a casos
pendientes". Filtros "Todos los estados" (Pendiente, Asignada, Descartada), "Todas las fuentes",
"Todos los periodos", "Limpiar filtros". Columnas: "Título como vino" (+ id), "Fuente", "Periodo",
"Estado", "Responsable" (`resuelto_por.nombre` o "—"), "Última actualización" (`formatearInstante`
en `<time>`), "Acciones" ("Ver historial"; "Resolver" solo si esta pendiente). Vacia sin filtros:
"No hay registros ONI"; con filtros: "No hay coincidencias" + "Ajusta los filtros para consultar
otros registros ONI.".

Historial del registro (modal): pendiente -> "Registro enviado a revisión", "Cascada de
identificación", evidencia; asignado -> "Registro asignado a “obra”", responsable, nota, "Ver la
obra"; descartado -> "Registro descartado", responsable, nota. "Cerrar historial".

### Historial de resoluciones (detalle de obra)

"Historial de resoluciones manuales" / "Decisiones que vincularon registros de uso con esta obra." /
"N resoluciones". Por asiento `identificacion.asignada`, del mas reciente al mas antiguo: iniciales,
`actor_nombre`, fecha, "Asignó “titulo” de fuente, periodo, a esta obra.", la nota y
"uso_id · reporte_id". Un payload ilegible no se esconde: "Resolución manual con un detalle que no se
pudo leer.". Vacio: "Esta obra no tiene resoluciones manuales" / "Las decisiones futuras aparecerán
aquí con su nota y referencia de origen.".

## 6. Tests

Vitest + RTL, `fetch` global mockeado. Cada test tiene que poder fallar.

- **Unitarios** (`tipos.test.ts`, `resolucion.test.ts`): guardas, formato del puntaje, IDs de
  fuente, lectura del asiento, clasificacion del 409 con los mensajes de ejemplo del contrato,
  `resolverCaso` (metodo, ruta, cuerpo; un 2xx con cuerpo ilegible es exito).
- **Componentes**: `BandejaIdentificacion.test.tsx` (fila con candidatas y puntajes, sin
  preseleccion, forma de la llamada por cada accion, remocion optimista, restauracion si falla,
  cada 409, conteo empujado al shell), `PanelResolucion.test.tsx` (nota obligatoria, tope, busqueda,
  foco y Escape), `ListaOni.test.tsx` (estado y responsable por fila, filtros en la URL y en la
  consulta, historial), `HistorialResoluciones.test.tsx`, `Layout`/`navegacion` (badge y rutas).
- **Integracion** (`identificacion/flujo.test.tsx`, `<App />` con un servidor falso con estado):
  resolver un caso -> sale de la bandeja y del badge -> aparece como "Asignada" con su responsable
  en la lista ONI -> su historial -> "Ver la obra" -> el historial de la obra muestra usuario, fecha,
  decision y nota.
- **Contrato**: fixtures con `satisfies` sobre los tipos generados (el `tsc` del build rompe si el
  contrato cambia), rutas atadas a `keyof paths` y parametros de consulta tipados con el `query` del
  contrato.

## 7. Orden

1. Capturas y README (docs).
2. `tipos.ts` + `resolucion.ts` con sus tests.
3. Navegacion, badge y contexto del Outlet (`pendientes.ts`, `iniciales.ts`, Layout, App).
4. `Dialogo.tsx`, bandeja y panel con sus tests.
5. Lista ONI.
6. Historial en el detalle de obra.
7. Integracion, estilos finales, verificacion.

Commits pequenos por tema, locales. Sin push ni PR.

## 8. Verificacion

En `web/`: `npm run lint`, `npm run typecheck`, `npm test` (incluye `contrato:check`) y
`npm run build`. Linea base de este worktree: 48 archivos / 692 tests.

## 9. Cuando #175 mergee

1. `git fetch && git rebase main` en el worktree. El commit provisional `d88edde` queda vacio si el
   contrato no cambio (`git rebase --skip`) o choca en `api/openapi.yaml` / `contrato.d.ts`: tomar
   los de main y `npm --prefix web run contrato`.
2. Si #175 agrego el `codigo` del 409: `ApiError` lo conserva y `causaDelConflicto` lo lee en vez
   del prefijo del mensaje; actualizar sus tests.
3. Correr la verificacion y la pasada a mano del punto 10 contra el backend real.

## 10. Verificar a mano (con backend real)

- Con la semilla de casos (PR #181), la bandeja lista los pendientes con candidatas y puntajes.
- Asignar a una candidata, a una obra buscada y descartar: el caso sale, el badge baja y la lista
  ONI lo muestra resuelto con el nombre de quien lo hizo.
- El detalle de la obra muestra la resolucion con la nota.
- Dos pestanas sobre el mismo caso: la segunda recibe el 409 de "otra persona".
- Un identificador que ya apunta a otra obra: el 409 de alias deja el caso en la bandeja.

## 11. Estado al traspaso (2026-09-28)

Hecho: pasos 1 y 2 (capturas y README; `tipos.ts`, `resolucion.ts` y sus tests). `contrato.test.ts`
esta escrito y tiene UN test en rojo a proposito ("las dos rutas son solo de administrador..."):
pasa a verde en el paso 3, cuando `navegacion.ts` tenga `/identificacion` y `/lista-oni`.

#175 abrio el PR #186. Su `api/openapi.yaml` es identico al del commit provisional `d88edde` (sin
`codigo` en el 409 todavia) y ya trae `web/src/contrato.d.ts` regenerado.

Hallazgos que el plano ya recoge o que hay que tener presentes:

- El 409 de alias NO empieza como su ejemplo del contrato: Go lo envuelve
  (`resolver el uso "uso-1": ese identificador ya apunta a otra obra: ...`). Por eso
  `causaDelConflicto` busca el texto centinela DENTRO del mensaje. Hay que decirselo a #175: o se
  corrige el ejemplo, o (mejor) llega el `codigo`.
- jsdom 30 no implementa `HTMLDialogElement.showModal` (D6).
- No hay `@types/node`: `contrato.test.ts` lee el YAML con `import ... from "...openapi.yaml?raw"`.
- El badge: `useRecurso(RUTA_CONTEO_PENDIENTES, habilitado)` de `tablero/useDashboard.ts` (404, red
  o 503 = sin badge), `habilitado = puedeVer(rol, "/identificacion")`, validar solo `pendientes`
  (entero >= 0), y el valor empujado por la bandeja manda sobre el leido.
- Tests existentes que el paso 3 rompe y hay que adaptar sin perder asercion: en
  `navegacion.test.ts` el del auditor (tampoco ve las dos rutas nuevas), el titulo "nueve rutas" y
  el de secciones; en `Layout.test.tsx` "nueve enlaces" (pasan a once) y el logout, que encola
  respuestas por orden y lee `mock.calls[1]`: la peticion del badge le roba el 204.
- El paso 6 agrega una peticion a `/api/auditoria/obra/{id}` en `DetalleObra`: su simulador
  responde 404 a lo desconocido, asi que hay que darle una respuesta (`[]`) y revisar las tres
  aserciones de `role="alert"` de ese fichero.
