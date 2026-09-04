-- Catalogo maestro de obras: los campos que le faltaban y sus coautores.
--
-- `obras` nacio con id, titulo, tipo y los tres identificadores globales. El
-- catalogo que pide OE-2 necesita ademas genero y anio de produccion -los dos
-- van en la propia Declaracion de Obra, `RD 13.1.2`- y los coautores con su
-- IPI, que es por donde se busca a una persona (`RD 3`).
--
-- El numero 00003 y no 00002: hay cuatro PRs abiertos que reclaman el 00002 a
-- la vez. Dos ficheros con la MISMA version son un error duro de goose en cada
-- arranque, incluido el contenedor de pruebas; un hueco entre versiones no lo
-- es. Quien merguee despues renumera con un `git mv`.

-- +goose Up

-- ---------------------------------------------------------------------------
-- Los dos campos que faltaban
-- ---------------------------------------------------------------------------
--
-- Se anaden con DEFAULT y el DEFAULT se retira acto seguido: la columna es NOT
-- NULL, asi que sin un valor de relleno el ALTER no puede correr sobre una
-- tabla con filas; y con el DEFAULT permanente, una fila nueva podria entrar
-- sin genero y sin anio, que es justo lo que el catalogo no puede permitir.
--
-- Los dos CHECK van despues, y ahi esta la parte que conviene saber: sobre una
-- base que YA tenga obras, el CHECK falla y la migracion se para. Es lo
-- correcto. La alternativa -inventar un genero de relleno y darlo por bueno-
-- mete datos que nadie declaro en el sitio contra el que resuelve todo el
-- matching. Hoy `obras` no tiene filas en ningun entorno: el comando de seed
-- todavia no existe.
-- +goose StatementBegin
ALTER TABLE obras
  ADD COLUMN genero TEXT NOT NULL DEFAULT '',
  ADD COLUMN anio   INT  NOT NULL DEFAULT 0;

ALTER TABLE obras
  ALTER COLUMN genero DROP DEFAULT,
  ALTER COLUMN anio   DROP DEFAULT;

-- `genero` es texto libre A PROPOSITO, y no un CHECK como `tipo`.
--
-- Son dos clasificaciones distintas y no hay que confundirlas: `tipo` es la
-- del reglamento, la que usa la metodologia de reparto (`RD 9.1.1`), y por eso
-- esta cerrada. `genero` es el del catalogo y el de las parrillas -Telenovela,
-- Magazine, Noticiero, Agro...-, y el mapeo entre esos generos y las cuatro
-- categorias del reglamento es una pregunta ABIERTA con el cliente
-- (docs/dominio/reglas-negocio.md, pregunta 5). Cerrar el enum aqui seria
-- tomar esa decision de tapadillo, en una migracion.
ALTER TABLE obras
  ADD CONSTRAINT obra_genero_no_vacio CHECK (btrim(genero) <> ''),
  ADD CONSTRAINT obra_anio_positivo   CHECK (anio > 0);

-- Filtros exactos del buscador. El de titulo ya existe desde 00001
-- (`obras_titulo_trgm`, GIN con gin_trgm_ops) y sirve tal cual para el ILIKE
-- '%...%' de la busqueda parcial: no hace falta crearlo otra vez.
CREATE INDEX obras_genero ON obras (genero);
CREATE INDEX obras_anio   ON obras (anio);
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Coautores del catalogo
-- ---------------------------------------------------------------------------
--
-- NO tiene columna de porcentaje, y no es un olvido.
--
-- Esta tabla dice QUIEN ESCRIBIO la obra; `declaraciones` dice A QUIEN SE LE
-- PAGA y cuanto. Son la misma clase de dato que las columnas `Autor*` y
-- `Guionista*` de una parrilla: evidencia para identificar la obra, jamas
-- insumo de reparto (`R-02`, `R-03`, `RD 7.3.1`). Un `porcentaje` aqui abriria
-- un segundo camino hasta un pago, y ese no lo firma ninguna Declaracion.
--
-- Tampoco referencia a `titulares`: una obra del catalogo puede nombrar a
-- quien todavia no esta afiliado, y exigir el padron para catalogar dejaria
-- obras fuera del cubo contra el que resuelve el matching. Lo que si se exige
-- es el IPI, que es el identificador de PERSONAS de la CISAC
-- (docs/dominio/identificadores.md) y por donde busca el catalogo.
--
-- `rol` esta cerrado por CHECK porque `RD 7.3.3` deja fuera por su nombre a
-- productores ejecutivos, revisores, ejecutivos de cadena, actores y
-- directores de casting. Con texto libre, un "director" entra como coautor y
-- nadie lo nota (`R-01`, `R-02`).
--
-- La clave primaria es (obra_id, ipi, rol): la misma persona puede figurar en
-- dos roles -guionista y adaptador de su propia obra-, y dos personas pueden
-- compartir rol. Lo que no puede repetirse es el par.
-- +goose StatementBegin
CREATE TABLE obra_coautores (
  obra_id TEXT NOT NULL REFERENCES obras(id) ON DELETE CASCADE,
  ipi     TEXT NOT NULL CHECK (btrim(ipi) <> ''),
  nombre  TEXT NOT NULL CHECK (btrim(nombre) <> ''),
  rol     TEXT NOT NULL
    CHECK (rol IN ('guionista','libretista','adaptador','argumentista')),
  PRIMARY KEY (obra_id, ipi, rol)
);

-- Buscar una obra por el IPI de uno de sus coautores es uno de los tres
-- caminos del buscador, y sin este indice recorre la tabla entera.
CREATE INDEX obra_coautores_ipi ON obra_coautores (ipi);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS obra_coautores;

DROP INDEX IF EXISTS obras_anio;
DROP INDEX IF EXISTS obras_genero;

ALTER TABLE obras
  DROP CONSTRAINT IF EXISTS obra_anio_positivo,
  DROP CONSTRAINT IF EXISTS obra_genero_no_vacio;

ALTER TABLE obras
  DROP COLUMN IF EXISTS anio,
  DROP COLUMN IF EXISTS genero;
-- +goose StatementEnd
