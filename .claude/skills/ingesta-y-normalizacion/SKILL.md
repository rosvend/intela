---
name: ingesta-y-normalizacion
description: Usar al cargar, perfilar o normalizar reportes de REDES SGC - parrillas de television, reportes OTT, archivos del cliente en CSV, Excel o JSON, el esquema canonico, la boveda de reportes crudos, deteccion de duplicados, log de rechazos, o al escribir cualquier adaptador de adquisicion o mapeo de columnas. Tambien al perfilar un archivo nuevo del cliente y al decidir que hacer con un registro que no se puede normalizar.
---

# Ingesta y normalizacion

## Lo primero, porque condiciona todo el modulo

**Ningun reporte de uso trae importes.** La bolsa se forma en `Recaudo` a partir de una factura al
usuario; los reportes solo la **ponderan** despues. Un normalizador que busque una columna de
dinero por obra esta resolviendo un problema que no existe, y si la encuentra es casi seguro que
la esta malinterpretando.

Ver `recaudo-y-tarifas` para de donde sale el dinero, y `reparto-y-distribucion` para como se
pondera.

## El esquema canonico

Cada fuente se lleva al mismo esquema antes de que nada aguas abajo la toque. Lo minimo que el
esquema tiene que capturar, segun lo que exigen las formulas de `RD 9`:

- Identidad de la fuente y del reporte (con su version en el almacen de objetos).
- Identificadores de origen tal como vienen, sin normalizar, para la tabla de alias.
- Titulos, en todas las variantes que traiga el archivo (localizado y original).
- La metrica de uso que corresponda a la modalidad: duracion y emisiones en TV, visualizaciones en
  OTT, taquilla en cine.
- Periodo y granularidad declarada del registro.

Lo que **no** entra al esquema canonico: nada que parezca un importe por obra, y nada que parezca
un porcentaje de autor. Los porcentajes salen solo de la Declaracion de Obra (`R-03`).

## Los reportes crudos se congelan

Los originales se guardan **inmutables y versionados** en el almacen de objetos con su huella
SHA-256, y cada corrida referencia **la version exacta que consumio**. Un reproceso posterior no
vuelve a leer "el archivo de Caracol": lee el archivo que se uso.

Sin esto, la reproducibilidad que exige `RD 16` a diez anos no se sostiene, porque el archivo de
origen pudo haberse reemplazado por una version corregida.
Fuente: `0005-reparto-determinista-y-reproducible.md`

## Los rechazos no se descartan en silencio

Un registro que no se puede normalizar queda en el **log de rechazos con su motivo**. Ni se pierde
ni se cuela. Si un archivo no cumple la estructura minima, el mensaje debe decir **que campos
faltan o estan mal formateados**, no fallar genericamente.

Es criterio de aceptacion explicito de OE-1 y de KR-1, y ademas es lo que permite volver a pedirle
al cliente exactamente lo que falta.

## Trampas medidas en los archivos reales

Perfilado reproducible con `uv run --script src/scripts/sample.py`. Detalle en
`docs/dominio/fuentes-datos.md`. Estas no son hipotesis: estan medidas sobre la muestra.

### Parrilla de television (CARACOL, 59 filas x 48 columnas)

- **Granularidad: la emision, no la obra.** 29 `ID_Ficha` distintos en 59 filas; algunos titulos se
  emiten hasta 4 veces. **Agrupar antes de valorizar**, o se cuentan emisiones como obras.
- `Fecha` es un **entero `YYYYMMDD`**, no una fecha. `Hora` es objeto de tiempo. `Año` es float.
- `Capitulo ID_IMDB` y `Capitulo Puntaje_IMDB` son **constantes en 0**: es relleno, no dato. No
  confundir "poblada" con "util".
- **18 de 48 columnas estan vacias al 100%**, incluidos los cuatro campos de episodio
  (`Titulo_capitulo`, `Temporada`, `ID_Ficha_Capitulo`, `Numero_Capitulo`) pese a que 18 filas son
  series. Hoy solo se puede identificar el programa, no el capitulo.
- Creditos muy incompletos: `Guionista1` 39%, `Autor1` 34%. **36 de 59 filas no tienen ni autor ni
  guionista**, y eso no bloquea nada: por `R-03` los autores salen de la Declaracion de Obra.
- `Titulo` difiere de `Titulo_original` en 16 filas.

### Reporte OTT (Netflix, 49 filas x 19 columnas)

- **Granularidad: el episodio.** `netflix_id` unico en las 49 filas.
- **`eidr` vacia en 49 de 49.** Era el identificador que habria resuelto el cruce entre fuentes.
- `episode_nbr` es de **tipo mixto** y trae un placeholder `--`.
- `term_end_date`, `year` y `viewing_country` son **constantes**: son metadatos del export, no
  datos del registro. No modelarlos como campos por fila.
- `stream_starts` es la metrica de uso y alimenta `V` de `RD 9.7`.

### El identificador que no lo es

**`Id_Ntx` es un contador de fila del export**, de 1 a 49. Se renumera en cada entrega.
**No persistirlo como clave** bajo ninguna circunstancia. Ver `matching-de-obras`.

## Duplicados

Detectar en dos niveles, porque son fallos distintos:

- **Por huella del archivo** — la misma entrega cargada dos veces. El SHA-256 de la boveda cruda lo
  resuelve solo.
- **Por registro** — dos filas del mismo reporte, fuente y periodo que representan el mismo hecho.

Ambos son criterio de aceptacion de OE-5 y del objetivo 9.

## Aviso de tamano

Son **59 y 49 filas**. Sirven para disenar esquemas y mapeos. **No sirven para ajustar un motor de
matching ni para medir tasas de acierto.** Cualquier metrica calculada sobre esta muestra es
falsa; decirlo explicitamente al reportar resultados.

## Lo que falta pedir al cliente

Ordenado por impacto. Detalle y contexto en `docs/dominio/fuentes-datos.md`.

1. Export de **Declaraciones de Obra** desde REDES-SYS — sin autores y porcentajes no hay reparto
   posible (`R-03`, `R-04`). Es el dato mas critico y no esta en ninguna muestra.
2. **Reportes de recaudo** por usuario y periodo — la bolsa. Ningun archivo actual trae importes.
3. **Feed de rating** por franja horaria — bloquea `RD 9.1.1` completo.
4. Coeficientes **`Wa`, `Wb`, `Wc`** — bloquean el calculo OTT.
5. **Tabla de mapeo** de generos y subgeneros a las cuatro categorias de tipo de obra, mas el
   criterio de repertorio por programa.
6. **`eidr` poblado**, o acceso a IDA.
7. **Campos de episodio** en la parrilla, para identificar capitulos de series.
8. **Extractos del mismo periodo** en ambas fuentes, y de mayor volumen.

## Los scripts de perfilado

Viven en `src/scripts/`, declaran dependencias en linea con PEP 723 y se ejecutan con
`uv run --script <archivo>`. Cada uno resuelve su propio entorno efimero para no interferir con el
stack de la aplicacion. Al perfilar un archivo nuevo del cliente, extender `sample.py` en vez de
escribir un script paralelo: `normalize_text` de ahi es la normalizacion de referencia que usa
tambien el matching.
