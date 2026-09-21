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

### Recuperar sin una transaccion por titulo: medido, no se puede (review de #146, S2)

`Similitud.Candidatos` abre una transaccion por titulo para fijar `pg_trgm.similarity_threshold`
con `SET LOCAL`, porque `%` -el operador que usa el indice- toma su corte del GUC y no de un
argumento. El review propuso una consulta KNN (`ORDER BY titulo_norm <-> $1, id LIMIT n` con el
piso como filtro) que no lo necesitara. **No sirve**, con la misma tabla de 20.004 obras:

| | Plan | Tiempo |
| --- | --- | --- |
| `%` + `set_config(..., true)` (la actual) | `Index Scan` sobre `obras_titulo_norm_gist` | 0,6 ms |
| KNN con desempate por `id` | `Seq Scan` + `top-N heapsort` | 217 ms |
| KNN sin desempate | `Index Scan`, pero empates a la suerte | 6,2 ms |

El desempate por `id` es lo que da orden total (ADR 0005) y con el el planificador no usa el
indice. Sin el, dos corridas podrian elegir obras distintas ante un empate en el limite del
`LIMIT`. Fijar el GUC una vez por corrida tampoco: es de sesion y el pool reparte conexiones. Se
conserva `%` en una transaccion, abierta con `enTransaccionDe`: dentro de una unidad de trabajo
corre en ella. Planes completos en [`explain-trgm.md`](explain-trgm.md).

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

Los dos cortes tienen que cumplir `0 < banda < umbral <= 1` (`Umbrales.Validar`, review de #146,
S4), y una corrida con parametros que no lo cumplan **no empieza** y nombra las dos claves. Con
`banda == umbral` la banda no tiene ancho: nada llega a la bandeja y lo que casi casa se va a ONI a
ciegas. Una banda en cero es "ausente" (ADR 0004), no un piso: con el corte en cero el motor
propondria cualquier obra. `cmd/metricas-matching` aplica la misma comprobacion, porque medir contra
umbrales que la cascada rechazaria es medir otra cosa.

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

### Convive con `SnapshotEnFecha` (#118, ya en main)

#118 aterrizo antes que este PR y trae `SnapshotEnFecha`, que resuelve **todas** las clausulas del
reparto contra una fecha y las **congela** con un id direccionado por contenido. `ParametroVigente`
no se absorbe en el, por tres razones:

- **No congela nada.** Identificar no mueve dinero (ADR 0003): no hay corrida de reparto que
  reproducir, y escribir un snapshot por cada resolucion de usos seria ruido en
  `snapshots_parametros`.
- **No exige las diecinueve clausulas.** `SnapshotEnFecha` falla si falta cualquiera de ellas, con
  razon, porque un reparto a medias es un error. Identificar solo necesita dos filas, y no tiene
  por que caerse porque falte, por ejemplo, `ott.wb`.
- **`matching.umbral_banda` no entra en el snapshot** y `matching.umbral` si. Este ultimo asigna
  obras y por eso el reparto lo congela; el piso de la banda no asigna nada, solo decide que se
  muestra en la bandeja (#39), y meterlo en el snapshot cambiaria el id de todos los repartos por
  un parametro que no los afecta. Los dos leen la **misma fila** de `matching.umbral` porque los
  dos resuelven contra la fecha del periodo y la `EXCLUDE` deja una sola respuesta.

Ambos comparan por **dia UTC** y no por instante: comparar un `timestamptz` con una `DATE` dejaria
que la zona de la sesion decidiera la vigencia. `ParametroVigente` pide su ejecutor (`ejecutorDe`)
como el resto del paquete, asi que participa en la unidad de trabajo del contexto.

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
| `manual` | **no**, y consta como `EscalonManual` | una decision humana no se pisa con una automatica |
| cualquier otro | **error** | fallar cerrado (D8): saltarlo en silencio dejaria una fila sin decidir sin que nada lo cuente |

Un escalon fuera de la tabla aborta la corrida con `uso "<id>": escalon desconocido "<x>"`. Hoy no
puede pasar -el `CHECK` de `usos` admite exactamente los siete de arriba-, y por eso mismo el dia
que el esquema admita uno nuevo, la cascada avisa en vez de ignorarlo.

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

**Una lista vacia tambien reemplaza.** Una fila que en una corrida quedo en la banda y en la
siguiente cae por debajo del piso se queda con la bandeja **vacia**, no con los candidatos viejos:
`ResolverUsos` llama al puerto aunque no haya candidatos, y el adaptador (que ya borra y luego
inserta) solo borra.

**Y es una sola unidad con el match.** La bandeja y el `GuardarMatch` de una ONI se escriben dentro
de `UnidadDeTrabajo` (review de #146, S3): si el match no se escribe porque la fila ya es de otro
-una resolucion manual concurrente, `GuardarMatch` responde `ErrNoEncontrado`-, la unidad recibe
`errFilaCambiada`, revierte la bandeja con el y la corrida sigue. Sin la unidad, la bandeja de la
decision perdida quedaba puesta sobre una fila que esa decision ya no considera ONI. El orden
dentro de la unidad sigue siendo el de D5.

**Y dice contra que titulo se puntuo.** `candidatos_match.titulo_consultado` guarda cual de los
titulos de la fila -el emitido o el original, ver D11- produjo cada puntaje. Sin esa columna la
bandeja muestra un `0.52` sin decir de donde sale, que es exactamente la pregunta de un auditor de
`RD 16`.

## D10. La busqueda del catalogo comparte el indice, no el umbral

`GET /obras?titulo=` casa por subcadena (`ILIKE`) **o** por parecido, y ordena por parecido. Eso
es lo que hace que el buscador tolere tildes, mayusculas y orden de palabras.

Su corte es `pg_trgm.similarity_threshold` en su valor de fabrica (0.3). Es una caja de busqueda:
lo que sale de ella no decide a quien se le paga. El corte que si decide es el del escalon 3, y
ese es un parametro con vigencia.

## D11. Titulo original: se consultan los dos y se unen por el mejor puntaje

Caracol entrega `Titulo` (el emitido en Colombia) y `Titulo_original`, y difieren en 16 de 59
filas. Los catalogos de las fuentes no comparten ni idioma: buscar solo por el emitido pierde
justo las filas donde el nombre local no se parece al del catalogo. Hasta el review de #146 el
original **ni siquiera se guardaba**: `usos` no tenia columna, la ingesta no lo mapeaba y
`Entrada.TituloOrig` viajaba siempre vacio.

- **Persistencia.** `usos.titulo_original` (migracion 00014, `NOT NULL DEFAULT ''`: vacio = la
  fuente no lo trae) y `Titulo_original` en `MapaCaracol`, **no requerida**: una entrega sin esa
  columna se lee igual y solo pierde ese recall. Netflix y cine no lo traen.
- **Se consultan los dos titulos**, cada uno con el piso de la banda, y se **unen por obra**
  (`identificacion.UnirCandidatos`): por obra gana el mejor puntaje, y se recorta a
  `MaxCandidatos`. El original se prueba solo si difiere del emitido sin distinguir mayusculas ni
  espacios: el mismo titulo dos veces es una consulta de mas en el bucle caro (D6).
- **Regla de empate.** Si una obra puntua igual con los dos titulos, gana la lista anterior: el
  emitido va primero, que es el que la cascada ya usaba, y asi un empate no cambia lo que se
  escribia antes de este cambio. El orden final es total: puntaje descendente y `ObraID`
  ascendente (ADR 0005), sin depender del orden en que se recorran mapas.
- **Evidencia.** `Candidato.TituloConsultado` viaja hasta la evidencia del match
  (`difuso "Without Breasts There Is No Paradise" ~ obra-45 (0.88000)`) y hasta la bandeja (D9): se
  sabe si la fila caso por el emitido o por el original. No entra en el orden ni en la decision.
- **Un fallo de la segunda consulta aborta la corrida** (D8) y el error nombra el titulo que
  fallo; una fila a medias no se escribe.

Los creditos (`Autor*`, `Guionista*`) siguen sin puntuar (D7): `TituloOrig` es un titulo, no un
credito.

## Ver tambien

- [`explain-trgm.md`](explain-trgm.md) — por que el indice es GiST y no GIN, medido.
- [`../28-cascada-identificacion/01-design.md`](../28-cascada-identificacion/01-design.md) — escalones 0-2.
