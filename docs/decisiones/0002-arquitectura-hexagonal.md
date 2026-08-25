# 0002 Arquitectura hexagonal con frontera Clean

Fecha: 2026-08-10
Estado: Vigente
Modificada por: [0012 La frontera se verifica sobre el codigo, no sobre el diagrama](0012-la-frontera-se-verifica-sobre-el-codigo.md) — la mitigacion en CI
sigue existiendo, pero se verifica sobre los `import` con `depguard`, no sobre el diagrama.
(Via [0011](0011-verificacion-del-diagrama-como-aviso.md), ya sustituida.)

## Contexto

Cuatro cosas del dominio de REDES SGC se contradicen con la intuicion de cualquiera que se siente
a programar un sistema de regalias, y las cuatro producen codigo plausible y equivocado:

1. Ningun reporte de uso trae importes. La bolsa la fija el Reglamento de Tarifas y los reportes
   solo la ponderan (`docs/dominio/formulas.md`).
2. Los porcentajes de reparto salen unicamente de la Declaracion de Obra, nunca de las columnas
   `Autor*` y `Guionista*` de una parrilla (`R-03`, `R-02`).
3. Si lo declarado no suma 100%, no se reparte nada de esa obra (`R-04`, `RD 13.1.3`).
4. Solo se paga a escritores personas naturales (`R-01`, `RD 4.5`).

Un error en cualquiera de ellas no se manifiesta como una excepcion: produce un numero, el numero
se paga, y aparece en una auditoria de `RD 16` anos despues. La disciplina no puede depender de que
quien escriba el codigo se acuerde de las cuatro.

Ademas el proyecto esta en fase de analisis. No hay stack elegido, faltan datos del cliente
(declaraciones de obra, reportes de recaudo, feed de rating, coeficientes `Wa/Wb/Wc`), y no se sabe
todavia si Intela reemplaza a REDES-SYS y AVSYS, se integra con ellos, o convive. Cualquier decision
que ate el dominio a una tecnologia concreta hoy se va a pagar cuando esas respuestas lleguen.

## Decision

**Puertos y adaptadores, con la regla de dependencia de Clean Architecture: nada del nucleo nombra
nada de afuera.**

El nucleo tiene dos anillos. El interior es el dominio: entidades, invariantes y las funciones de
calculo, sin E/S. El exterior es la capa de aplicacion: casos de uso, orquestacion y limites de
transaccion. Todo lo demas es un adaptador y vive fuera.

La frontera se cruza siempre por un puerto:

- **Puertos de entrada** son los casos de uso. Los adaptadores de entrada (portales, API, cargador
  de archivos, planificador, webhooks, CLI) los invocan y no conocen nada mas del nucleo.
- **Puertos de salida** son interfaces que el nucleo declara y que los adaptadores implementan:
  repositorios, catalogos externos, dispersion de pagos, contabilidad, notificaciones, almacen de
  documentos, bitacora, parametros normativos, reloj, motor de similitud.

Los cuatro invariantes de arriba dejan de ser disciplina y pasan a ser estructura:

- El tipo `ReporteDeUso` no tiene campo de dinero, asi que **no existe la operacion de sumar
  importes por fila**.
- Las columnas de creditos de una parrilla entran por el puerto de identificacion, tipadas como
  evidencia de matching. El puerto de titularidad es otro y solo acepta declaraciones. **No hay
  camino de tipos desde una parrilla hasta un porcentaje de pago.**
- `declaracion_incompleta` es un estado de la obra, no un flag de error, y el motor solo sabe
  retener el total o repartir el total.
- `Titular` exige IPI de persona natural. **No existe firma que emita una orden de pago a un
  productor.**

Dos puertos que parecen infraestructura y son dominio, y por eso se declaran explicitamente:

- **`PuertoReloj`.** El tiempo entra inyectado. Las prescripciones de 3 y 10 anos (`R-19`, `R-20`) y
  las ventanas de 15 dias (`R-10`, `R-22`) tienen que poder reproducirse en una auditoria y probarse
  sin esperar una decada.
- **`PuertoNotificaciones`.** Devuelve un acuse persistible. Notificar no es enviar un correo: es el
  hecho que arranca el reloj de prescripcion (`RD 13.8.8`).

## Alternativas consideradas

**Capas clasicas (presentacion, negocio, datos).** Es lo que se ensena y lo que produciria un equipo
sin instrucciones. Descartada porque la dependencia apunta hacia la base de datos: el negocio importa
el ORM, y en la practica el esquema termina dictando el modelo. Aqui el modelo lo dicta el
reglamento, y el reglamento cambia por decision del Consejo Directivo, no por una migracion.

**Transaction script: un servicio por caso de uso, sin modelo de dominio.** Es la opcion mas rapida
para llegar a un demo y es tentadora en un proyecto academico. Descartada porque los cuatro
invariantes quedarian repetidos en cada script y bastaria con que uno se olvide en uno solo. El
valor del modelo aqui no es la elegancia, es que hace imposible el error caro.

**Microservicios desde el dia uno**, uno por contexto delimitado. Descartada por sobreingenieria:
ver `0003-monolito-modular.md`.

**Un motor de reglas configurable** en vez de codigo, sobre la premisa de que los reglamentos
cambian. Descartada: los reglamentos cambian en sus **cifras** (porcentajes, ponderaciones,
tarifas), no en su estructura, y esas cifras ya salen a datos por
`0004-parametros-normativos-como-dato.md`. Un motor de reglas generico pagaria el precio de la
indireccion para resolver un problema que ya esta resuelto de forma mas barata, y haria mucho mas
dificil explicar una cifra, que es justo lo que exige `RD 16`.

## Consecuencias

Positivas: se puede construir y probar todo el reparto sin base de datos, sin banco y sin CISAC,
usando implementaciones en memoria de los puertos, lo cual importa porque **hoy faltan casi todos
esos insumos**. La pregunta abierta de si Intela reemplaza a REDES-SYS y AVSYS o se integra con
ellos deja de bloquear: es la eleccion de un adaptador. Y la frontera es verificable sobre el
codigo: se puede comprobar que ningun paquete del nucleo importa nada de fuera sin pasar por un
puerto. El diagrama de la pagina 1 de `docs/diagrams/PATIC2 - Arquitectura.drawio` documenta esa
misma frontera, pero no la hace cumplir ([ADR 0012](0012-la-frontera-se-verifica-sobre-el-codigo.md)).

A cambio: mas indireccion. Guardar una obra pasa por una interfaz en vez de llamar al ORM, y hay que
escribir y mantener las implementaciones de cada puerto. Para un CRUD seria un costo injustificado;
aqui se paga porque el sistema mueve dinero de terceros y responde ante la DNDA.

Riesgo asumido: la frontera se erosiona sola. Basta un `import` del cliente HTTP dentro de un caso de
uso, hecho con prisa, para que deje de valer. La mitigacion es un test de arquitectura en CI que
falle si el paquete del dominio importa cualquier cosa de infraestructura: la etapa
`Architecture boundary`, que corre `depguard` sobre los `import` reales con las reglas de
[`.golangci.yml`](../../.golangci.yml). El diagrama no participa en ese criterio
([ADR 0012](0012-la-frontera-se-verifica-sobre-el-codigo.md)).
