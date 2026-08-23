---
name: matching-de-obras
description: Usar al trabajar en identificacion y resolucion de obras - cruzar reportes de uso (parrillas de TV, reportes OTT) contra el catalogo maestro, manejar IDs de fuente, alias, matching difuso de titulos, normalizacion, umbrales de similitud, cola de resolucion manual u obras no identificadas (ONI). Tambien al elegir o calibrar un algoritmo de similitud, al decidir que hacer con una coincidencia dudosa, y al filtrar repertorio antes de matchear.
---

# Matching e identificacion de obras

## Arquitectura: cubo y radios

No se cruza una fuente contra otra. Cada fuente se cruza contra el **catalogo maestro de obras
declaradas**, que es lo unico que tiene autores y porcentajes.

```
parrilla Caracol  ─┐
                    ├─→  obra declarada (REDES-SYS)  ─→  autores (IPI) + % declarado
reporte Netflix   ─┘
```

Dos problemas independientes contra un mismo cubo. Que Netflix y Caracol no coincidan entre si
es esperado, no un defecto.

## Los IDs de fuente no cruzan

Verificado sobre la muestra: `ID_Ficha` (5-6 digitos, catalogo del proveedor de parrillas) y
`show_id`/`series_id`/`netflix_id` (8 digitos, catalogo de Netflix) tienen **interseccion
cero**. Son claves locales de autoridades emisoras distintas.

`Id_Ntx` es un contador de fila del export (1 a 49). No persistirlo como clave.

Detalle y evidencia en `docs/dominio/identificadores.md`.

## Los IDs de fuente si sirven como alias

Resolver una vez, reutilizar siempre. Tabla `alias_obra(fuente, tipo_id, valor_id, obra_id,
confianza, resuelto_por, resuelto_en)`. El difuso solo corre para IDs nunca vistos, y queda
trazabilidad para auditoria.

## Orden de precedencia

1. Alias conocido, consulta exacta
2. Identificador global: IDA, EIDR o IMDB cuando esten poblados
3. Matching difuso sobre titulo y features
4. Cola de resolucion manual con la evidencia adjunta

## Cuidado con la granularidad

| Columna | Granularidad |
| ------- | ------------ |
| `ID_Ficha` | Programa u obra |
| `show_id` | Show |
| `series_id` | Temporada |
| `netflix_id` | Episodio |

Hay que decidir explicitamente que nivel corresponde a una obra de REDES. La parrilla de
Caracol trae los cuatro campos de episodio **vacios al 100%**, asi que hoy solo identifica el
programa, no el capitulo.

Ademas la parrilla esta a nivel de **emision**: 29 obras en 59 filas. Agrupar antes de
valorizar.

## Features utiles, nunca insumo de pago

`Titulo`, `Titulo_original`, `Año`, `NacionalidadOrigen`, `Genero`, `Duracion_total`,
`Actores*`, `Director*`, `Autor*`, `Guionista*`, `show_name`, `series_name`, `episode_name`,
`release_year`, `country_of_origin`, `distributor`.

Los nombres de personas de estos campos identifican la obra. **No determinan a quien se le
paga**: eso solo sale de la Declaracion de Obra.

## Lo que enfrenta el difuso

Sobre la muestra: 0 coincidencias exactas y 0 candidatos sobre 0.6 de similitud. Idiomas
distintos, catalogos distintos, sin solape temporal, y titulo localizado contra titulo
original que difieren en 16 de 59 filas de Caracol.

Normalizacion minima antes de comparar: NFKD, quitar diacriticos, minusculas, colapsar todo lo
no alfanumerico a espacio. Implementada en `normalize_text` de `src/scripts/sample.py`.

Comparar `Titulo` **y** `Titulo_original` contra `show_name`, `series_name` y `episode_name`.
Limitarse a un solo par de columnas pierde matches.

## Filtro de repertorio antes de matchear

`R-27` / `RD 9.5` excluye canales sin contenido del catalogo. Hace falta el equivalente por
programa: noticieros y magazines de la parrilla casi con seguridad no son repertorio de REDES,
porque no tienen guionista en el sentido de `RD 7.1`. Sin ese filtro se generan ONI falsas y
se diluye el valor punto.

## El umbral es un parametro, y arranca conservador a proposito

El umbral del escalon difuso es un **parametro normativo con vigencia y responsable**, igual que
el resto (`0004-parametros-normativos-como-dato.md`). No es una constante que se ajusta en el
codigo hasta que "se ve bien".

Arranca **deliberadamente conservador**: con la muestra actual no hay forma de calibrarlo, asi que
el sesgo por defecto es **mandar a la cola manual antes que decidir mal**. Un falso positivo paga a
quien no corresponde, y `R-05` deja a REDES SGC fuera de las disputas entre coautores: el error no
se corrige solo.

Riesgo a vigilar: la tentacion de bajar el umbral para vaciar la cola. Reduce trabajo visible hoy y
produce pagos incorrectos que aparecen como reclamaciones meses despues, cuando el dinero ya salio.
Por eso cada resolucion automatica guarda **su puntaje**, y la tasa de reclamaciones por obra mal
asignada se vigila contra el umbral que estaba vigente cuando se decidio.

## El modulo no toca dinero

`Identificacion` escribe alias y emite ONI. **No escribe titulares, no escribe porcentajes y no
toca importes.** Los campos `Autor*`, `Guionista*` y `Director*` entran tipados como *evidencia de
identificacion*; no existe camino desde una parrilla hasta un porcentaje de pago (`R-03`, `R-02`,
`RD 7.3.3`).

Cuando el volumen real llegue, un modelo de resolucion de entidades entra exactamente en el
escalon 3, detras de `PuertoMotorDeSimilitud`, sin tocar el resto de la cascada. Hoy no hay con que
entrenarlo: 59 y 49 filas sin solape temporal ni de catalogo no son un conjunto de entrenamiento, y
una metrica calculada sobre eso seria falsa.
Fuente: `0007-identificacion-en-cascada-con-cola-manual.md`

## Obras no identificadas

Lo que no resuelva termina en ONI, con reglas propias: listado publico sin montos,
prescripcion a 3 anos, reserva del dinero. Ver `R-18` a `R-21`.

**Lo no identificado es un estado, no un fallo.** `RD 13.8` prevee explicitamente que haya obras no
identificadas y define su tratamiento: son parte del funcionamiento normal. Bloquear el reparto
hasta que todo este identificado contradice el reglamento.
