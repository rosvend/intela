# 0005 El calculo del reparto es una funcion pura y reproducible

Fecha: 2026-08-10
Estado: Vigente

## Contexto

`RD 16` somete todo proceso de distribucion a auditoria **en cualquier tiempo y lugar**, interna por
el Revisor Fiscal y externa por la DNDA o por auditores que autorice el Consejo Directivo. `RD 13.2`
y `RD 13.4` obligan a conservar registros y evidencias por un minimo de diez anos. Y
`docs/reglamentos/README.md` deja claro que los repartos historicos se calcularon con la version del
reglamento vigente en su momento.

Juntando las tres cosas, el requisito real no es "guardar el resultado". Es: **dentro de diez anos
hay que poder volver a correr el reparto de este ano y obtener exactamente la misma cifra**, con las
reglas de entonces, para demostrar que el numero que se pago fue el que correspondia.

Eso descarta cualquier calculo que dependa de la hora en que se ejecuta, del orden en que llegaron
las filas, de un valor consultado en vivo a un servicio externo, o de una configuracion que desde
entonces cambio.

Hay una tension que conviene nombrar: `RD 13.5` describe el reparto como un flujo con etapas,
dueno por etapa y verificaciones humanas. Eso **no** es determinista, y no pretende serlo. Ver
`0008-reparto-como-flujo-con-aprobaciones.md`. Esta ADR es sobre el calculo, no sobre el proceso.

## Decision

**El motor de reparto es una funcion pura.** Su firma completa es:

```
reparto(bolsa, reportes_normalizados, snapshot_de_parametros, declaraciones_vigentes) → asignaciones
```

Nada mas entra. En concreto:

- **No hace E/S.** No lee base de datos, no llama a CISAC, no consulta la TRM. Todo lo que necesita
  llega como argumento; quien lo reune es la capa de aplicacion.
- **No lee el reloj.** Las fechas relevantes son datos del periodo. Cuando hace falta la hora
  actual, entra por `PuertoReloj` en la capa de aplicacion, nunca dentro del calculo.
- **No usa aleatoriedad ni iteracion sobre colecciones sin orden.** Los recorridos van sobre
  secuencias ordenadas por una clave estable.
- **Aritmetica decimal exacta con regla de redondeo declarada**, nunca coma flotante binaria. El
  valor punto es una division (`RD 9.1.1`) y el residuo de redondeo tiene que ser explicito y
  reproducible, no un artefacto del hardware.

**Cada corrida fija su snapshot.** Al abrirse, el proceso resuelve todos los parametros normativos a
su valor vigente (ver `0004-parametros-normativos-como-dato.md`), congela el conjunto, y guarda la
referencia junto con la version del reglamento aplicada. El motor recibe ese snapshot; no tiene
acceso al puerto de parametros.

**Las entradas se congelan igual que los parametros.** Los reportes crudos se guardan inmutables y
versionados en el almacen de objetos, y la corrida referencia la version exacta que consumio. Un
reproceso posterior no vuelve a leer "el archivo de Caracol": lee el archivo que se uso.

**Reejecutar es una operacion de primera clase**, expuesta por el CLI de operaciones. Reejecutar una
corrida cerrada contra su propio snapshot debe dar un resultado identico, y esa comparacion corre en
CI sobre corridas de referencia.

## Alternativas consideradas

**Guardar solo el resultado y confiar en el.** Es lo mas barato y lo que hace la mayoria de los
sistemas de liquidacion. Descartada porque no responde la pregunta que hace un auditor. Guardar que
a un autor le tocaron $219.024 no demuestra que $219.024 fuera lo correcto; solo demuestra que eso
fue lo que se pago.

**Registrar cada paso intermedio en una bitacora y reconstruir el razonamiento leyendola.** Es lo que
resuelve `0006-trazabilidad-como-asiento-append-only.md` y es necesario, pero no suficiente:
demuestra que el sistema hizo lo que dice que hizo, no que lo que hizo fuera correcto. La
reproducibilidad y el linaje son complementarios, no alternativos.

**Event sourcing sobre el proceso completo**, reconstruyendo el estado reproduciendo eventos.
Descartada como mecanismo principal: da reproducibilidad del estado pero arrastra el problema de la
evolucion del esquema de eventos a diez anos, que es exactamente el horizonte en el que esto tiene
que seguir funcionando. Un snapshot explicito de entradas y parametros es mas simple de auditar y no
depende de que el codigo que interpreta los eventos antiguos siga existiendo.

**Contenerizar la version del sistema de cada ano** y reejecutar la imagen historica. Descartada
como estrategia principal, aunque conviene conservar las imagenes igual. Depender de poder levantar
un contenedor de hace diez anos es una apuesta fragil: cambia el hardware, cambia el runtime, y la
imagen puede no arrancar. Ademas no ayuda si lo que hace falta es explicar una cifra, solo
recalcularla.

## Consecuencias

Positivas: el motor se puede probar exhaustivamente sin infraestructura, incluyendo los ejemplos
numericos del propio reglamento (el canal Z de `RD 9.1.1`, las peliculas X e Y de `RD 9.2`) como
casos de prueba tomados de la fuente legal. Un reproceso por reclamacion (`RD 14.5.9` paga el ajuste
en el reparto siguiente) parte de una base verificable. Y la suite de CI que reejecuta corridas
historicas convierte "no rompimos el calculo" en algo que se comprueba en cada commit.

A cambio: la capa de aplicacion carga con todo el trabajo de reunir insumos antes de llamar al
motor, y eso es codigo que en un diseno mas laxo no existiria. Congelar reportes y parametros por
corrida ocupa almacenamiento que crece y del que no se puede borrar nada durante diez anos.

Riesgo asumido: basta una llamada al reloj o una consulta en vivo dentro del motor para que la
propiedad se pierda, y el sistema seguiria funcionando con normalidad, asi que nadie lo notaria
hasta la auditoria. La mitigacion es doble: el motor no recibe ningun puerto en su firma, y la
prueba de CI reejecuta corridas de referencia y falla si el resultado cambia. Si esa prueba se
desactiva, esta ADR deja de valer.
