# 0006 La trazabilidad es un asiento append-only, no un log de aplicacion

Fecha: 2026-08-10
Estado: Vigente

## Contexto

`CLAUDE.md` fija el requisito en una frase: toda cifra que el sistema produzca debe ser explicable
hasta su origen, es decir fuente, reporte y regla aplicada. El reglamento lo respalda desde varios
lados:

- `RD 13.2` exige que el pago vaya acompanado de un resumen con **montos brutos, deducciones y
  netos**, y conservar registros y evidencias de transferencias y pagos por un minimo de diez anos.
- `RD 13.4` obliga a que la documentacion este numerada y fechada, que las carpetas digitales esten
  numeradas y fechadas, y remite al Art. 60 del Codigo de Comercio: conservar cuando menos diez anos,
  y destruir despues solo si se garantiza reproduccion exacta.
- `RD 16` somete todo a auditoria interna y externa en cualquier tiempo.
- `RA 3.2.5` pide que las aprobaciones de anticipo vivan en un repositorio electronico con su
  ubicacion referenciada en los documentos, textualmente "con el fin de garantizar trazabilidad".

Un log de aplicacion no cumple nada de esto. Se escribe para depurar, no para probar; rota y se
borra por politica de retencion; no tiene esquema estable; y a los diez anos ni existe ni serviria.

Conviene separar dos cosas que suelen confundirse. **Observabilidad** es infraestructura: logs,
metricas y trazas para operar el sistema, con retencion corta y sin valor probatorio.
**Trazabilidad** es dominio: el registro de por que una cifra es la que es, con valor ante la DNDA.
En el diagrama son cajas distintas a proposito.

## Decision

**La trazabilidad es un libro de asientos append-only dentro del dominio, escrito por
`PuertoBitacoraAuditoria`.**

Un asiento nunca se modifica ni se borra. Corregir es escribir otro asiento que referencia al
anterior, igual que en contabilidad.

Cada cifra que el sistema produce deja un asiento que responde estas preguntas sin necesidad de
recalcular nada:

- **De donde salio la bolsa**: usuario, convenio, tarifa aplicada, periodo, factura.
- **Que reportes la ponderaron**: version exacta del archivo crudo en el almacen de objetos.
- **Como se reconocio la obra**: si fue alias conocido, identificador global, coincidencia difusa
  con su puntaje, o decision manual, y en ese ultimo caso quien la tomo y cuando.
- **Que reglas se aplicaron**: la referencia al snapshot de parametros y a la version del reglamento
  de esa corrida (ver `0004` y `0005`).
- **Como se dividio entre autores**: la declaracion de obra vigente que se uso, con su version.
- **Que se dedujo**: bruto, cada deduccion con su concepto y su porcentaje, reserva, y neto.
- **Quien aprobo**: las firmas de las compuertas de doble control (ver `0008`).

Reglas de implementacion que son parte de la decision:

- **Cada modulo publica sus propios asientos.** Ninguno escribe en la trazabilidad de otro. Es lo que
  mantiene la frontera de `0003-monolito-modular.md`.
- **Append-only exigido por el motor de almacenamiento**, no por convencion: sin `UPDATE` ni `DELETE`
  sobre la tabla, mas copia inmutable en el almacen de objetos con bloqueo de retencion. La
  disciplina de no borrar no puede depender de que nadie escriba la sentencia.
- **La informacion economica de las ONI no se publica.** El listado publico lleva titulos e
  informacion identificatoria y nunca los montos (`R-18`, `RD 13.8.2`). El asiento guarda el monto;
  la vista publica no lo expone. Trazabilidad interna completa no es lo mismo que publicidad.
- **La notificacion es un asiento, no un efecto secundario.** El acuse con su fecha arranca el reloj
  de prescripcion de diez anos (`RD 13.8.8`, `R-20`, `R-21`), asi que se persiste por titular y por
  corrida.

El caso de uso `ExplicarCifra` recorre estos asientos y es lo que consumen el Portal del Titular y el
Portal de Auditoria. Que un titular pueda ver el origen de su dinero y que un auditor pueda
verificarlo son la misma consulta con distinto alcance.

## Alternativas consideradas

**Logs estructurados con retencion larga.** Descartada. No tienen esquema estable, se escriben para
depurar, y nadie firma un dictamen de auditoria sobre una consulta a un agregador de logs. Ademas
mezclarian ruido operativo con evidencia legal.

**Tablas de historial con disparadores**, guardando el antes y el despues de cada fila. Descartada
como mecanismo principal: registra **que cambio en la base de datos**, no **por que la cifra es
esa**. Un auditor no pregunta que fila se actualizo; pregunta de donde salieron los $219.024.

**Event sourcing como fuente de verdad de todo el sistema**, reconstruyendo el estado desde los
eventos. Descartada: el estado actual pasaria a depender de que el codigo capaz de interpretar
eventos de hace diez anos siga existiendo, y ese es justo el horizonte de conservacion. Aqui el
estado vive en tablas normales y el libro de asientos es un registro paralelo con esquema propio y
estable, disenado para ser legible por un humano dentro de una decada.

**Delegar la trazabilidad al ERP contable.** Descartada: el ERP registra el asiento contable del
pago, no la cadena que va desde una emision en una parrilla hasta el porcentaje declarado por un
coautor. Esa cadena solo la conoce Intela.

## Consecuencias

Positivas: responder a una reclamacion (`R-22` da quince dias habiles) es una consulta, no una
investigacion. La auditoria de `RD 16` se atiende con acceso de solo lectura al Portal de Auditoria
en vez de con exportaciones manuales. Y la exigencia de `RD 13.2` de mostrar bruto, deducciones y
neto sale del mismo sitio que alimenta el panel del titular.

A cambio: los asientos crecen mas rapido que los datos operativos y no se pueden podar durante diez
anos, asi que el almacenamiento hay que dimensionarlo desde el principio. Y cada caso de uso que
mueve dinero tiene que escribir su asiento, lo cual es codigo adicional en todos ellos.

Riesgo asumido: un asiento incompleto es peor que no tenerlo, porque da falsa confianza. Si se
guarda el resultado del matching pero no el puntaje de similitud ni quien resolvio la coincidencia
manual, la cadena se rompe justo donde un auditor va a mirar. La mitigacion es tratar el asiento
como parte de la definicion de hecho de cada caso de uso, no como instrumentacion que se anade
despues.
