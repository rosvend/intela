---
name: trazabilidad-y-auditoria
description: Usar al implementar o revisar cualquier cosa que tenga que dejar rastro en Intela - la bitacora de asientos, el linaje de una cifra, el caso de uso ExplicarCifra, retencion de documentos, el listado publico de ONI, acuses de notificacion, o al responder a un auditor, el Revisor Fiscal o la DNDA. Tambien al decidir si algo va al log de aplicacion o a la bitacora de dominio, y siempre que se escriba un caso de uso que mueva dinero.
---

# Trazabilidad y auditoria

## La distincion que hay que tener clara antes de escribir una linea

**Observabilidad** es infraestructura: logs, metricas y trazas para operar el sistema. Retencion
corta, esquema inestable, cero valor probatorio.

**Trazabilidad** es **dominio**: el registro de por que una cifra es la que es, con valor ante la
DNDA. Retencion de diez anos, esquema estable, disenado para que lo lea un humano dentro de una
decada.

En el diagrama de arquitectura son cajas distintas a proposito. Meter la trazabilidad en el
agregador de logs es el error clasico: nadie firma un dictamen de auditoria sobre una consulta a
Grafana, y el log ya se roto antes de que llegue la auditoria.

Fuente: `0006-trazabilidad-como-asiento-append-only.md`

## Que exige el reglamento

- `RD 13.2` — el pago va acompanado de un resumen con **montos brutos, deducciones y netos**, y se
  conservan registros y evidencias de transferencias y pagos por un minimo de **diez anos**.
- `RD 13.4` — documentacion numerada y fechada, carpetas digitales numeradas y fechadas. Remite al
  Art. 60 del Codigo de Comercio: conservar cuando menos diez anos, y destruir despues solo si se
  garantiza reproduccion exacta.
- `RD 16` — auditoria interna (Revisor Fiscal) y externa (DNDA o auditores que autorice el Consejo
  Directivo) **en cualquier tiempo y lugar**.
- `RA 3.2.5` — las aprobaciones de anticipo viven en un repositorio electronico con su ubicacion
  referenciada en los documentos, "con el fin de garantizar trazabilidad".

Traducido: la auditoria puede llegar sin aviso, y la respuesta tiene que ser una consulta, no una
investigacion.

## El asiento es append-only

Un asiento **nunca se modifica ni se borra**. Corregir es escribir otro asiento que referencia al
anterior, igual que en contabilidad.

**Exigido por el motor de almacenamiento, no por convencion.** Sin `UPDATE` ni `DELETE` sobre la
tabla, mas copia inmutable en el almacen de objetos con bloqueo de retencion. La disciplina de no
borrar no puede depender de que nadie escriba la sentencia: basta una migracion apurada para
perder la propiedad, y nadie lo nota hasta que importa.

## Las siete preguntas que todo asiento debe responder

Sin necesidad de recalcular nada:

1. **De donde salio la bolsa** — usuario, convenio, tarifa aplicada, periodo, factura.
2. **Que reportes la ponderaron** — la version exacta del archivo crudo en el almacen de objetos,
   no "el archivo de Caracol".
3. **Como se reconocio la obra** — alias conocido, identificador global, coincidencia difusa **con
   su puntaje**, o decision manual; en ese ultimo caso quien la tomo y cuando.
4. **Que reglas se aplicaron** — referencia al snapshot de parametros y a la version del
   reglamento de esa corrida (`0004`, `0005`).
5. **Como se dividio entre autores** — la declaracion de obra vigente que se uso, con su version.
6. **Que se dedujo** — bruto, cada deduccion con su concepto y su porcentaje, reserva, y neto.
7. **Quien aprobo** — las firmas de las compuertas de doble control (`0008`).

Un asiento incompleto es **peor** que no tenerlo, porque da falsa confianza. Si se guarda el
resultado del matching pero no el puntaje ni quien resolvio la coincidencia manual, la cadena se
rompe justo donde el auditor va a mirar. Tratar el asiento como parte de la definicion de hecho de
cada caso de uso, nunca como instrumentacion que se anade despues.

## Reglas de implementacion

**Cada modulo publica sus propios asientos.** Ninguno escribe en la trazabilidad de otro. Es lo que
mantiene la frontera del monolito modular (`0003`).

**La notificacion es un asiento, no un efecto secundario.** El acuse con su fecha **arranca el
reloj de prescripcion** de diez anos, asi que se persiste por titular y por corrida. Se entiende
notificado el envio del proyecto de reparto al correo que el socio informo, **o** su puesta a
disposicion en la pagina web de la sociedad: ambas vias son validas y ambas producen acuse.
Fuente: `R-21`, `RD 13.8.8`, `R-20`, `RD 15.1`

Por eso `PuertoNotificaciones` devuelve un acuse persistible en vez de `void`. Notificar no es
enviar un correo: es el hecho juridico que empieza a contar el plazo.

**La informacion economica de las ONI no se publica.** El listado publico lleva titulos e
informacion identificatoria y **nunca los montos**; la informacion economica se mantiene en
reserva. El asiento guarda el monto, la vista publica no lo expone. Trazabilidad interna completa
no es lo mismo que publicidad.
Fuente: `R-18`, `RD 13.8.1` a `RD 13.8.4`

## ExplicarCifra

El caso de uso que recorre los asientos. Lo consumen el **Portal del Titular** y el **Portal de
Auditoria**: que un titular vea el origen de su dinero y que un auditor lo verifique son la misma
consulta con distinto alcance.

Es tambien lo que hace que responder una reclamacion quepa en los quince dias habiles que da
`R-22` / `RD 14.3`.

## Que no sirve, y por que

**Logs estructurados con retencion larga** — sin esquema estable, escritos para depurar, mezclan
ruido operativo con evidencia legal.

**Tablas de historial con disparadores** — registran *que cambio en la base de datos*, no *por que
la cifra es esa*. Un auditor no pregunta que fila se actualizo; pregunta de donde salieron los
$219.024.

**Event sourcing como fuente de verdad** — el estado actual pasaria a depender de que el codigo
capaz de interpretar eventos de hace diez anos siga existiendo, y ese es justo el horizonte de
conservacion.

**Delegar al ERP contable** — el ERP registra el asiento contable del pago, no la cadena que va
desde una emision en una parrilla hasta el porcentaje declarado por un coautor. Esa cadena solo la
conoce Intela.

## Complemento, no sustituto

La bitacora demuestra que el sistema **hizo lo que dice que hizo**. La reproducibilidad del motor
(`0005`) demuestra que lo que hizo **era correcto**. Hacen falta las dos: guardar que a un autor le
tocaron $219.024 no prueba que $219.024 fuera lo que le correspondia. Ver
`reparto-y-distribucion`.
