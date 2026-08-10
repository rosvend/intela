---
actualizado: 2026-08-10
evidencia: src/scripts/sample.py sobre data/files
---

# Identificadores y resolucion de obras

## El error a evitar

Los IDs que traen los reportes de uso **no sirven para cruzar una fuente contra otra**. Son
claves locales del sistema que las emitio. Verificado sobre la muestra:

| Columna | Fuente | Valores | Digitos | Rango | Interseccion con `ID_Ficha` |
| ------- | ------ | ------- | ------- | ----- | --------------------------- |
| `ID_Ficha` | Caracol | 29 | 5-6 | 50.332 - 871.732 | — |
| `show_id` | Netflix | 46 | 8 | 60.035.992 - 81.003.997 | **0** |
| `series_id` | Netflix | 48 | 8 | 60.035.992 - 81.004.793 | **0** |
| `netflix_id` | Netflix | 49 | 8 | 60.035.992 - 81.012.675 | **0** |
| `Id_Ntx` | Netflix | 49 | 1-2 | 1 - 49 | **0** |

`ID_Ficha` pertenece al catalogo del proveedor especializado de parrillas. Los de ocho
digitos pertenecen al catalogo interno de Netflix. Son autoridades emisoras distintas: una
coincidencia numerica seria casualidad, no un match.

`Id_Ntx` es un contador de fila del export, de 1 a 49. No es un identificador y se renumera
en cada entrega. **No persistirlo como clave.**

Ademas estan en granularidades distintas:

| Columna | Granularidad |
| ------- | ------------ |
| `ID_Ficha` | Programa u obra |
| `show_id` | Show |
| `series_id` | Temporada |
| `netflix_id` | Episodio |

## El modelo correcto: cubo y radios

No hay que cruzar Caracol contra Netflix. Hay que cruzar **cada fuente contra el catalogo
maestro de obras declaradas** en REDES-SYS, que es lo unico que tiene los autores y sus
porcentajes (`R-03`).

```
parrilla Caracol  ─┐
                    ├─→  obra declarada (REDES-SYS)  ─→  autores (IPI) + % declarado
reporte Netflix   ─┘
```

Son dos problemas de matching independientes contra un mismo cubo. Las fuentes nunca
necesitan coincidir entre si.

## Los IDs de fuente si sirven, como alias

Una vez resuelto `show_id 80141259` a la obra REDES numero X, esa correspondencia se guarda y
todo reporte futuro de Netflix con ese `show_id` hace match por consulta exacta. El matching
difuso solo corre para IDs nunca vistos. El costo baja con el tiempo y queda trazabilidad,
que es lo que exigen `RD 13` y `RD 16`.

Forma minima de la tabla de alias:

```
alias_obra(
  fuente,            -- 'caracol' | 'netflix' | ...
  tipo_id,           -- 'ID_Ficha' | 'show_id' | 'series_id' | 'netflix_id'
  valor_id,
  obra_id,           -- FK al catalogo maestro
  confianza,
  resuelto_por,      -- usuario o proceso automatico
  resuelto_en
)
```

## Identificadores globales que si cruzarian

El reglamento nombra dos, y los datos sugieren dos mas del mercado:

**IPI** — identifica **personas** (autores). Administrado por SUISA dentro de CISAC. Es el
identificador correcto para los titulares. Explica el archivo
`data/IPI - form to report members to IPI 01-03-24.xls`. Fuente: `RD 3`

**IDA** — identifica **obras audiovisuales** y sus titulares. Base centralizada de CISAC.
REDES SGC es miembro pleno desde 2020, asi que deberia tener acceso. `RD 14.5.6` confirma que
la documentacion de obras se extrae de IDA **o** de lo declarado por los autores. Es la clave
real entre sociedades. Fuente: `RD 3`, `RD 14.5.6`

**EIDR** — estandar de la industria audiovisual. El archivo de Netflix trae la columna y esta
**vacia en 49 de 49 filas**.

**IMDB** — el archivo de Caracol trae `Programa ID_IMDB`, poblada en **32 de 59 filas**.

### La asimetria que bloquea hoy

| | EIDR | IMDB |
| --- | --- | --- |
| Caracol | no existe la columna | 32/59 poblada |
| Netflix | columna vacia 0/49 | no existe la columna |

**No hay hoy ningun identificador global poblado en comun entre las dos fuentes.** Es la
peticion de mayor valor para la reunion con el cliente: que Netflix entregue `eidr` poblado,
o que se trabaje contra IDA.

## Estrategia de matching sugerida

En orden de precedencia, cayendo al siguiente solo si el anterior no resuelve:

1. **Alias conocido** — consulta exacta en `alias_obra`. Costo cero, confianza maxima.
2. **Identificador global** — IDA, EIDR o IMDB cuando esten poblados.
3. **Matching difuso sobre titulo** — con las salvedades de abajo.
4. **Cola de resolucion manual** — con la evidencia adjunta para que un humano decida. El
   reglamento ya prevee flujo manual y auditoria.

### Lo que el matching difuso enfrenta aqui

Medido sobre la muestra: 36 titulos normalizados en Caracol, 141 en Netflix,
**0 coincidencias exactas y 0 candidatos por encima de 0.6 de similitud**. Causas:

- Sin solape temporal. Netflix es 2018, Caracol es 2024-12-31 a 2025-01-03.
- Catalogos distintos. Solo 2 filas de Netflix tienen `distributor = CARACOL`.
- Idioma. Netflix usa titulos en ingles, Caracol en espanol.
- Titulo localizado contra titulo original. En Caracol `Titulo` difiere de `Titulo_original`
  en 16 de 59 filas, por ejemplo *El destino de Melek* frente a *Benim Adim Melek*.

Senales utiles como features de matching, nunca como insumo de pago (`R-02`):
`Titulo`, `Titulo_original`, `Año`, `NacionalidadOrigen`, `Genero`, `Duracion_total`,
`Actores*`, `Director*`, `Autor*`, `Guionista*`, y del lado Netflix `show_name`,
`series_name`, `episode_name`, `release_year`, `country_of_origin`, `distributor`.
