---
name: reparto-y-distribucion
description: Usar al implementar o revisar cualquier calculo de reparto de REDES SGC - valorizacion de obras por puntos, valor punto, deducciones legales, reservas, splits entre coautores, liquidaciones, anticipos, ONI o prescripciones. Tambien al modelar las tablas de recaudo, reparto o pago, y al escribir el motor de calculo, su firma, su aritmetica decimal o sus pruebas de reproducibilidad.
---

# Reparto y distribucion

## Modelo mental

Bolsa, no transacciones. El flujo completo:

```
recaudo del usuario (tarifa concertada)
  - deducciones legales (hasta 20% admin + hasta 10% social)
  - reserva errores tecnicos (hasta 5%, solo recaudo nacional)
  = neto a repartir del usuario/periodo
      -> se reparte entre obras por puntos
          -> se reparte entre autores por porcentaje declarado
```

Nunca sumar una columna de dinero por obra. Ningun reporte de uso trae importes.

De donde sale la bolsa es otro modulo: ver `recaudo-y-tarifas`.

**Con una excepcion, y es importante: lo anterior vale para el recaudo NACIONAL.** En el
internacional el reporte de la sociedad hermana **si discrimina montos por titular**, y de hecho
*es* la liquidacion: `RD 7.4` obliga a repartir exactamente en esa proporcion, sin que los montos
puedan modificarse, asignarse a otra persona ni alterarse. Ahi no hay bolsa que ponderar ni
valorizacion que calcular. Confundir los dos casos lleva a aplicar una ponderacion por tipo de obra
a un reparto internacional, que es justo lo que el reglamento prohibe. Ver `proceso-y-aprobaciones`.

## Antes de escribir el calculo

Leer `docs/dominio/formulas.md` completo. Tiene las cuatro formulas (TV, cine y teatro,
suscripcion, OTT), la tabla de ponderacion, el ejemplo numerico del reglamento y la tabla de
que insumos existen hoy y cuales no.

Luego `docs/dominio/reglas-negocio.md` para las reglas que rodean el calculo.

## El motor es una funcion pura

Su firma completa, y no entra nada mas:

```
reparto(bolsa, reportes_normalizados, snapshot_de_parametros, declaraciones_vigentes)
    → asignaciones
```

- **No hace E/S.** No lee base de datos, no llama a CISAC, no consulta la TRM. Todo lo que
  necesita llega como argumento; quien lo reune es la capa de aplicacion.
- **No lee el reloj.** Las fechas relevantes son datos del periodo. Cuando hace falta la hora
  actual, entra por `PuertoReloj` en la capa de aplicacion, nunca dentro del calculo.
- **No usa aleatoriedad** ni iteracion sobre colecciones sin orden. Los recorridos van sobre
  secuencias ordenadas por una clave estable.
- **No recibe ningun puerto.** Si un parametro no viene en el snapshot, el codigo no debe
  compilar. Es la unica defensa real contra que alguien incruste una ponderacion "temporalmente".

El requisito de fondo: `RD 16` somete el reparto a auditoria **en cualquier tiempo**, y `RD 13.2`
y `RD 13.4` obligan a conservar registros diez anos. Dentro de diez anos hay que poder reejecutar
el reparto de este ano y obtener **exactamente la misma cifra**, con las reglas de entonces.
Fuente: `0005-reparto-determinista-y-reproducible.md`

## Aritmetica decimal exacta

Decimal con **regla de redondeo declarada**, nunca coma flotante binaria. El valor punto es una
division (`RD 9.1.1`) y el residuo de redondeo tiene que ser explicito y reproducible, no un
artefacto del hardware. La suma repartida debe igualar la base sin perder centavos, y el residuo
se asigna con una regla escrita.

## Cada corrida fija su snapshot

Al abrirse el proceso se resuelven todos los parametros normativos a su valor **vigente en la
fecha del periodo**, el conjunto queda congelado, y la corrida guarda la referencia junto con la
version del reglamento aplicada. El motor **recibe** ese snapshot; no tiene acceso al puerto de
parametros.

Las entradas se congelan igual: los reportes crudos se guardan inmutables y versionados, y la
corrida referencia la version exacta que consumio. Un reproceso no vuelve a leer "el archivo de
Caracol": lee el archivo que se uso.
Fuente: `0004-parametros-normativos-como-dato.md`, `0005`

Reejecutar es una operacion de primera clase, y la comparacion corre en CI sobre corridas de
referencia. Los ejemplos numericos del propio reglamento — el canal Z de `RD 9.1.1`, las peliculas
X e Y de `RD 9.2` — son casos de prueba tomados de la fuente legal.

## Son dos flujos, no uno con una bandera

El calculo es puro; el **proceso** que lo rodea no lo es, y no pretende serlo. `RD 13.5` define dos
procesos con etapas y dueno por etapa, y el internacional **no tiene valorizacion**: se reparte tal
como lo discrimino la sociedad hermana (`R-14`, `RD 7.4`). Los Fees in Error van ademas **por fuera
del pipeline de deducciones**.

No modelar eso con un condicional. Ver `proceso-y-aprobaciones`.

## Errores que hay que evitar

**Repartir parcialmente cuando falta declaracion.** `R-04` / `RD 13.1.3`: si los porcentajes
no suman exactamente 100%, se retiene el **total** de esa obra. No prorratear, no pagar lo
declarado y dejar el resto. Modelar `declaracion_incompleta` como estado, no como excepcion.

**Pagar a quien no es escritor.** `R-01` y `R-02`. Solo autores del guion o libreto, personas
naturales. Los campos de director, actores, conductores y productores de los reportes sirven
para identificar la obra, no para pagar.

**Usar la duracion reportada tal cual.** `RD 9.1.1(c)`: la duracion artistica es el **80%** de
la reportada, y la hora de emision televisiva se computa como **48 minutos**.

**Aplicar la reserva del 5% al recaudo internacional.** `R-07` / `RD 14.5.4`: solo aplica al
nacional.

**Deducir sobre los Fees in Error.** `R-16` / `RD 13.7`: se devuelven integros, sin
deducciones administrativas ni de bienestar social.

**Modificar montos del recaudo internacional.** `R-14` / `RD 7.4`: se distribuye exactamente
como lo discrimina la sociedad hermana.

**Olvidar que las emisiones multiplican.** En el ejemplo de `RD 9.1.1` los puntos de la Serie Y
incorporan sus 10 emisiones, aunque la formula escrita no incluya el factor explicitamente. Ver la
nota de lectura en `formulas.md`.

## Datos que hoy no existen

No inventar valores por defecto para estos. Se modelan como **ausentes**, no como cero. Si el
calculo los necesita, debe fallar ruidosamente en vez de producir una cifra falsa:

- Rating por franja horaria (bloquea TV completo)
- Coeficientes `Wa`, `Wb`, `Wc` (bloquean OTT)
- Porcentajes de la Declaracion de Obra (bloquean todo pago)
- Base de calculo de salas de cine: el reglamento se contradice, ver `T-02`

## Parametros que cambian en el tiempo

Deben ser datos con **vigencia y organo aprobador**, no constantes en codigo: SMMLV por ano
(`R-11`, `R-31`), porcentaje de reserva aprobado por Asamblea (`R-07`), tarifas por convenio con
cada usuario (`T-11`), TRM para tarifas en dolares (`T-05`), umbral del matching difuso, y la
version del reglamento vigente en el periodo que se liquida.

Forma minima de cada parametro: clave, valor, vigencia desde, vigencia hasta, organo que lo
aprobo, acto que lo aprobo, y version del reglamento.
Fuente: `0004-parametros-normativos-como-dato.md`

## Trazabilidad

`RD 13` y `RD 16` exigen que toda cifra sea explicable y auditable, con registros conservados
diez anos (`R-13`). Cada importe calculado debe poder responder: de que recaudo salio, que
reporte de uso lo pondero, que version del reglamento y que regla se aplico, y que declaracion
de obra fijo el porcentaje.

La reproducibilidad y el linaje son complementarios, no alternativos: reejecutar demuestra que la
cifra **era correcta**, la bitacora demuestra que el sistema **hizo lo que dice**. Ver
`trazabilidad-y-auditoria`.
