# 0014 El log de rechazos de la ingesta vive en una tabla aparte

Fecha: 2026-08-31
Estado: Vigente

## Contexto

La skill `ingesta-y-normalizacion` fija la regla en una linea: un registro que no se puede
normalizar **queda en el log de rechazos con su motivo**; ni se pierde ni se cuela. Es criterio de
aceptacion explicito de `OE-1` y de `KR-1`, y ademas es lo que permite volver a pedirle al cliente
exactamente lo que falta en vez de decirle que "el archivo fallo".

Los archivos reales lo hacen inevitable, no hipotetico. En la parrilla de CARACOL, 18 de 48 columnas
estan vacias al 100%; en el reporte de Netflix, `episode_nbr` es de tipo mixto y trae un placeholder
`--`. Detener una entrega de 59 filas porque una no se puede mapear haria la ingesta inservible.

El esquema de `00001_init.sql` no tenia donde poner eso: `usos` es la forma canonica y no tiene
columna de motivo, y no existia ninguna otra tabla. Habia que elegir donde cae una fila
estructuralmente rota.

Hay una restriccion mas, que es la que decide: `usos` no es una tabla de datos, es una tabla con
invariantes. `modalidad` esta acotada a las cuatro de `RD 8`, `escalon` a los seis de la cascada,
`uso_resuelto_tiene_obra` exige coherencia entre `oni` y `obra_id`, `manual_tiene_autor` exige autor
e instante, y cada columna de medida lleva su `CHECK (>= 0)`. Una fila estructuralmente rota, por
definicion, viola alguno de esos `CHECK`.

## Decision

**Las filas rechazadas van a una tabla propia, `usos_rechazados`, que nunca se mezcla con `usos`.**

Cuatro partes:

1. **Tabla aparte, no una columna `rechazo_motivo` en `usos`.** Lo que sigue.
2. **Comparte el espacio de identificadores con `usos`.** Un id vive en una de las dos tablas y
   nunca en las dos, asi que una linea de un archivo se rastrea sin saber de antemano si llego a ser
   canonica.
3. **No copia ninguna columna de medida.** Guarda lo identificatorio —fuente, titulo, ids de origen,
   modalidad tal como vino— y el motivo. Sin `duracion_min`, `emisiones` ni `vistas` no hay forma de
   que una consulta futura sume una fila rechazada "solo para ver".
4. **El motivo es obligatorio y no vacio**, con `CHECK (btrim(motivo) <> '')`. Mismo patron que
   `resultados_obra.retenida_tiene_motivo` y misma razon: en este sistema lo que se aparta se aparta
   **con** su razon.

El reparto de responsabilidades queda asi: **`aplicacion` decide QUE fila es invalida y por que**
(`validarUso` en `internal/aplicacion/ingesta.go`, que estampa `UsoPersistido.RechazoMotivo`), y
**el adaptador decide DONDE acaba cada clase de fila** (`Store.GuardarUsos`). Es la misma division
que en `0013`: alli el caso de uso maneja tokens en claro y el adaptador decide guardar un resumen,
porque "no guardar el secreto en claro" es una propiedad del almacenamiento. En que tabla vive algo
tambien lo es.

Por eso el puerto no cambia de forma. `GuardarUsos(ctx, []UsoPersistido)` recibe el lote **entero**,
valido y rechazado mezclados, y lo escribe en una sola transaccion: las dos escrituras son el mismo
hecho, y un lote guardado a medias dejaria una entrega cuyo recuento no cuadra con el archivo sin
que nadie sepa cual mitad falta.

## Alternativas consideradas

**Una columna `rechazo_motivo TEXT` nullable en `usos`,** filtrando las lecturas canonicas con
`WHERE rechazo_motivo IS NULL`. Es la opcion mas corta y es la equivocada, por tres razones
independientes:

- **Obliga a relajar los `CHECK` de `usos`.** El ejemplo que da el propio issue —modalidad fuera de
  `tv|cine|ott|hotel`— literalmente no cabe en la tabla sin quitar ese `CHECK`. Y esos `CHECK` son
  lo unico que garantiza que lo que hay en `usos` es canonico: relajarlos para que quepa la basura
  deja entrar tambien la basura que nadie marco.
- **Contamina el listado publico de ONI.** La vista `oni_publico` selecciona `WHERE u.oni`, y `oni`
  tiene `DEFAULT TRUE`. Una fila rechazada guardada en `usos` aparece en el listado publico de obras
  no identificadas (`R-18`, `RD 13.8.1`) sin que nadie lo pida y sin que nada avise.
- **Convierte la exclusion en una convencion.** Con la columna, que un rechazo no pondere depende de
  que **toda** consulta futura se acuerde de anadir el filtro. Con la tabla aparte, la fila no esta:
  la exclusion es estructural. Es el mismo argumento que `00001_init.sql` usa para poner el
  append-only de la bitacora en un trigger y no en la documentacion —"la disciplina no puede
  depender de que nadie escriba la sentencia"—, aplicado al reves: aqui la disciplina no puede
  depender de que todos escriban el `WHERE`.

**Descartar la fila y devolver el motivo solo en la respuesta HTTP.** Descartada de plano: es
exactamente lo que la skill prohibe. El motivo desaparece con la peticion, y a la semana siguiente
nadie puede decir que filas del archivo de enero no entraron.

**Guardar el payload crudo de la fila en una columna `JSONB`.** Tentador, y aplazado. Hoy los
adaptadores de formato no existen (`#25`), asi que no hay una forma acordada de "fila cruda" que
guardar; inventarla ahora seria fijar en el esquema un contrato que todavia no se ha escrito.
Cuando exista, se anade como columna nueva a esta tabla, que es un cambio aditivo.

**Un `CHECK` o un trigger que impida que un id este en las dos tablas.** Descartado por ahora:
requiere una comprobacion entre tablas —o sea un trigger— y el unico camino de escritura es
`GuardarUsos`, que encamina cada fila una sola vez. Se anadiria el dia que haya un segundo camino.

## Consecuencias

Una consulta que quiera ver "todo lo que trajo el reporte X" tiene que mirar las dos tablas. Es el
precio, y es barato: las dos cuelgan de `reporte_id` con `ON DELETE CASCADE`, asi que un `UNION` con
el mismo filtro las junta, y borrar un reporte se lleva las dos por delante.

A cambio, ninguna consulta escrita a partir de ahora puede sumar por accidente una fila que no se
pudo normalizar, ni exponerla en el listado publico, ni contarla como un uso pendiente de
identificar. Y `usos` conserva intactos todos sus `CHECK`, que es lo que hace que la palabra
"canonico" signifique algo.

Queda una duplicacion consciente: `validarUso` reimplementa en Go las restricciones de `usos`. No es
un descuido, es el objetivo —una sola fila que viole un `CHECK` aborta el `INSERT` del lote entero y
se lleva por delante las filas buenas que la acompanan—, pero **son dos sitios que hay que cambiar
a la vez**. Si un dia se anade una modalidad a `RD 8`, hay que tocar la migracion, la constante del
dominio y `validarUso`. Lo que protege de que se olvide es
`TestGuardarUsosEsAtomico`: comprueba que una fila que la validacion no vio revienta el lote entero
en vez de colarse.
