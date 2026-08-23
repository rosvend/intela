# 0008 El reparto es un flujo con compuertas humanas, y son dos flujos distintos

Fecha: 2026-08-10
Estado: Vigente

## Contexto

La lectura natural de `docs/dominio/formulas.md` sugiere que el reparto es un calculo por lotes: se
carga la bolsa, se aplican deducciones, se ponderan las obras, se divide entre autores, se paga. El
diagrama de flujo previo del proyecto (`docs/Flujo de datos B.png`) lo modela asi.

`RD 13.5` dice otra cosa. Define **dos procesos nombrados, con etapas y dueno por etapa**:

```
Nacional:      Recaudo → Deducciones → Importe de la Obra → Importe por Titular →
               Liquidacion Parcial → Verificacion → Liquidacion Final → Pago y Registro → Auditoria

Internacional: Recaudo → Deducciones → Liquidacion Parcial → Verificacion →
               Liquidacion Final → Pago y Registro → Fees in Error → Auditoria
```

Dos cosas saltan a la vista al comparar las listas. La primera: **el camino internacional no tiene
valorizacion**. No hay Importe de la Obra ni Importe por Titular, porque `RD 7.4` obliga a repartir
exactamente en la proporcion que discrimino la sociedad hermana, sin que los montos puedan
modificarse ni reasignarse. Y tiene una etapa que el nacional no tiene, Fees in Error, que ademas
devuelve el dinero **sin deducciones administrativas ni de bienestar social** (`R-16`, `RD 13.7`),
es decir por fuera del pipeline de deducciones.

La segunda: hay etapas que no son calculo, son autorizacion. Y el reglamento es insistente en que el
dinero no sale con una firma sola:

- `RD 14.5.10` a `RD 14.5.12`: el ajuste y pago de una reclamacion requiere aval escrito de
  Revisoria Fiscal o auditoria interna **y** de Distribucion y contabilidad.
- `RD 13.6`: los pagos a sociedades extranjeras van avalados por documentacion, distribucion y
  contabilidad, y solo a sociedades con contrato de representacion vigente.
- `RD 13.8.6`: el control del dinero de las ONI corresponde a Distribucion y Gerencia, con aval de
  Contabilidad.
- `RA 3.2`: cada anticipo se analiza uno por uno, con acta y resolucion individual del Consejo
  Directivo, notificacion a mas tardar el segundo dia habil y giro dentro de los diez dias habiles
  siguientes.

Hay tension con `0005-reparto-determinista-y-reproducible.md`, y merece decirse en voz alta: el
**calculo** tiene que ser una funcion pura, y el **proceso** que lo rodea no puede serlo, porque el
reglamento mete humanos en medio a proposito.

## Decision

**Se separa el calculo del proceso, y se modelan dos maquinas de estado distintas, no una con un
condicional.**

`ProcesoDeReparto` es un agregado con identidad, periodo, tipo (nacional o internacional), snapshot
de parametros fijado al abrirse, y etapa actual. Avanzar de etapa es una operacion del dominio con
precondiciones explicitas, no un paso de un script.

**Nacional e internacional son tipos separados.** Comparten el concepto y muy poco mas: los
conjuntos de etapas son distintos, el internacional no invoca el motor de valorizacion, y Fees in
Error no pasa por deducciones. Meterlos en una sola maquina con banderas produciria, tarde o
temprano, un reparto internacional al que se le aplico una ponderacion por tipo de obra, que es
exactamente lo que `RD 7.4` prohibe.

**Las compuertas son de primera clase.** `Verificacion` y `Pago y Registro` no avanzan sin las firmas
que exige el reglamento. Una compuerta guarda quien firmo, en que rol, cuando, y sobre que version
del contenido. Los roles son los del reglamento: Consejo Directivo, Revisor Fiscal, Distribucion,
Contabilidad, Gerencia. El modulo transversal de Aprobaciones es el mismo para reclamaciones
(`RD 14.5.10-12`), pagos al exterior (`RD 13.6`), ONI (`RD 13.8.6`) y anticipos (`RA 3.2`): la forma
del doble control es la misma en los cuatro casos.

**Una compuerta rechazada retrocede, no aborta.** El proceso vuelve a la etapa anterior con el
motivo registrado. Un reparto no puede quedar a medias: `RD 13.5` lo describe como un ciclo que
termina en Auditoria.

**El calendario abre el proceso; no lo ejecuta.** `RD 12` deja las fechas al Consejo Directivo, que
puede modificarlas por fuerza mayor con re-notificacion. El planificador consulta el
`CalendarioDeDistribucion` del dominio y dispara `AbrirProcesoDeReparto`. A partir de ahi el proceso
avanza por accion humana o por trabajos que el propio proceso encola.

**El motor de calculo sigue siendo puro.** Lo invoca la etapa correspondiente, recibe el snapshot y
devuelve asignaciones. No conoce la maquina de estados, no sabe que hay compuertas, y se puede
reejecutar solo, que es lo que hace posible `0005`.

## Alternativas consideradas

**Un job por lotes de una sola pasada.** Es lo que sugiere el diagrama de flujo previo y lo mas
simple de construir. Descartada: no hay donde poner las firmas que exige el reglamento, y la
Liquidacion Parcial existe justamente para que alguien verifique antes de que exista la Final. Un
job de una pasada convierte esa verificacion en un paso opcional fuera del sistema.

**Una sola maquina de estados con una bandera nacional/internacional.** Descartada. Ahorra codigo
duplicado a cambio de que cada etapa tenga que preguntarse de que tipo es, y de que el dia que
alguien anada una etapa se olvide de una rama. Los dos flujos difieren en su parte mas cara, la
valorizacion, y esa diferencia viene de una prohibicion legal.

**Un motor de BPM o de workflow externo.** Descartada: la maquina de estados esta escrita en el
reglamento y es pequena, con nueve y ocho etapas fijas. Un motor externo anadiria un despliegue y un
lenguaje mas para expresar algo que cabe en un tipo del dominio, y sacaria fuera del nucleo una
regla que es dominio puro.

**Aprobaciones como un campo booleano `aprobado_por`.** Descartada: no soporta doble firma, no guarda
el rol ni sobre que version se firmo, y no sirve para el patron que se repite en reclamaciones,
pagos al exterior, ONI y anticipos. La auditoria de `RD 16` pregunta quien autorizo, con que rol y
sobre que.

**Dejar las aprobaciones fuera del sistema**, en actas y correos, y que Intela solo registre el
resultado. Es lo que probablemente pasa hoy. Descartada porque rompe la cadena de trazabilidad justo
en el eslabon donde sale el dinero: se podria explicar como se calculo una cifra pero no quien
autorizo pagarla.

## Consecuencias

Positivas: el estado de un reparto es consultable y auditable en cualquier momento, que es lo que
`RD 16` permite exigir sin aviso. El doble control existe una vez y lo usan los cuatro escenarios que
lo requieren. Y el proceso puede detenerse en una compuerta durante dias sin que eso sea un fallo,
que es como funciona de verdad una sociedad de gestion colectiva.

A cambio: mas complejidad que un job por lotes, y dos maquinas de estado que mantener con algo de
codigo parecido entre ellas. La consola administrativa tiene que soportar el flujo de firmas, con sus
roles y sus rechazos.

Riesgo asumido: la tentacion de anadir una ruta rapida que salte compuertas para pruebas o para
cerrar un periodo con prisa. Una vez que existe, se usa. La mitigacion es que avanzar de etapa sea la
unica operacion que mueve el proceso, que no haya forma de fijar la etapa directamente, y que el
salto de una compuerta sin firma no sea un caso de uso que exista.
