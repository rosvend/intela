# 0022 Descartar un caso de identificacion es un escalon propio

## Contexto

La bandeja de casos ONI (#39) permite dos cosas sobre un uso que la cascada no reconocio:
darle una obra, o decir que **no es un uso del repertorio de REDES SGC**. La segunda no
existia en el modelo: `usos.escalon` tenia siete valores y ninguno significaba "una persona
lo miro y decidio que aqui no va".

El efecto es monetario y por eso hay que decidirlo una vez y por escrito: un uso que no
pondera no recibe parte, y la bolsa se reparte entre las obras que si ponderan. El mismo
efecto que una exclusion R-27.

Se consideraron dos caminos, y los dos tienen una trampa:

1. **Reusar `excluido`.** Es el escalon que ya existe para "fuera de repertorio" (R-27,
   `RD 9.5`) y tiene exactamente la forma que hace falta: sin obra y con `oni = false`. Pero
   `ResolverUsos` **reprocesa** las filas excluidas en cada corrida, a proposito: la
   exclusion es configuracion (`identificacion.FuentesExcluidas`) y tiene que poder
   corregirse. Una decision humana guardada ahi se desharia sola en la corrida siguiente.
2. **Dejarlo en ONI con una nota.** No cambia el esquema, pero `oni = true` significa "no se
   pudo reconocer" (`RD 13.8`), y una persona acaba de decir lo contrario: que si se
   reconocio y que no es del repertorio. El uso seguiria saliendo en el listado publico de
   ONI y seguiria contando como pendiente en la bandeja.

## Decision

Se anade el escalon **`descartado`** a `usos.escalon` (migracion 00022):

- El uso queda **sin obra**, `oni = false` y **no pondera**. Fuera del listado publico de
  ONI, fuera del indice `usos_pendientes` y fuera del motor de reparto.
- Va **firmado**: `resuelto_por`, `resuelto_en` y una **nota** obligatoria de hasta 300
  caracteres, en `usos.nota_resolucion` y en el payload del asiento. La firma dice quien, la
  nota dice por que, y la auditoria de `RD 16` pregunta las dos cosas (ADR 0006).
- **La cascada no lo vuelve a tocar.** `reprocesable()` devuelve `false` para `manual` y para
  `descartado`: una decision humana no se pisa. Solo `oni` es resoluble a mano, y corregir
  una decision anterior —reasignar, o deshacer un descarte— queda fuera de alcance.
- Las citas: `RD 7.1` (REDES SGC solo representa autores de guion o libreto), `RD 9.5` y
  R-27 (queda fuera lo que no es catalogo de REDES). `RD 13.8` es el regimen ONI, que
  precisamente **no** aplica al descartado.

La asignacion manual, que es la otra mitad, no estrena escalon: usa `manual`, que ya existia
reservado para la cola desde 00001, y aprende ademas el alias del par canonico (fuente, tipo,
valor) del uso con `quien` = el usuario que decidio (ADR 0007: resolver una vez, reutilizar
siempre).

## Alternativas consideradas

- **Una tabla `usos_descartados` aparte.** Duplicaria la fila para no tocar un CHECK, y el
  uso pasaria a tener dos sitios donde estar "resuelto". La tabla `usos` ya tiene siete
  escalones: el vocabulario crece, pero el sitio donde se guarda el desenlace sigue siendo
  uno.
- **Un booleano `descartado` en paralelo a `escalon`.** Dos columnas que dicen lo mismo se
  contradicen el dia que alguien escriba una sola.
- **Cerrar la alerta `oni` de #37 al descartar.** No hace falta: `deteccionONI` filtra por
  `escalon = 'oni'`, asi que la siguiente evaluacion ya no la detecta y el autocierre la
  cierra con el actor de sistema. Que la cierre una persona impediria que se reabriera sola
  si el uso volviera a ONI.

## Consecuencias

- El vocabulario de `escalon` pasa a ocho valores, y `uso_resuelto_tiene_obra` y
  `manual_tiene_autor` se extienden a `descartado` (misma rama que `excluido` la primera,
  misma firma que `manual` la segunda).
- **Hoy `descartado` y ONI pesan lo mismo: cero.** La reserva ONI de R-18 no esta
  implementada (dueno #33/#34), asi que la parte de un uso ONI tampoco se reserva: se
  reparte entre las obras identificadas. La diferencia entre los dos escalones se vuelve
  real el dia que exista la reserva, y entonces el descartado tendra que quedar fuera de
  ella por construccion — su parte no existe.
- Resolver un caso toca un periodo que puede estar en reparto o ya distribuido, asi que la
  resolucion se serializa con el **cerrojo de periodo** de #171, la misma clave que la
  compuerta de anomalias, la ingesta y la valorizacion. Una corrida ya valorizada no cambia:
  sus asientos `reparto.obra_valorizada` congelaron como se identifico cada uso. El dinero de
  lo ya distribuido va por `RD 13.8.5`, que paga "en el siguiente proceso", cuando exista la
  reserva ONI.
- Queda pendiente de confirmar con el PO (P-22): si el descarte es un concepto propio o una
  marca de "revisado, no es repertorio", y si su efecto monetario debe ser el de R-27 (la
  parte no existe) o el de `RD 13.8` (la parte queda en reserva hasta prescribir).
- Efecto lateral conocido: el detector `duplicado_registro` de #37 no mira el escalon, asi
  que descartar una fila duplicada **no apaga** su alerta critica; hay que resolverla en
  `/alertas`. Con `excluido` pasa lo mismo hoy.
