---
name: proceso-y-aprobaciones
description: Usar al implementar o revisar el flujo del reparto de REDES SGC y sus autorizaciones - las etapas del RD 13.5, liquidacion parcial contra final, compuertas de verificacion, doble firma, roles del reglamento, la diferencia entre el proceso nacional y el internacional, Fees in Error, o el calendario de distribucion. Tambien al modelar la maquina de estados de una corrida, al anadir una etapa, y siempre que se escriba codigo que autorice una salida de dinero.
---

# Proceso de reparto y aprobaciones

## El error que esta skill existe para evitar

Leer `docs/dominio/formulas.md` sugiere que el reparto es un job por lotes: cargar la bolsa,
deducir, ponderar, dividir, pagar. El diagrama de flujo previo del proyecto lo modela asi.

`RD 13.5` dice otra cosa: define **dos procesos nombrados, con etapas y dueno por etapa**, y mete
humanos en medio a proposito. Un job de una sola pasada no tiene donde poner las firmas que exige
el reglamento, y convierte la verificacion en un paso opcional fuera del sistema.

Hay una tension que conviene nombrar en voz alta: el **calculo** tiene que ser una funcion pura y
reproducible (`0005`), y el **proceso** que lo rodea no puede serlo. Son cosas distintas y no se
contradicen. Ver `reparto-y-distribucion` para el calculo.

Fuente: `0008-reparto-como-flujo-con-aprobaciones.md`

## Las dos secuencias

```
Nacional:      Recaudo → Deducciones → Importe de la Obra → Importe por Titular →
               Liquidacion Parcial → Verificacion → Liquidacion Final →
               Pago y Registro → Auditoria

Internacional: Recaudo → Deducciones → Liquidacion Parcial → Verificacion →
               Liquidacion Final → Pago y Registro → Fees in Error → Auditoria
```

Fuente: `RD 13.5`

### Lean las dos listas comparadas

**El camino internacional no tiene valorizacion.** No hay Importe de la Obra ni Importe por
Titular, porque `RD 7.4` obliga a repartir exactamente en la proporcion que discrimino la sociedad
hermana: los montos **no** pueden modificarse, asignarse a otra persona ni alterarse para
beneficiar a otro autor (`R-14`).

**El internacional tiene una etapa que el nacional no tiene: Fees in Error.** Y ademas devuelve el
dinero **sin deducciones administrativas ni de bienestar social**, es decir **por fuera** del
pipeline de deducciones, en el menor tiempo posible y con soporte que indique obras y montos
reintegrados (`R-16`, `RD 13.7`).

## Dos maquinas de estado, no una con un `if`

Nacional e internacional son **tipos separados**. Comparten el concepto y muy poco mas.

Meterlos en una sola maquina con banderas produce, tarde o temprano, un reparto internacional al
que se le aplico una ponderacion por tipo de obra — que es exactamente lo que `RD 7.4` prohibe. La
diferencia entre los dos flujos esta en su parte mas cara, la valorizacion, y viene de una
prohibicion legal, no de una preferencia de diseno.

`ProcesoDeReparto` es un agregado con identidad, periodo, tipo, **snapshot de parametros fijado al
abrirse** y etapa actual. Avanzar de etapa es una operacion del dominio con precondiciones
explicitas, no un paso de un script.

## Las compuertas son de primera clase

`Verificacion` y `Pago y Registro` no avanzan sin las firmas que exige el reglamento. El dinero no
sale con una firma sola, y el reglamento es insistente en ello:

| Escenario | Quien avala | Fuente |
| --------- | ----------- | ------ |
| Ajuste y pago de una reclamacion | Revisoria Fiscal o auditoria interna **y** Distribucion y contabilidad | `RD 14.5.10` a `RD 14.5.12` |
| Pago a sociedades extranjeras | Documentacion, distribucion y contabilidad; solo a sociedades con **contrato de representacion vigente** | `RD 13.6`, `R-15` |
| Control del dinero de las ONI | Distribucion y Gerencia, con aval de Contabilidad | `RD 13.8.6` |
| Anticipos | Acta y resolucion individual del Consejo Directivo | `RA 3.2` |

Es **el mismo patron de doble control en los cuatro casos**, asi que el modulo de Aprobaciones es
uno solo y transversal. Una compuerta guarda **quien firmo, en que rol, cuando, y sobre que
version del contenido**.

Roles, que son los del reglamento y no roles inventados de aplicacion: Consejo Directivo, Revisor
Fiscal, Distribucion, Contabilidad, Gerencia.

Un booleano `aprobado_por` no sirve: no soporta doble firma, no guarda el rol, y no dice sobre que
version se firmo. La auditoria de `RD 16` pregunta las tres cosas.

## Una compuerta rechazada retrocede, no aborta

El proceso vuelve a la etapa anterior con el motivo registrado. Un reparto **no puede quedar a
medias**: `RD 13.5` lo describe como un ciclo que termina en Auditoria.

Que el proceso se detenga en una compuerta durante dias no es un fallo: es como funciona de verdad
una sociedad de gestion colectiva.

## El calendario abre el proceso; no lo ejecuta

`RD 12` deja las fechas al Consejo Directivo, que fija los rangos y puede modificarlos por fuerza
mayor con re-notificacion. Frecuencia minima: **al menos una vez por ano calendario** (`R-09`).

El planificador consulta el `CalendarioDeDistribucion` **del dominio** y dispara
`AbrirProcesoDeReparto`. No es dueno de las fechas y no lleva expresiones cron escritas a mano.
A partir de ahi el proceso avanza por accion humana o por trabajos que el propio proceso encola.

Los trabajos con reloj propio y fijo por reglamento tambien entran como parametros, no como cron:
reserva semestral (`RD 14.5.2`), cortes de rendimientos el **20 de octubre** (nacional) y el
**30 de septiembre** (internacional) (`R-34`, `RD 10.1`, `RD 10.2.1`), padron de socios en marzo
(`RS 5.2`).
Fuente: `0004-parametros-normativos-como-dato.md`

## Plazos que corren dentro del proceso

- **Liquidacion aceptada por silencio a los 15 dias calendario** desde el envio (`R-10`, `RD 13.2`).
- **Reclamaciones: 15 dias habiles** para responder, individualmente y por escrito (`R-22`,
  `RD 14.3`). Calendario y habiles no son lo mismo; no unificarlos.
- **Anticipos**: notificacion a mas tardar el segundo dia habil, giro dentro de los diez habiles
  siguientes (`RA 3.2`).

El tiempo entra por `PuertoReloj`, inyectado, nunca leido dentro del calculo. Es lo que permite
probar una prescripcion de diez anos sin esperar una decada (`0002`).

## El motor de calculo no conoce nada de esto

Lo invoca la etapa que corresponde, recibe el snapshot y devuelve asignaciones. No conoce la
maquina de estados, no sabe que hay compuertas, y se puede reejecutar solo. Esa separacion es lo
que hace posible `0005`.

## Riesgo a vigilar

La tentacion de anadir una ruta rapida que salte compuertas para pruebas o para cerrar un periodo
con prisa. Una vez que existe, se usa. Por eso avanzar de etapa debe ser **la unica** operacion que
mueve el proceso: sin forma de fijar la etapa directamente, y sin que el salto de una compuerta sin
firma sea un caso de uso que exista.
