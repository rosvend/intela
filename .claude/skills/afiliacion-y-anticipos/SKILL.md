---
name: afiliacion-y-anticipos
description: Usar al modelar o implementar quien es afiliado de REDES SGC y que puede pedir - Socio contra Titular Administrado, admision, exclusividad de sociedad, padron IPI, herederos y derechohabientes, documentos para cobrar (RUT y certificacion bancaria), o cualquier cosa relacionada con anticipos sobre derechos futuros, sus topes, su recoupment y quien tiene derecho a pedirlos. Tambien al decidir si alguien puede reclamar un reparto de un periodo pasado.
---

# Afiliacion y anticipos

## La distincion que decide permisos

Todo afiliado es una de dos cosas, y la diferencia no es cosmetica: gobierna quien puede pedir un
anticipo y quien tiene derechos politicos.

**Socio** — vinculo **societario**. Es titular originario: guionista, libretista, autor de teatro,
escritor de radio. Se subdivide en honorarios (sin derechos politicos ni patrimoniales),
fundadores y activos. `RS 4.1`

**Titular Administrado** — vinculo **contractual**, no societario. Son quienes cumpliendo
requisitos optan por esta figura, quienes perdieron la calidad de socio, y los **derechohabientes**
(herederos). `RS 4.2`

Requisito minimo para ser socio activo: ser titular originario de al menos **una obra literaria
explotada publicamente** (`R-29`, `RS 4.1`).

## Exclusividad de sociedad

No se acepta como afiliado a quien pertenezca a otra SGC del mismo genero, en el pais o en el
exterior, **sin renuncia previa y expresa**.
Fuente: `R-28`, `RS 1`, `RS 4.1`, Decision Andina 351 Art. 45 literal k

Implementacion: es una precondicion de admision con evidencia adjunta, no una casilla de
verificacion. Guardar el documento de renuncia, no solo el booleano.

## La afiliacion es una consulta punto-en-el-tiempo

Este es el error facil de cometer. **No se pregunta "¿es afiliado?" sino "¿estaba afiliado
entonces?"**:

- No se atiende el reclamo de un escritor que no estuviera afiliado a REDES SGC, o a una sociedad
  con contrato de representacion, **al cierre del proceso de distribucion** (`R-24`, `RD 14.5.5`,
  `RD 14.5.8`).
- Los autores de sociedades hermanas reclaman **por medio de su sociedad**; no se reciben
  reclamaciones directas (`R-25`, `RD 14.5.7`).

Implementacion: la afiliacion es un intervalo con vigencia, no un estado actual. Un modelo que
guarde `activo: boolean` no puede responder la pregunta que hace el reglamento, y el fallo aparece
como un pago indebido dos anos despues.

## Documentos para poder cobrar

RUT actualizado y certificacion bancaria. Si el pago va a un tercero: autorizacion escrita y
firmada **mas** la certificacion bancaria de ese tercero.
Fuente: `R-12`, `RD 13.1.6`

Estos documentos se conservan en el almacen de objetos y su ausencia bloquea la orden de pago, no
la liquidacion: se puede liquidar a quien no puede cobrar todavia.

## Identificacion: IPI

**IPI** identifica **personas**, administrado por SUISA dentro del sistema CISAC. Es el
identificador correcto de los titulares (`RD 3`). No confundir con **IDA**, que identifica
**obras**. Ver `matching-de-obras`.

El repositorio ya tiene `data/IPI - form to report members to IPI 01-03-24.xls`, el formato con el
que se reportan miembros al sistema IPI. Todavia no esta perfilado: `src/scripts/sample.py` solo
recorre `data/files/`.

## Anticipos

Un anticipo es una **liquidacion adelantada de derechos futuros**, no un prestamo. Se cubre
automaticamente con la obra que el afiliado siga produciendo. Esa naturaleza explica todas las
restricciones que siguen.

### Solo los Socios pueden pedirlo

Los Titulares Administrados quedan excluidos precisamente porque el anticipo se cubre con obra
futura, y los herederos o titulares derivados **no seguiran creando**.
Fuente: `R-30`, `RA 2.2`

Es la regla que mas se rompe por intuicion: parece discriminacion arbitraria y es una consecuencia
directa del mecanismo de recoupment.

### Topes, y son dos simultaneos

No puede superar el **25%** de los derechos generados a favor del socio en los repartos ordinarios
de los **ultimos dos anos** (nacional e internacional sumados), y en ningun caso puede superar
**5 SMMLV**.
Fuente: `R-31`, `RA 3.1.g`, `RA 3.1.h`

Se aplica el menor de los dos. El SMMLV es un parametro normativo por ano, no una constante
(`0004`).

### Un solo anticipo vigente

No se otorga uno nuevo mientras exista saldo pendiente, ni a afiliados con **embargos reportados**.
Fuente: `R-32`, `RA 3.1.c`, `RA 3.1.d`, `RA 3.1.e`

### Recoupment automatico

El anticipo se descuenta automaticamente con cargo a los derechos futuros del afiliado, en los
procesos de recaudo **nacional e internacional**.
Fuente: `R-33`, `RA 2.1`

Implementacion: el descuento ocurre en la etapa de liquidacion, sobre el importe ya calculado del
titular. No toca la valorizacion de la obra ni el valor punto: si lo tocara, el anticipo de un
autor alteraria lo que cobran sus coautores.

### Cada anticipo se aprueba uno por uno

Analisis individual, con **acta y resolucion individual del Consejo Directivo**, notificacion a
mas tardar el segundo dia habil y giro dentro de los diez dias habiles siguientes. Las
aprobaciones viven en un repositorio electronico con su ubicacion referenciada, textualmente "con
el fin de garantizar trazabilidad".
Fuente: `RA 3.2`, `RA 3.2.5`

Es el mismo patron de doble control que rige reclamaciones, pagos al exterior y ONI. Ver
`proceso-y-aprobaciones` y `trazabilidad-y-auditoria`.

## Frontera del modulo

`Afiliacion` no conoce a nadie: es el modulo del que dependen `Repertorio`, `Liquidacion y Pago`,
`Reclamaciones` y `Anticipos`, y el que no depende de ninguno. Mantenerlo asi es lo que permite
responder "¿estaba afiliado en ese periodo?" sin arrastrar el resto del sistema.
Fuente: pagina 2 del diagrama de arquitectura, `0003-monolito-modular.md`.
