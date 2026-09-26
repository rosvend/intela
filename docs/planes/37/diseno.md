---
issue: 37
actualizado: 2026-09-25
---

# Deteccion de anomalias: razones de diseno

La decision de fondo (recurso propio, tabla, roles, compuerta, que significa "resuelta") esta en
[ADR 0021](../../decisiones/0021-alertas-como-recurso-propio.md). Aqui va el porque de cada
detector de `internal/dominio/anomalias` y de la migracion `00019_alertas_de_anomalias.sql`. El
codigo remite aqui en vez de repetirlo.

## D0. Orden y determinismo

`Detectar` ordena por `(Tipo, RefTipo, RefID, RefTitular)` y no por orden de deteccion: dos
pasadas sobre el mismo dato devuelven la misma lista (ADR 0005). Ninguna funcion recorre un
mapa; los mapas solo se consultan. `Periodo` llega con sus listas ordenadas para que los mensajes
("ya venia en el uso X") nombren siempre al mismo.

## D1. ONI: el filtro es el escalon, nunca la bandera `oni`

`usos.oni` es `DEFAULT TRUE` (00001): una fila recien ingerida llega con `escalon='pendiente'` y
`oni=TRUE`. Filtrar por la bandera contaria como ONI todo lo que todavia no se intento identificar.
`excluido` (00007, `R-27`) tampoco es ONI: esta fuera del repertorio a proposito. `manual` ya lo
resolvio una persona.

## D2. Duplicado por huella: solo lo que el UNIQUE deja pasar

`reportes` tiene `UNIQUE (sha256, fuente)`: la misma fuente con los mismos bytes ya falla al
insertar (`ErrReporteDuplicado`). Lo que se busca es la colision entre fuentes distintas, y como
el periodo no esta en esa clave la otra pata puede estar en otro mes: por eso `Entregas` son
todas las conocidas. Se alerta sobre CADA entrega del periodo que colisiona (las dos ponderan) y
el detalle nombra la otra pata.

## D3. Duplicado por registro

- La ingesta ya rechaza el registro repetido dentro de un archivo (`Mapa.ClaveRegistro`); esto
  caza la repeticion entre archivos del mismo periodo, que infla los puntos de una obra y desinfla
  los del resto del canal.
- No filtra por entrega: las dos puertas no miran lo mismo (celdas crudas contra clave
  normalizada), y un duplicado dentro de una entrega sigue contando dos veces.
- La clave llega derivada de la capa de aplicacion (vocabulario del ADR 0018). Clave vacia no se
  compara: agruparlas marcaria como duplicadas todas las filas de las que no se sabe nada y
  bloquearia el periodo. `SinClaveDeRegistro` cuenta ese punto ciego y sube a
  `usos_sin_cotejar`. Caso real: `Hora` no es requerida en `MapaCaracol` y es parte de su clave.
- La fuente entra en la clave: los ids de fuente no cruzan entre fuentes.
- La alerta va sobre la fila repetida, no sobre la primera.
- Que fuentes tienen clave y cuales no: P-21 en `docs/dominio/preguntas-cliente.md`.

## D4. Titulares sin porcentaje y D5. Retencion por declaracion incompleta

Miran el mismo hecho desde dos alturas: la retencion habla de la OBRA y del dinero (una alerta por
obra, `R-04`); titulares habla de la PERSONA a quien perseguir (una alerta por obra e IPI).
Colapsarlas perderia el nombre.

- Una obra sin ninguna declaracion no entra en titulares (serian N avisos iguales); la cubre la
  retencion con una alerta y `aQuienReclamar` pone los IPI del catalogo en el detalle. Una version
  abierta y vacia si entra.
- Titulares levanta dos casos: IPI de `obra_coautores` sin parte, y parte con IPI vacio (el
  esquema lo admite). No busca porcentajes cero: el CHECK y `NuevaDeclaracion` ya lo impiden.
- El criterio de "completa" es `repertorio.Declaracion.Completa`, no una suma propia: un segundo
  criterio de `R-04` divergiria. `motivoIncompleta` solo narra el motivo real, en el mismo orden.
- `RefTitular` lleva prefijo (`ipi:` / `titular:`) porque los dos casos nombran espacios
  distintos que comparten columna; sin prefijo la clave natural colapsaba dos hallazgos en cuanto
  un `titulares.id` coincidia con un IPI.

## D6. `tipo_obra` sin mapear

`RD 9.1.1` pondera por cuatro categorias y `usos.tipo_obra` admite vacio (el mapa de Caracol no
la trae hasta P-05). Desde #120 el motor aborta la corrida (`ErrRepartoInvalido`, `case ""`): la
alerta es el preaviso. Hoy la parada esta latente porque `MapaCaracol` tampoco mapea `canal_id` y
`UsosDeCanal` no devuelve esas filas. Rellenar `tipo_obra` desde `obras.tipo` se midio y empeora:
sin `rating` la corrida reparte cero con error nil. Los tres huecos (`tipo_obra`, `canal_id`,
`rating`) van en su propia issue (texto en `issues-de-seguimiento.md`, B).

Solo filas con obra identificada (las demas no llegan al motor) y solo modalidades cuyo motor lee
el campo: `ponderacionTipo` solo la llama `puntosTV`, alcanzado por TV, suscripcion y hotel. Sin
ese filtro un periodo de Netflix levantaba una critica por fila y la compuerta no abria nunca. La
tabla vive en `ponderaPorTipoObra` y una prueba de `aplicacion` la cruza con
`reparto.Modalidades()`. Modalidad desconocida cuenta como que pondera: ante la duda, ruido.

## D7. Criticidad (`EsCritica`)

Critica = dejarla sin resolver hace que las cifras salgan mal:

- `duplicado_archivo`, `duplicado_registro`: el valor punto es un cociente; la obra duplicada se
  lleva de mas y todas las del canal de menos.
- `tipo_obra_sin_mapear`: el motor aborta (y solo dispara donde el motor lee el campo).
- `oni`: no; es una etapa del diseno (ADR 0007), su parte se reserva (`R-19`).
- `reserva_declaracion_incompleta`: no; `R-04` es un estado valido.
- `titular_sin_porcentaje`: no; es la causa de la anterior.

`TipoReservaDeclaracionIncompleta` se llama asi por el contrato de #104; lo que mide es la
RETENCION de `R-04`, no la reserva por errores tecnicos de `R-07`.

## D8. La migracion 00019

- Tabla propia y no `usos_rechazados`: sus FK son de fila de reporte, no tiene estado ni periodo
  (ADR 0021, punto 2).
- `(ref_tipo, ref_id)` sin FK, como `asientos`: apunta a tres tablas, y #39 puede borrar el uso
  ofensor; un CASCADE se llevaria el rastro.
- Clave natural `UNIQUE (periodo, tipo, ref_tipo, ref_id, ref_titular)`: idempotencia de
  `Evaluar`. `detalle` no entra (es prosa).
- No es append-only: una alerta registra que algo SIGUE pasando. El rastro de cada cambio va a
  `asientos` en la misma transaccion.
- CHECKs: firma de persona XOR autocierre del sistema, nota obligatoria al resolver, `ref_titular`
  solo para `titular_sin_porcentaje`.
- UUID de la base como `id`: un id derivado del hallazgo seria una segunda clave natural.
- Indices: bandeja por periodo, abiertas por periodo y tipo (lo que cuenta la compuerta), y por
  registro ofensor (bandeja de #39).
- Historia del numero: nacio 00015, paso a 00016 al mergear #144 y a 00017 al mergear #80
  (renumera el que mergea segundo).
