# 0021 Las anomalias de un periodo son un recurso propio, no la cola de revision

Fecha: 2026-09-21
Estado: Vigente

## Contexto

OE-5 / KR-4 pide que, antes de repartir un periodo, se vea lo que impide o falsea ese reparto:
obras no identificadas, entregas duplicadas, registros duplicados, titulares sin porcentaje y
obras retenidas por declaracion incompleta. El issue #37 anade ademas la resolucion manual de
cada alerta, con actor e instante.

Al llegar a implementarlo, tres cosas del repo apuntaban a tres sitios distintos:

1. **`api/openapi.yaml` prometia que esto saldria por `GET /admin/cola-revision`.** La
   descripcion de esa ruta decia "cuando aterrice el #37, las anomalias de un periodo", y el
   schema `ItemRevision` decia servir "a las anomalias (OE-5 / #37)". El enum de
   `usos_rechazados.tipo` admite `anomalia` desde la migracion 00010.

2. **El tablero de anomalias del frontend (PR #104, abierta) ya habia fijado otro contrato.**
   `web/src/reparto/tipos.ts` declara la ruta `/api/alertas?periodo=` y el tipo
   `Alerta = { id, tipo, detalle, periodo?, referencia?, resuelta? }`, con cinco valores de
   `TipoDeAlerta`. `web/src/navegacion.ts` ya tiene `/anomalias` visible para `administrador`,
   `distribucion` y `auditor`. El propio comentario de ese fichero deja escrito el riesgo: *"el
   YAML ya reserva `/admin/*` para `administrador`. Si el pipeline aterriza ahi, este panel queda
   en 404 permanente"*.

3. **El cuerpo del issue #37 afirmaba que la tabla `alertas` ya existia.** No existia: no hay una
   sola coincidencia de `alerta` en ninguna migracion de `main`. El puerto `RepositorioAlertas`
   estaba declarado con un unico metodo `Listar(ctx)` y sin tabla, sin adaptador y sin caso de
   uso. La revision del dueno del repo sobre el issue lo corrigio y pidio que la migracion entrara
   en esta misma PR, no en una aparte.

4. **Lo que el efecto de un `tipo_obra` vacio cambio DESPUES de esa revision.** El comentario de
   revision (2026-09-15) describe la fila sin `tipo_obra` como una que *"puntua cero y no se paga,
   en silencio"*. Era exacto **para el repo de esa fecha**. Lo que lo cambio es #120 (commit
   `5f40ceb`, 2026-09-18, tres dias despues): esa PR introdujo `ponderacionTipo` con su rama
   `case "":`, que ABORTA la corrida con `ErrRepartoInvalido` en vez de puntuar cero. O sea que
   hoy el efecto es una parada dura y no un cero silencioso, y **no porque la revision se
   equivocara**: porque el motor cambio debajo. Se deja escrito aqui para que la diferencia no se
   lea como una correccion a quien reviso.

Las tres no pueden ser ciertas a la vez.

## Decision

**Las anomalias de un periodo viven en `/alertas`, un recurso de nivel superior con su propia
tabla, su propio schema y su propio estado de resolucion.** La cola de revision se queda con lo
que siempre fue: filas que no se pudieron normalizar.

1. **Recurso propio y no `/admin/*`.**
   - `GET /alertas` — `administrador`, `distribucion`, `contabilidad`, `auditor`.
   - `POST /alertas/evaluacion` — `administrador`, `distribucion`.
   - `POST /alertas/{id}/resolver` — `administrador`, `distribucion`.

   `auditor` mira y no cierra: `docs/architecture/roles.md` dice del Revisor Fiscal *"Lectura de
   todo. No opera el pipeline ni firma"*, y cerrar una alerta es una decision sobre a quien se le
   paga. `distribucion` si cierra, porque es quien persigue estas alertas con los autores.

   Que ver y resolver pidan roles distintos obliga a **dos sub-grupos de `chi`** dentro de
   `Route("/alertas")`. El chequeo no se escribe a mano en el handler: `rbac.go` explica que
   olvidar uno es justo el fallo que la seguridad de #47 quiere poder auditar en un solo sitio.

2. **Tabla propia, `alertas` (migracion 00017).** `usos_rechazados` no sirve: su clave y sus
   claves foraneas son de una FILA DE REPORTE (`reporte_id NOT NULL REFERENCES reportes`), y
   cuatro de las seis anomalias no se cuelgan de una fila de reporte — `duplicado_archivo` se
   cuelga de una entrega, y las dos de declaracion de una obra. Ademas no tiene estado de
   resolucion ni periodo.

3. **Se corrige la prosa del contrato, no su estructura.** Los dos bloques de `api/openapi.yaml`
   que prometian `/admin/cola-revision` pasan a decir a donde fue de verdad. **`anomalia` se
   queda en el enum de `ItemRevision.tipo`**: la columna lo admite desde 00010 y quitarlo seria un
   cambio de esquema que esta issue no necesita.

4. **El vocabulario lo fija el frontend, no el backend.** Los cinco strings de
   `web/src/reparto/tipos.ts` se respetan al pie de la letra y se anade el sexto,
   `tipo_obra_sin_mapear`, que la revision del issue pide (`RD 9.1.1`). Cambiar una grafia dejaria
   el tablero pintando la etiqueta cruda, y eso no lo ve ninguna prueba de Go.

5. **La clave natural de una alerta es el hallazgo**: `(periodo, tipo, ref_tipo, ref_id,
   ref_titular)`. Es lo que hace idempotente a `EvaluarAnomalias`, que se corre varias veces sobre
   el mismo periodo. `detalle` no entra: es prosa, y reescribir una frase duplicaria la fila.

6. **La referencia al registro ofensor es el par `(ref_tipo, ref_id)`**, el mismo patron
   polimorfico de `asientos` (00001), **sin clave foranea**. No puede haberla — apunta a tres
   tablas distintas — y ademas no debe: la resolucion de #39 puede borrar el uso ofensor, y una FK
   con `ON DELETE CASCADE` se llevaria por delante la alerta, que es el rastro de que aquello paso.
   `ref_titular` es la segunda coordenada y un CHECK la reserva a `titular_sin_porcentaje`.

7. **`critica` se deriva al leer, no se persiste.** Que un tipo bloquee la distribucion es
   `anomalias.EsCritica`, en el dominio. Guardarlo congelaria la clasificacion en el momento de
   detectar, y cambiarla pediria una migracion de datos.

## Alternativas consideradas

**Servirlas por `GET /admin/cola-revision`, como prometia el contrato.** Es lo que el YAML decia,
y se descarto por dos motivos independientes. El de rol: `/admin/*` es solo `administrador`, asi
que el tablero de #104 responderia 403 a `distribucion` y `auditor`, que son dos de los tres roles
que la navegacion ya declara para `/anomalias`. El de forma: `ItemRevision` no tiene campo de
estado ni de resolucion, y #37 exige `ResolverAlerta` con actor e instante. Anadirselos convertiria
el schema de la cola de normalizacion en el de otra cosa, y las filas de `usos_rechazados`
arrastrarian tres columnas que nunca usan.

**Abrir `/admin/*` a `distribucion` y `auditor`.** Arregla el 403 y rompe otra cosa: ese prefijo es
la superficie del pipeline (`/admin/pipeline`), y ampliarlo para que quepa un tablero de lectura le
daria a `auditor` la puerta de operacion que `roles.md` le niega.

**Reutilizar `usos_rechazados` con `tipo='anomalia'`.** Ver punto 2. Obligaria ademas a inventar un
`reporte_id` para las alertas de obra, y un identificador inventado en una columna con FK es un dato
falso que el esquema no puede distinguir de uno real.

**Derivar las alertas al vuelo en cada `GET`, sin tabla.** Tentador — no hay estado que mantener ni
migracion que numerar — y no funciona: sin fila persistida no hay donde escribir la resolucion, y el
criterio de #37 la pide. Ademas convertiria un listado en una consulta que recorre el periodo
entero, y el panel de #104 sondea cada quince segundos.

**Evaluar dentro del `GET /alertas`.** Ahorraria el tercer endpoint. Se descarto: un GET no escribe.
Con el sondeo cada quince segundos dejaria un asiento de bitacora por refresco, y convertiria una
lectura concurrente en una escritura concurrente.

**Un id de alerta derivado del hallazgo** (`'al-' || tipo || ref_id`) en vez de un UUID con clave
natural aparte. Serian dos claves naturales que mantener de acuerdo, y la primera vez que se
separaran la misma alerta se escribiria dos veces. Es el mismo razonamiento que la cabecera de
`asientos` ya dejo escrito, aplicado al reves.

**Derivar la clave logica del detector de duplicados por registro dentro del dominio.** Habria
obligado a duplicar en `internal/dominio` la lista de claves de `ids_fuente`, que el ADR 0018 fija
en `internal/aplicacion/idsfuente.go` como unico sitio donde se decide. La clave se deriva en la
capa de aplicacion — donde ya vive ese vocabulario — y llega al dominio ya compuesta;
`TestClaveDeRegistroCuadraConLosMapasDeIngesta` ata esa lista con los `Mapa.ClaveRegistro` de los
adaptadores para que no puedan separarse.

## Consecuencias

- El tablero de anomalias de la PR #104 funciona contra este backend **sin cambios en su cliente**:
  la ruta y los cinco tipos son los que ya declaro. Lo unico que no vera es una tarjeta propia para
  el sexto tipo, `tipo_obra_sin_mapear`. **No revienta** -- la fila sale en la tabla y
  `etiquetaDeTipo` cae a la etiqueta cruda -- pero **no tiene tarjeta KPI y no la va a tener sola**:
  `TableroAnomalias.tsx` pinta las tarjetas recorriendo `TIPOS_DE_ALERTA`, una lista ESTATICA de
  cinco, y **no importa `conteosPorTipo`**. Anadir la tarjeta es una linea en la rama de #104 y hay
  que pedirsela a quien la lleva; queda avisado en el cuerpo de esta PR.

- **`contabilidad` tiene LECTURA de `/alertas` y no escritura.** Es el mismo patron que `/bolsas` en
  el mismo router -- se lee desde mas sitios de los que se escribe -- y la razon es normativa, no de
  comodidad: `docs/architecture/roles.md` describe a contabilidad como *"La otra firma de las mismas
  compuertas"* y el ADR 0008 cita `RD 13.8.6`, el control del dinero de las ONI **con aval de
  Contabilidad**. Lo que decide esa firma es, entre otras cosas, cuantas alertas criticas siguen
  abiertas en el periodo: sin lectura firmaria a ciegas. Se queda **fuera de escritura** porque
  perseguir la anomalia con los autores es trabajo de `distribucion` (`RD 13.5`), y resolver una
  alerta es decir que alguien se hizo cargo de esa gestion.

  No es una ampliacion gratuita: `web/src/navegacion.ts` de la PR #104 ya declara `/anomalias` para
  los cuatro roles, con el comentario *"Contabilidad es la segunda firma de la compuerta: sin
  /anomalias no ve el aviso de alertas abiertas antes de firmar. Roles a alinear con el
  `requiereRol` de #17 cuando aterrice el middleware."* El middleware es este. Sin esta linea,
  contabilidad veria la entrada de menu y comeria 403 -- y `web/src/tablero/ausente.ts` deja escrito
  que un 403 *"es un bug de permisos y debe verse"*, o sea banner rojo.

- `GET /alertas` es una lectura pura. `POST /alertas/evaluacion` sigue siendo el disparador
  manual para mirar el tablero antes de tiempo.

- **La compuerta de dinero vive en `Procesos.AvanzarEtapa` (#34).** Al salir de `deducciones`
  (hacia `importe_obra` en el nacional, `liquidacion_parcial` en el internacional) el proceso
  consulta el puerto `CompuertaAnomalias`. Su implementacion, `Anomalias.Bloqueantes`, **evalua
  primero** el periodo con el actor de sistema y despues cuenta las criticas abiertas: un periodo
  nunca evaluado, o que recibio entregas despues de la ultima pasada, no puede dar "0 porque nadie
  miro". Si queda alguna, `ErrAnomaliasCriticasAbiertas` y la API responde 409. Sin la compuerta
  cableada la transicion falla (cerrada, no abierta); `cmd/api` y `cmd/lambda` la cablean y
  `cmd/lambda/cableado_test.go` lo vigila sobre el AST. `cmd/worker` no la necesita: solo abre
  corridas, nunca avanza.

  La compuerta es por **periodo**, no por bolsa: una critica de cualquier fuente del periodo
  detiene todas sus corridas. Es mas estricto de lo necesario para una bolsa de otro canal, y es
  lo que se puede afirmar hoy sin una relacion alerta -> bolsa.

- **Que significa "resuelta" para el dinero.** Resolver exige una nota no vacia
  (`ErrNotaObligatoria` -> 400, y el CHECK `alerta_resuelta_tiene_nota`), que viaja al asiento
  `alerta.resuelta`: es la justificacion auditable de la decision. Pero **resolver no cambia los
  datos que pondera el reparto**: la fila o la entrega duplicada sigue ponderando, y una obra sin
  `tipo_obra` sigue sin poder ponderarse. "Resuelta" solo afirma que una persona, con nombre y
  razon escrita, acepta que el periodo avance tal como esta. La accion correctiva (excluir la fila
  o la entrega, asignar la obra o el tipo) es de #39; la issue de seguimiento esta redactada en
  `docs/planes/37/issues-de-seguimiento.md`.

- **Una alerta resuelta no se reabre**, aunque una pasada posterior vuelva a detectar la misma
  anomalia (`ON CONFLICT DO NOTHING`, no `DO UPDATE`). Es deliberado: la resolucion de #39 actua
  sobre el registro ofensor, asi que si la anomalia sigue ahi es porque el registro sigue igual, y
  reabrirla borraria la decision de quien la cerro. La contrapartida es real y queda declarada: una
  alerta cerrada por error hay que reabrirla a mano en la base hasta que #39 traiga esa operacion.

- El detector de `tipo_obra_sin_mapear` levanta **una alerta por fila**, no por obra. Sobre la
  parrilla real de Caracol eso son tantas alertas como filas identificadas, porque su mapa de
  columnas deja `tipo_obra` vacio a proposito hasta que el cliente conteste la pregunta P-05. Es la
  cifra correcta — ninguna de esas filas se puede ponderar — pero conviene saberlo antes de mirar el
  tablero.

- `anomalias` es un modulo nuevo del dominio y trae dos aristas nuevas al grafo del ADR 0003:
  lee `repertorio` (para `Declaracion.Completa()`, el unico criterio de `R-04`) y `identificacion`
  (para las constantes de escalon). No toca dinero, y la regla `modulos-anomalias` de
  `.golangci.yml` le deniega `reparto`, `recaudo`, `liquidacion` y `anticipos`. El diagrama
  `docs/diagrams/PATIC2 - Arquitectura.drawio` se actualiza **en esta misma PR** con el modulo y sus
  dos aristas, que es lo que el checklist pide; la frontera que manda sigue siendo `depguard`
  (ADR 0012), el diagrama la acompana.
