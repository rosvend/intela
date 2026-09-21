---
issue: 32
actualizado: 2026-09-21
---

# Escalon 3: matching difuso

Decisiones de diseno del escalon 3 de la cascada del [ADR 0007](../../decisiones/0007-identificacion-en-cascada-con-cola-manual.md).
El codigo remite aqui en vez de repetirlo.

## D1. La similitud se calcula en la base, la decision en el dominio

`Similitud.Candidatos` es un puerto. El adaptador de PostgreSQL puntua y recorta; el dominio
decide. El reparto de responsabilidades:

| | Quien | Por que |
| --- | --- | --- |
| Normalizar el titulo | adaptador (SQL) | es la misma operacion con la que esta indexado el catalogo; en dos sitios divergen |
| Puntuar y ordenar | adaptador (pg_trgm) | con indice; en Go seria N x M sin indice |
| Recortar por el piso | adaptador | traer lo que nadie va a mirar es mover filas para tirarlas |
| Aplicar el umbral | dominio | decide si se asigna una obra: es negocio |
| Elegir entre candidatos | dominio | el desempate tiene que ser reproducible (ADR 0005) |

Esto **no** rompe la regla de dependencia: el nucleo no nombra SQL ni pg_trgm. El dia que entre
un modelo de resolucion de entidades —lo que el ADR 0007 deja previsto para este escalon— se
cambia el adaptador y nada mas.

Alternativas descartadas, con el motivo:

- **Algoritmo en Go puro.** Sin indice: obliga a traer el catalogo entero por cada fila.
- **Una libreria de Go** (`go-edlib`, `levenshtein`). Lo mismo, mas una dependencia para algo que
  PostgreSQL ya trae. Tendria sentido como *re-ranker* de los ≤5 candidatos; hoy es YAGNI.
- **Un servicio en Python.** Segundo runtime, segundo artefacto y un salto de red dentro del
  bucle de la cascada. El puerto existe para poder hacerlo el dia que haya datos con los que
  entrenar; hoy seria coste sin beneficio.

## D2. Dos umbrales, no uno

Hay **tres** desenlaces, no dos:

```
puntaje >= matching.umbral        -> escalon 'difuso', se asigna la obra
puntaje >= matching.umbral_banda  -> ONI + candidatos adjuntos (cola manual, #39)
por debajo                        -> ONI sin candidatos
```

Con un solo corte, "no llego al umbral" mezclaria un 0.58 —que una persona resuelve en diez
segundos— con un 0.02, que no tiene nada que mirar.

Las dos ultimas son ONI. Lo que las distingue es si hay filas en `candidatos_match`.

Los dos valores son parametros normativos con vigencia (ADR 0004), sembrados sinteticos:
`matching.umbral = 0.60`, `matching.umbral_banda = 0.45`. **No estan calibrados**, y con la
muestra actual no hay con que hacerlo: 0 candidatos sobre 0.6 sobre 59 y 49 filas sin solape
(`docs/dominio/identificadores.md`). El sesgo por defecto es mandar a revision antes que decidir
mal.

## D3. Los umbrales se leen una vez por corrida, contra la fecha del periodo

No contra el reloj: lo que se defiende en una reclamacion es el criterio vigente **cuando se
decidio**. Y una sola vez porque, leyendolos por fila, un cambio de parametro a mitad de lote
partiria la corrida en dos criterios distintos sin que nada lo registrara.

Se lee con `ParametroEnFecha`, un puerto de una sola pregunta, deliberadamente mas estrecho que
`ParametrosNormativos` (#118): identificar una obra no mueve dinero (ADR 0003) y no arrastra la
maquinaria de congelar un snapshot.

Un parametro ausente es un error que **nombra la clave**, nunca un cero: un umbral en cero
asignaria la primera obra que se pareciera en algo a cualquier titulo.

## D4. El difuso aprende alias

Igual que el escalon 2. Resuelto una vez que `show_id 80141259` es la obra X, todo reporte futuro
con ese id entra por el escalon 1, a coste cero.

El precio: un match difuso equivocado no se queda en su fila, se convierte en un alias que
resuelve mal todas las siguientes sin volver a puntuar. Por eso el umbral arranca conservador y
por eso cada match guarda su `puntaje` — un alias malo se encuentra buscando por el puntaje con
que se aprendio.

## D5. Orden de escritura

```
candidatos -> alias -> match
```

Siempre. Si la corrida muere entre dos pasos, la fila sigue en su escalon anterior y el reintento
la rehace entera. Al reves quedaria una ONI con la bandeja vacia, que no le sirve a quien la abra.

`GuardarMatch` sigue siendo condicional al escalon con que se leyo la fila: una resolucion manual
concurrente no se pisa.

## D6. Que filas reprocesa una corrida

| escalon | entra | por que |
| --- | --- | --- |
| `pendiente` | si | la sembro la ingesta |
| `excluido` | si | la lista de fuentes es configuracion, y pudo estar mal |
| `oni` | si | **el catalogo crece**: una obra dada de alta hoy identifica un uso que el mes pasado no se parecia a nada |
| `alias`, `id_global`, `difuso` | no | ya resuelta; el conocimiento vive en `alias_obra` |
| `manual` | **no** | una decision humana no se pisa con una automatica |

Reprocesar las ONI cuesta una consulta de similitud por fila en cada corrida. Es el precio de que
el sistema mejore solo a medida que se declara repertorio; sin eso, un uso se queda en ONI hasta
que prescriba aunque su obra ya este declarada.

## D7. Los creditos no puntuan

`Autor*`, `Guionista*` y `Director*` identifican la obra, **no** determinan a quien se le paga
(`R-02`, `R-03`, `RD 7.3.3`). `identificacion.Entrada` ni siquiera los transporta, y hay una
prueba que vigila la forma del tipo para que no aparezcan "para mejorar el matching".

## D8. Un fallo del motor no es "no hay match"

Aborta la corrida. Tragarselo mandaria a ONI una fila que quiza se identificaba sola, y
desenredarlo despues es trabajo manual sobre dinero retenido.

## D9. `candidatos_match` no es una cache

Es la evidencia de que algo se considero y se descarto, con cuanto. Sin ella, la bandeja de #39
tendria que volver a correr la consulta y mostraria el catalogo de **hoy**, no lo que habia
cuando se decidio. Un auditor de `RD 16` pregunta por lo segundo.

Se reemplaza entera por uso, no se acumula: son el resultado de una corrida contra el catalogo tal
como estaba.

## D10. La busqueda del catalogo comparte el indice, no el umbral

`GET /obras?titulo=` casa por subcadena (`ILIKE`) **o** por parecido, y ordena por parecido. Eso
es lo que hace que el buscador tolere tildes, mayusculas y orden de palabras.

Su corte es `pg_trgm.similarity_threshold` en su valor de fabrica (0.3). Es una caja de busqueda:
lo que sale de ella no decide a quien se le paga. El corte que si decide es el del escalon 3, y
ese es un parametro con vigencia.

## Ver tambien

- [`explain-trgm.md`](explain-trgm.md) — por que el indice es GiST y no GIN, medido.
- [`../28-cascada-identificacion/01-design.md`](../28-cascada-identificacion/01-design.md) — escalones 0-2.
