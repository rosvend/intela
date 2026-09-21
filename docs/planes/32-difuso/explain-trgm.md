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

## Por que no se reescribio como KNN (review de #146, S2)

El review propuso quitar la transaccion por titulo (`EnTransaccion` + `SET LOCAL`, un par de
viajes de mas por fila) con una consulta que no necesitara el GUC: pedir los N vecinos mas
cercanos con el operador de distancia y aplicar el piso como filtro. `gist_trgm_ops` soporta
`<->` (distancia = 1 - similitud) para busqueda por vecindad, asi que era razonable esperar que el
indice la sirviera. Se midio antes de tocar el adaptador, con la misma tabla (20.004 obras,
`postgres:16.15-alpine`, `ANALYZE`, migraciones reales hasta 00013), y **no sirve**.

La candidata:

```sql
SELECT id, puntaje FROM (
  SELECT id, similarity(titulo_norm, titulo_normalizado($1)) AS puntaje
    FROM obras
   ORDER BY titulo_norm <-> titulo_normalizado($1), id
   LIMIT 5
) c
WHERE puntaje >= $2::numeric::float4
ORDER BY puntaje DESC, id ASC;
```

| Consulta | Plan | Tiempo |
| --- | --- | --- |
| Actual: `%` + `set_config(..., true)` | `Index Scan using obras_titulo_norm_gist` | **0,6 ms** |
| KNN con desempate por `id` | `Seq Scan` + `top-N heapsort` | **217 ms** |
| KNN con desempate por `id`, como sentencia preparada de plan generico | `Seq Scan` + `top-N heapsort` | 300 ms |
| KNN con desempate por `id` y `enable_seqscan = off` | `Seq Scan` + `top-N heapsort` (sigue sin usar el indice) | 270 ms |
| KNN **sin** desempate | `Index Scan using obras_titulo_norm_gist` | 6,2 ms |

El plan de la candidata, tal cual salio:

```
Limit  (actual time=217.239..217.241 rows=5 loops=1)
  ->  Sort  (actual time=217.238..217.239 rows=5 loops=1)
        Sort Key: ((obras.titulo_norm <-> 'la casa de las dos palmas'::text)), obras.id
        Sort Method: top-N heapsort  Memory: 25kB
        ->  Seq Scan on obras  (actual time=0.031..210.072 rows=20004 loops=1)
```

Lo que decide:

- **El desempate por `id` no es negociable.** Sin el, dos obras con el mismo puntaje en el
  limite del `LIMIT` salen en el orden que el indice recorra, y dos corridas sobre el mismo dato
  podrian tener por "mejor candidato" obras distintas: el ADR 0005 pide orden total. Con el, el
  planificador no combina un orden por `<->` del indice con una segunda clave (ni con Incremental
  Sort: `enable_seqscan = off` tampoco lo consigue) y barre la tabla.
- Sin el desempate el indice si se usa, pero 10 veces mas lento que `%` -el KNN visita 319
  paginas para dar 5 filas- y el problema anterior queda abierto.
- Las 10.000 filas de KR-1 (#46) a 217 ms son **treinta y seis minutos**. Con `%`, seis segundos.

Por eso el adaptador **conserva** `%` con `set_config('pg_trgm.similarity_threshold', ..., true)`
dentro de una transaccion, y solo cambia a [`enTransaccionDe`]: si el contexto ya trae la unidad
de trabajo de otro puerto, corre en ella en vez de abrir la suya. La otra salida del review,
fijar el GUC una vez por corrida, esta descartada: un GUC de sesion no sobrevive al pool, que
reparte conexiones distintas a cada consulta. `WHERE similarity(...) >= piso` sin `%` tampoco:
la funcion no es indexable y vuelve a ser un barrido (68 ms).

Un detalle que la transaccion deja al descubierto y que queda documentado en el metodo: dentro de
una unidad, el corte LOCAL dura hasta que termina **la unidad**. `ResolverUsos` consulta la
similitud fuera de su unidad de escritura, asi que no lo sufre.

## Reproducirlo

```
docker run -d --name trgm-check -e POSTGRES_PASSWORD=x -e POSTGRES_USER=intela \
  -e POSTGRES_DB=intela_test postgres:16-alpine
# aplicar 00001 + 00013, rellenar obras con generate_series, ANALYZE
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
completo y el precio asumido estan en la cabecera de `migrations/00013_matching_difuso.sql`.
En PostgreSQL 17 la forma de dos argumentos ya viene `IMMUTABLE`.
