---
actualizado: 2026-09-21
evidencia: postgres:16-alpine, 20.004 filas en `obras`, EXPLAIN (ANALYZE, BUFFERS)
---

# Por que el escalon 3 indexa con GiST y no con GIN

El mismo metodo que `docs/planes/90-buscar-obras/explain-ipi.md`: la eleccion de indice se
decide midiendo el plan, no razonando sobre el.

## La consulta

La del adaptador `Similitud.Candidatos`, con el piso de banda puesto en la sesion:

```sql
SET LOCAL pg_trgm.similarity_threshold = 0.45;

SELECT id, similarity(titulo_norm, titulo_normalizado($1)) AS puntaje
  FROM obras
 WHERE titulo_norm % titulo_normalizado($1)
 ORDER BY puntaje DESC, id ASC
 LIMIT 5;
```

## Las tres medidas

Catalogo sinteticio de 20.004 obras, consulta `'LA CASA DE LAS DOS PALMAS'`:

| Indice sobre `titulo_norm` | Plan elegido | Tiempo |
| --- | --- | --- |
| ninguno | `Seq Scan` | **68,0 ms** |
| `gin_trgm_ops` | `Seq Scan` (el planificador lo prefiere) | **68,0 ms** |
| `gin_trgm_ops` con `enable_seqscan = off` | `Bitmap Index Scan` | 7,3 ms |
| `gist_trgm_ops` | `Bitmap Index Scan`, **sin forzar nada** | **3,1 ms** |

## Lo que decide

GIN de trigramas *puede* servir esta consulta -7,3 ms cuando se le obliga- pero el
planificador no lo elige: el modelo de coste de `%` sobre GIN sobreestima el trabajo de
recorrer las listas de publicacion de cada trigrama y sale perdiendo contra un barrido
secuencial. **Un indice que el planificador no elige no existe**, y en produccion se pagarian
los 68 ms igual, con el agravante de que el indice si ocupa disco y si cuesta en cada
escritura.

GiST, ademas de que el planificador lo elige solo, encaja con el acceso real de este escalon:
lo que se pide no es "todas las que pasen el corte" sino "las N mas parecidas", y
`gist_trgm_ops` soporta el operador de distancia `<->` justo para eso.

## Por que importa el numero

`KR-1` (#46) pide 10.000 filas en menos de cinco minutos. A 68 ms por fila son **once
minutos**: fuera de limite. A 3,1 ms son **treinta segundos**.

El calculo es deliberadamente pesimista -supone una consulta de similitud por fila, sin alias
que corte antes-. En operacion real el escalon 1 se lleva casi todo a coste cero en cuanto la
tabla de alias se llena, que es precisamente el efecto que el ADR 0007 busca. El numero de
arriba es el del primer periodo, cuando no hay ningun alias aprendido.

## Reproducirlo

```
docker run -d --name trgm-check -e POSTGRES_PASSWORD=x -e POSTGRES_USER=intela \
  -e POSTGRES_DB=intela_test postgres:16-alpine
# aplicar 00001 + 00012, rellenar obras con generate_series, ANALYZE
# y comparar EXPLAIN (ANALYZE, BUFFERS) con cada indice
```

## Nota sobre `unaccent`

En PostgreSQL 16 las **dos** formas de `unaccent` son `STABLE`, comprobado sobre la imagen:

```sql
SELECT p.oid::regprocedure, p.provolatile
  FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
 WHERE p.proname = 'unaccent' AND n.nspname = 'public';
-- unaccent(text)                | s
-- unaccent(regdictionary,text)  | s
```

Por eso `titulo_normalizado` se declara `IMMUTABLE` envolviendo una llamada `STABLE`: es el
rodeo documentado y sin el no se puede crear ni la columna generada ni el indice. El razonamiento
completo y el precio asumido estan en la cabecera de `migrations/00012_matching_difuso.sql`.
En PostgreSQL 17 la forma de dos argumentos ya viene `IMMUTABLE`.
