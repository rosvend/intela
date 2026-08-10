---
name: matching-de-obras
description: Usar al trabajar en identificacion y resolucion de obras - cruzar reportes de uso (parrillas de TV, reportes OTT) contra el catalogo maestro, manejar IDs de fuente, alias, matching difuso de titulos, normalizacion, cola de resolucion manual u obras no identificadas (ONI).
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

## Obras no identificadas

Lo que no resuelva termina en ONI, con reglas propias: listado publico sin montos,
prescripcion a 3 anos, reserva del dinero. Ver `R-18` a `R-21`.
