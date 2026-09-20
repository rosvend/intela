---
actualizado: 2026-09-20
evidencia: internal/infraestructura/semilla/dataset.go
---

# Especificacion de las fixtures sinteticas

Que forma tiene cada dato que el cliente todavia no entrego, y como se distingue de un dato real.
Cierra la issue #21.

**Los valores no se copian aqui.** Viven en `internal/infraestructura/semilla/dataset.go`, que es
una funcion pura y la unica fuente de verdad. Este documento especifica **forma, rangos y
procedencia**; si un numero cambia, cambia ahi y no en dos sitios. Es la misma convencion que el
repo aplica a las reglas de `depguard`.

Las respuestas de negocio que sostienen estos valores estan en
[`preguntas-cliente.md`](preguntas-cliente.md), y **todas son provisionales**: las dio el equipo,
no REDES SGC.

## Como se marca lo sintetico

No es una etiqueta cosmetica. Un parametro normativo lleva dos columnas que van juntas:

| Columna | Dato real | Dato sintetico |
| ------- | --------- | -------------- |
| `organo` | El organo que lo aprobo, p. ej. `Consejo Directivo` | `sintetico` |
| `reglamento` | El numeral, p. ej. `RD 9.1.1` | `RD-IX-seed-sintetico` |

Asi, "que parametros tienen organo de verdad" es una consulta:
`WHERE reglamento <> 'RD-IX-seed-sintetico'`. `dataset_test.go` fija el conjunto exacto de claves
sinteticas esperadas, de modo que **promover un valor a real, u olvidar marcar uno nuevo, pone el
test en rojo**. Ese test es el que impide que una cifra inventada se cuele a produccion
disfrazada de norma.

Por que importa: `RD 16` somete el reparto a auditoria en cualquier tiempo. Un importe calculado
con una deduccion del 20% se defenderia citando un acta de Asamblea que nadie expidio. La etiqueta
es lo que permite decir "esta cifra es de demo" sin tener que leer el codigo.

## Estado por insumo

| Insumo | Fixture | Marcada | Falta |
| ------ | ------- | ------- | ----- |
| Declaraciones de Obra | Si | n/a | -- |
| Recaudo / bolsa | Si | n/a | El formato real del reporte (P-08) |
| Usuarios de recaudo | Si | n/a | La categoria real de cada pagador |
| Coeficientes OTT `Wa/Wb/Wc` | Si | Si | El valor real (P-04) |
| Rating por franja | **Parcial** | **No** | La tabla y la marca |
| Mapeo de generos | **No existe** | -- | Todo |
| Registro de canales | Si | n/a | El quintil real de `RD 9.5.4` (P-17) |
| Atribucion de canal en el uso | Si | n/a | -- |

## Declaraciones de Obra

Fixture: `Dataset.obrasYDeclaraciones()`. Periodo de referencia `2025-01`.

Forma: por obra, una lista de partes `(titular_id, ipi, porcentaje)`. Las cuatro declaraciones
cubren deliberadamente los casos que el motor tiene que distinguir:

- **dos coautores que suman 100** -- el caso corriente,
- **tres coautores que suman 100** -- que el reparto entre partes no sea un caso de dos,
- **un solo autor con 100** -- para ejercitar la ponderacion de sketches,
- **una parte sola que suma 60** -- el caso de `R-04`, en que se retiene el **total**.

El cuarto es el mas instructivo y conviene no simplificarlo al tocarlo: la obra tiene **dos
coautores en el catalogo y una sola parte declarada**. Eso es lo que hace explicable el 60%: la
obra la escribieron dos y el 40% del segundo no esta declarado. El arreglo no es repartir el 60%,
es que el coautor declare.

No hay ninguna obra **sin** declaracion en el dataset. Cuando #33 pruebe esa rama de `R-04`
-- obra sin declaracion, que tambien retiene -- hara falta anadirla.

No son sinteticas en el sentido normativo: no son parametros, son datos de negocio de ejemplo. No
llevan marca porque no hay riesgo de confundirlas con una norma.

Procedencia real (P-07): el autor declara en REDES-SYS y **Intela no se integra**. El documento
fuente es una declaracion jurada por autor, y `Otros autores` es **texto libre**
(`Juan Perez (30%), Pedro Lopez (10%)`), sin IPI y sin identificador de obra -- ver
[`fichas-fuente.md`](fichas-fuente.md#f-04-declaraciones-de-obra-redes-sys).

Cuando llegue un export real, la fixture tendra que crecer con **al menos una obra cuyas partes
vengan de esa cadena de texto**, para que el parseo no sea una sorpresa aguas abajo.

Nota (P-11): el IPI **sale del padron por `titular_id`**, no de la declaracion. El formulario de
REDES-SYS no tiene campo de IPI.

## Recaudo / bolsa

Fixtures: `Dataset.usuariosDeRecaudo()` y `Dataset.bolsas()`.

Forma de la bolsa: `(id, usuario_id, periodo, circuito, bruto, convenio, tarifa, factura)`.
Una bolsa por pagador del periodo `2025-01`, casi todas `nacional` y una `internacional` -- los
dos circuitos importan porque el internacional **no se valoriza por puntos** (`RD 7.4`), se
reparte tal como lo discrimino la sociedad hermana.

**Dos de las bolsas son de canales de TV abierta del mismo periodo**, y eso no es relleno: es la
unica forma de comprobar que el valor punto de `RD 9.1.1` se calcula por canal y no por periodo.
Una corrida es una bolsa (ADR 0019), asi que dos canales son dos corridas y dos valor punto
independientes.

Rangos: importes redondos del orden de $200.000 a $1.000.000 COP. Son redondos a proposito.

Alcance (P-08): Intela **recibe** el importe cobrado; no lo calcula ni factura. La bolsa es un
dato de entrada, no un resultado. Por eso no hay fixture de tarifa ni de convenio como
entidades: `convenio`, `tarifa` y `factura` son **procedencia** de la bolsa -- la pregunta 1 del
ADR 0006, de donde salio este dinero -- y van en la propia fila, marcadas `-sintetica`.

### Los pagadores

Desde la migracion 00009 (#27), `bolsas.usuario_id` tiene clave foranea a `usuarios_recaudo`, y
el dataset da de alta los pagadores que sus bolsas citan:

| `id` | Categoria | Por que esa |
| ---- | --------- | ----------- |
| `caracol` | `tv_abierta` | `RT 3.1.1`; se reparte por puntos (`RD 9.1.1`) |
| `rcn` | `tv_abierta` | El segundo canal, que es lo que hace observable `RD 9.1` |
| `procinal` | `cine` | `RT 3.2`; proporcional a taquilla (`RD 9.2`) |
| `netflix` | `medios_digitales` | `RT 3.6`; formula OTT (`RD 9.7`) |
| `expreso-bolivariano` | `transporte_terrestre` | `RT 3.4.1`; por exhibiciones (`RD 9.4`) |
| `dago-films` | `sin_clasificar` | Es el del circuito internacional |

La categoria **no** sirve para tarifar -- Intela no factura -- sino para saber que formula de
`formulas.md` aplica aguas abajo. Por eso cada uno lleva la suya y no una generica.

`dago-films` va `sin_clasificar` a proposito: el recaudo internacional no lo paga un usuario
colombiano de una categoria del `RT`, llega ya discriminado por una sociedad hermana. Ponerle
una categoria seria afirmar algo que el reglamento no dice (ADR 0004).

Los nombres llevan `(sintetico)` y los NIT van **vacios**: un NIT de aspecto real es
exactamente el tipo de dato que alguien puede llegar a creerse. No llevan las columnas `organo`
/ `reglamento` porque no son parametros normativos, son datos de negocio de ejemplo, igual que
las declaraciones.

## Coeficientes OTT `Wa`, `Wb`, `Wc`

Fixture: `Dataset.parametros()`, marcados sinteticos.

Forma: tres parametros decimales que **suman 1**. Los valores son redondos a proposito, para que
nadie los confunda con una ponderacion aprobada.

`RD 9.7` no los publica; la nota al pie 14 dice que se determinan por simulaciones (P-04).

Contrato con el motor: si un coeficiente **no esta** en el snapshot, `reparto` devuelve **error
tipado**, no un resultado en cero. Es criterio de aceptacion de #33, y es la diferencia entre "no
se pudo calcular" y "dio cero", que en un sistema que mueve dinero de terceros no es un matiz.

## Rating por franja horaria

**Fixture incompleta.** Hoy los valores viajan **inline en cada uso de TV**
(`usoTV(..., rating)`), no como tabla, y son **el unico dato sintetico del sembrador sin marcar**:
todos los `parametros` son `publicado` o `sintetico`, el rating no es ninguno.

Forma que debe tener (P-06): tabla indexada por **(canal, franja horaria, ano)**.

| Campo | Tipo | Nota |
| ----- | ---- | ---- |
| `canal` | texto | `RD 9.1.1` calcula el valor punto **por canal** |
| `franja` | texto | Franja horaria; el catalogo de franjas lo define el proveedor |
| `ano` | entero | El feed se actualiza anualmente |
| `rating` | decimal | Rango observado en la fixture actual: 2.0 a 9.0 |

Pendiente de implementar: sacar el rating de la firma de `usoTV`, llevarlo a una tabla propia y
marcarlo sintetico con el mismo mecanismo de dos columnas. Es cambio de codigo, no de este
documento: corresponde a un seguimiento de #22, o a #26 si se prefiere resolverlo al normalizar.

## Mapeo de generos

**No existe como fixture ni como dato.** Es el insumo que hoy impide que una fila de parrilla
sepa que ponderacion le toca.

Respuesta provisional (P-05), aprobada por el equipo el 2026-09-13:

Desde la columna `TIPO` de la parrilla:

| `TIPO` | Tipo de obra | Ponderacion |
| ------ | ------------ | ----------- |
| Pelicula | `cinematografica` | 5.0 |
| Unitario | `unitario` | 2.8 |
| Sketch / humor | `sketches` | 0.8 |

Desde `SubGenero`:

| `SubGenero` | Tipo de obra | Ponderacion | Repertorio |
| ----------- | ------------ | ----------- | ---------- |
| Telenovela | `serie` | 1.3 | Si |
| Drama | `serie` | 1.3 | Si |
| Magazine | -- | -- | **No** |
| Noticiero | -- | -- | **No** |
| Agro | -- | -- | **No** |
| Entretenimientos | -- | -- | **No** |
| Religioso | -- | -- | **No** |

Las cuatro ponderaciones **si son reales**: son la tabla de `RD 9.1.1` y ya estan en el sembrador
como `publicado`. Lo sintetico es **el mapeo**, no el peso.

Los cinco marcados como no repertorio son el filtro de `R-27` a nivel de programa: un noticiero no
tiene guionista en el sentido de `RD 7.1`. **Entretenimientos es el mas dudoso** -- si son shows
con libretista de planta si son repertorio, y habria que asignarles ponderacion. Esta explicito en
la agenda de la reunion.

Un `SubGenero` que no aparezca en la tabla **no se mapea a un tipo por defecto**: se trata como
ausente y falla ruidosamente (ADR 0004). Adivinar la ponderacion es adivinar cuanto se le paga a
alguien.

Pendiente de implementar: la tabla debe vivir como **dato con vigencia**, no como constantes en
codigo, para que corregirla despues sea cambiar una fila. Corresponde a #26, que es donde una fila
de parrilla adquiere su tipo de obra.

## Registro de canales y atribucion del uso

Fixture: `Dataset.canales()`. Tablas `canales` y `canales_clasificacion` (migracion 00011).

Forma del canal: `(id, nombre, grupo_estructural)`. Forma de la clasificacion:
`(canal_id, anio_audiencia, grupo_efectivo)`.

Son dos tablas y no una columna porque `RD 9.5` clasifica de dos maneras distintas.
`grupo_estructural` es lo que no cambia (`RD 9.5.1`-`9.5.3`: privado nacional, regional o de
operacion publica, premium). `grupo_efectivo` se recalcula **cada ano** contra el quintil de
audiencia del ano inmediatamente anterior (`RD 9.5.4`), asi que reejecutar un periodo pasado
tiene que leer la fila de aquel ano y no la vigente hoy (ADR 0005).

En el sembrador, `anio_audiencia` es el ano del periodo **menos uno**. No sale del reloj: sale
del periodo, igual que en el nucleo.

El quintil real es **P-17** y sigue abierto: hace falta el feed del proveedor de audiencia. Los
dos canales del dataset se clasifican como `privado_nacional`, que es su grupo estructural, y por
tanto no afirman nada sobre rating que nadie haya medido.

### El canal en la fila de uso

`usos.canal_id` es **quien pago**, no quien entrego el archivo. Son cosas distintas y la
confusion sale cara: `fuente` es la entrega (ADR 0018), y una sola entrega de Caracol puede
cubrir varios canales. Por eso la columna existe aparte y por eso `usoTV` la exige.

No tiene clave foranea a `canales` a proposito (migracion 00011): conserva el identificador que
declaro la fuente aunque el catalogo anual todavia no conozca ese canal. Un canal sin fila en
`canales_clasificacion` devuelve grupo vacio, que fuera de suscripcion no se usa y dentro de ella
falla ruidosamente en el motor.

### Medidas de `RD 9.2` y `RD 9.4`

`espectadores` acompana a `taquilla` en el uso de cine porque el ejemplo de `RD 9.2` reparte por
espectadores mientras su prosa dice taquilla. La contradiccion es **P-18** y se resuelve con
`Snapshot.BaseCineTeatro`, no reescribiendo el esquema: por eso el dato tiene que estar sembrado.

`exhibiciones` es la medida de `RD 9.4` (transporte publico) y es distinta de `emisiones`. El
reporte de `expreso-bolivariano` la ejercita.
