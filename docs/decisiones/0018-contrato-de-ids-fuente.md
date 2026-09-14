# 0018 El contrato de `usos.ids_fuente`

Fecha: 2026-09-13
Estado: Vigente

## Contexto

`usos.ids_fuente` guarda los identificadores con que cada fuente nombra una fila de su reporte. La
columna es `TEXT` y la tocan dos modulos que se construyen en PRs distintas:

```
archivo del cliente ──► ingesta (adaptadores, seed) ──► usos.ids_fuente ──► cascada de identificacion ──► alias_obra
                          ESCRIBE                                               LEE
```

Nadie habia fijado que va dentro, y al integrar aparecieron tres formas incompatibles a la vez:

| Quien | Que escribia o esperaba |
| ----- | ----------------------- |
| seed (`main`) | el valor solo: `871732`, con `alias_obra.tipo_id = 'ID_Ficha'` |
| adaptadores de ingesta (PR #108) | el valor solo, tomado de `netflix_id` (episodio) |
| cascada (PR #99) | `clave=valor`, con `show_id` para Netflix |

El problema no es de estilo. `alias_obra` se busca por `(fuente, tipo_id, valor)` exactos: si quien
escribe y quien lee no usan la misma clave con la misma grafia, el escalon 1 de la cascada (ADR 0007)
no casa nunca, todo cae al difuso o a la cola manual, y nada falla de forma visible.

Dos hechos del dominio condicionan la decision (`docs/dominio/identificadores.md`):

- Los ids de fuente **no cruzan entre fuentes**. Solo sirven como alias de cada fuente contra el
  catalogo de REDES.
- Estan a **granularidades distintas**. Caracol trae un id de programa (`ID_Ficha`). Netflix trae tres:
  show (`show_id`), temporada (`series_id`) y episodio (`netflix_id`). La obra de REDES es el
  programa o show, asi que lo comparable con `ID_Ficha` es `show_id`, no `netflix_id`.

## Decision

**`ids_fuente` es una linea `clave=valor` por identificador, con la clave tomada de una lista cerrada
en minuscula que define Intela.** El formato vive en codigo, en `internal/aplicacion/idsfuente.go`, y
es el unico sitio donde se decide.

1. **Formato.** Una linea por identificador, `clave=valor`, separadas por `\n`, ordenadas por clave.
   Una fila puede traer varios (`show_id`, `series_id` y `netflix_id` a la vez) y tambien los globales.
   Un valor vacio no se escribe.
2. **Claves.** Lista cerrada, en minuscula, **independiente del encabezado del archivo**:

   | Clave | Fuente | Identifica |
   | ----- | ------ | ---------- |
   | `id_ficha` | Caracol | programa u obra |
   | `show_id` | Netflix | show |
   | `series_id` | Netflix | temporada |
   | `netflix_id` | Netflix | episodio |
   | `id_pelicula` | cine | pelicula (sintetica hasta que llegue el formato del cliente) |
   | `ida`, `eidr`, `imdb` | cualquiera | identificadores globales (escalon 2) |

   La clave es tambien el `alias_obra.tipo_id`. Anadir una clave es un cambio de este ADR y de la
   lista en codigo.
3. **Escritura.** Quien escriba `ids_fuente` (adaptadores de ingesta, seed) usa
   `aplicacion.EscribirIDsFuente` con las constantes `aplicacion.Clave*`. No escribe strings de clave a
   mano. Esa funcion rechaza con error una clave fuera de la lista, una clave repetida o un valor con
   `=` o salto de linea.
4. **Lectura estricta.** La cascada lee con `aplicacion.LeerIDsFuente`. Solo cuentan las lineas con
   clave de la lista y valor no vacio. Un valor sin clave, una clave desconocida o con otra grafia **se
   ignoran**: adivinar de que columna salio un valor es la forma de aprender un alias falso y atribuir
   usos a la obra equivocada.
5. **Clave del escalon 1 por fuente.** Caracol busca y aprende por `id_ficha`. Netflix, por `show_id`:
   un alias aprendido cubre todos los episodios del show. `netflix_id` se sigue guardando, porque es la
   clave con que la ingesta detecta filas duplicadas.

## Alternativas consideradas

**JSON en `ids_fuente`.** Mas estructurado a la vista, pero la columna sigue siendo `TEXT` sin
validacion. Anade escapes y un parser mas pesado, y no resuelve el problema real, que es acordar las
claves.

**Dos columnas, `tipo_id` y `valor`.** Lo mas estricto a nivel de esquema, pero solo cabe un
identificador por fila: se pierde guardar `show_id` junto a `netflix_id` y junto a `imdb`, que es
justo lo que Netflix y Caracol entregan. Exige ademas una migracion sobre `usos`.

**Claves con el nombre de la columna del archivo (`ID_Ficha`).** Coincidia con el seed de entonces,
pero ata `alias_obra.tipo_id` a un encabezado que el cliente controla. Si una entrega cambia `ID_Ficha`
por `Id_Ficha`, todos los alias aprendidos dejan de casar sin que nada falle.

**Lector tolerante (aceptar el valor sin clave, normalizar mayusculas).** Se probo como compatibilidad
durante la revision de la PR #99 y se descarto: una linea basura se convertia en identificador y
podia ensenar un alias falso. Un contrato estricto mas un escritor compartido evitan el problema en
el origen.

**`netflix_id` como clave del escalon 1.** Es unico por fila y por eso sirve para detectar duplicados.
Para alias sirve mal: cada episodio necesitaria su propio alias y su propio paso por la cola manual.

## Consecuencias

- Los adaptadores de ingesta (PR #108) tienen que escribir `ids_fuente` con `EscribirIDsFuente`. Para
  Netflix eso significa guardar `show_id`, `series_id` y `netflix_id`, no solo el ultimo.
- El seed escribe con el contrato, y sus alias usan las claves de la lista (`id_ficha`, `show_id`,
  `id_pelicula`). `TestCargarSiembraElAliasDeCadaUso` compara `tipo_id=valor` contra `ids_fuente`, asi
  que un seed que se salga del contrato deja sus usos huerfanos y la prueba falla.
- `TestIDsFuenteIdaYVuelta` ata escritura y lectura: lo que escribe un lado es exactamente lo que lee
  el otro.
- Una fila que llegue fuera de contrato no rompe la corrida, pero pierde el escalon 1. Sigue por id
  global si lo trae, o queda pendiente para el difuso. Es un error silencioso por diseno, y la forma de
  evitarlo es que ningun escritor pueda producirlo: por eso el escritor es una funcion compartida y no
  una convencion.
- Cambiar o renombrar una clave invalida los alias ya aprendidos con ella. Requiere una migracion de
  datos de `alias_obra` y un ADR que sustituya a este.
