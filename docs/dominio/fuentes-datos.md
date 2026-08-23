---
actualizado: 2026-08-10
evidencia: uv run --script src/scripts/sample.py
---

# Fuentes de datos

Perfilado de la muestra que entrego el cliente, en `data/files/`. Reproducible con
`uv run --script src/scripts/sample.py`.

Aviso de tamano: son 59 y 49 filas. Sirven para disenar esquemas y mapeos. **No sirven para
ajustar un motor de matching ni para medir tasas de acierto.**

## Parrilla de television: CARACOL_REDES-SGC_(COLOMBIA)_20250202.xlsx

Una hoja, **59 filas x 48 columnas**. Log de emision del canal CARACOL.

- **Granularidad: la emision, no la obra.** 29 `ID_Ficha` distintos en 59 filas. Algunos
  titulos se emiten hasta 4 veces en la ventana. `ID_Ficha` es la clave de obra.
- **Cobertura: 4 dias.** `Fecha` de 20241231 a 20250103.
- `TIPO`: PR 33, SE 18, PE 8.
- Sin filas duplicadas completas.

### Columnas vacias al 100% (18 de 48)

`Duracion_original`, `Director3`, `Director4`, `Conductor1` a `Conductor4`, `Voz1` a `Voz4`,
`Autor4`, `Guionista3`, `Guionista4`, `Titulo_capitulo`, `Temporada`, `ID_Ficha_Capitulo`,
`Numero_Capitulo`.

Los cuatro campos de episodio estan vacios pese a que 18 filas son series. **Sin ellos no se
puede identificar el capitulo emitido**, solo el programa.

### Datos de creditos, muy incompletos

| Columna | Poblada |
| ------- | ------- |
| `Director1` | 55,9% |
| `Guionista1` | 39,0% |
| `Autor1` | 33,9% |
| `Autor2` | 10,2% |
| `Guionista2` | 8,5% |
| `Autor3` | 5,1% |

**36 de 59 filas no tienen ni autor ni guionista.** Esto no bloquea el proyecto: por `R-03`
los autores y porcentajes salen de la Declaracion de Obra, no de la parrilla. Estas columnas
son features de matching (`R-02`).

### Trampas de tipo y valor

- `Fecha` es entero `YYYYMMDD`, no fecha.
- `Hora` es objeto de tiempo.
- `Año` es float y trae 98,3% de cobertura.
- `Capitulo ID_IMDB` y `Capitulo Puntaje_IMDB` son **constantes en 0**: relleno, no dato.
- `Programa ID_IMDB` poblada 54,2%, unico identificador externo utilizable.
- `Titulo` difiere de `Titulo_original` en 16 filas.

### Aporte a las formulas

Cubre `Duracion` y permite contar emisiones para `RD 9.1.1`. **No trae rating.** El mapeo de
`TIPO` y `SubGenero` a las cuatro categorias del reglamento no es 1:1 y falta decidirlo.

Valores de `SubGenero` observados incluyen Telenovela, Magazine, Noticiero, Agro,
Entretenimientos, Religioso y Drama. Varios de ellos casi con seguridad **no son repertorio
de REDES SGC**: un noticiero no tiene guionista en el sentido de `RD 7.1`. Hace falta el
filtro de repertorio de `R-27` a nivel de programa, no solo de canal.

## Reporte OTT: Modulo identificación de Obras - Parrilla Netflix.xlsx

Hoja `NETFLIX_REDES_2018`, **49 filas x 19 columnas**.

- **Granularidad: el episodio.** `netflix_id` unico en las 49 filas. 46 shows, 48 temporadas.
- **Cobertura: una foto de 2018.** `term_end_date` 2018-12-31, `viewing_country` solo CO.
- Estructuralmente limpio: sin nulos salvo `eidr`.

### Hallazgos

- **`eidr` vacia en 49 de 49.** Es el identificador que habria resuelto el cruce entre
  fuentes. Ver `identificadores.md`.
- `stream_starts` es la metrica de uso, alimenta `V` de `RD 9.7`.
- `distributor` con 36 valores distintos. Solo 2 filas son `CARACOL`.
- `episode_nbr` es de tipo mixto y trae un placeholder `--`.
- `term_end_date`, `year` y `viewing_country` son constantes: metadatos del export.

### Aporte a las formulas

Cubre `V`. **No cubre `DU`**: `episode_runtime` es la duracion del episodio, no el tiempo
efectivamente visto, que es lo que pide `RD 9.7`. Multiplicar runtime por starts da una cota
superior, no la magnitud pedida.

## Padron IPI: data/IPI - form to report members to IPI 01-03-24.xls

Fuera de `data/files/` y por tanto **fuera del alcance de `sample.py`**. Formato de reporte de
miembros al sistema IPI de SUISA, citado en `identificadores.md` como la explicacion de por que IPI
es el identificador de los autores. Sin perfilar: se desconoce su cobertura, si trae los IPI ya
asignados o si es una plantilla en blanco.

Accion: extender `sample.py` para que lo recorra, y solo entonces decidir que pedir al cliente.

## Cruce entre las dos fuentes

Nombres de columna compartidos: **ninguno**.
Coincidencias exactas de titulo normalizado: **0**.
Candidatos difusos sobre 0.6 de similitud: **0**.

No es una senal de alarma: por diseno las fuentes se cruzan **contra el catalogo maestro**, no
entre si. Ver `identificadores.md`.

## Lo que falta pedir al cliente

Ordenado por impacto sobre el alcance.

1. **Export de Declaraciones de Obra desde REDES-SYS.** Sin autores y porcentajes no hay
   reparto posible (`R-03`, `R-04`). Es el dato mas critico y no esta en ninguna muestra.
2. **Reportes de recaudo por usuario y periodo.** La bolsa a repartir. Ningun archivo actual
   contiene importes.
3. **Feed de rating por franja horaria** del proveedor especializado. Bloquea `RD 9.1.1`
   completo.
4. **Coeficientes `Wa`, `Wb`, `Wc`** de `RD 9.7`. Bloquean el calculo OTT.
5. **Tabla de mapeo** entre generos y subgeneros de parrilla y las cuatro categorias de tipo
   de obra, mas el criterio de repertorio por programa.
6. **`eidr` poblado** por parte de Netflix, o acceso a IDA.
7. **Campos de episodio** en la parrilla de Caracol, para poder identificar capitulos de
   series.
8. **Extractos del mismo periodo** en ambas fuentes, y de mayor volumen.
9. Padron de socios y titulares administrados con numero **IPI**, poblado y al dia.
   Matiz: `data/IPI - form to report members to IPI 01-03-24.xls` ya esta en el repositorio — es
   el formato con el que se reportan miembros al sistema IPI de SUISA. **No esta perfilado**:
   `src/scripts/sample.py` solo recorre `data/files/`. Antes de pedirlo al cliente hay que
   perfilarlo y determinar que le falta.
