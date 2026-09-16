# EXPLAIN del filtro por IPI — issue #90

Medido contra PostgreSQL 16.15 con **50.003 obras** y **50.004 coautores**,
cache caliente, sentencia preparada con parametros ligados (como la manda
pgx v5 con `QueryExecModeCacheStatement`).

## Antes (`$5 = '' OR EXISTS (...)`)

```
Index Only Scan using obras_pkey on obras o
  Filter: (hashed SubPlan 2)
  Rows Removed by Filter: 50002
  ->  Index Scan using obra_coautores_ipi on obra_coautores c
        Index Cond: (ipi = 'IPI-00000001'::text)
Execution Time: 31.449 ms
```

El indice de IPI alimenta el hash del subplan, pero el barrido exterior sigue
siendo `obras` entero: el coste crece con el catalogo, no con el resultado.

## Despues — camino IPI (UNION ALL + One-Time Filter, plan generico)

Forzado con `SET plan_cache_mode = force_generic_plan`. `$5` es placeholder;
aun asi la rama muerta se poda porque el qual sin Vars es pseudoconstante:

```
Limit (actual time=0.366..0.410 rows=3 loops=1)
  ->  Append
        ->  Result   One-Time Filter: ($5 <> ''::text)
              ->  Nested Loop
                    ->  HashAggregate
                          ->  Index Scan using obra_coautores_ipi on obra_coautores c
                                Index Cond: (ipi = $5)
                    ->  Index Scan using obras_pkey on obras o
        ->  Result   One-Time Filter: ($5 = ''::text)
              ->  Nested Loop Left Join (never executed)
                    ->  Seq Scan on obras o_1 (never executed)
Execution Time: 0.751 ms
```

`obra_coautores_ipi` guia el Nested Loop. `never executed` en la rama muerta.
No se degrada en la sexta ejecucion (plan generico).

## Despues — listado sin filtros (LATERAL despues del LIMIT)

Primera pagina, `limite=100`, sin IPI. El LATERAL corre solo contra las 100
filas que sobreviven al recorte:

```
Nested Loop Left Join (actual time=417.699..420.677 rows=100 loops=1)
  ->  Limit (actual time=417.482..417.865 rows=100 loops=1)
        ->  Sort -> Append -> Seq Scan on obras o_2 (rows=50003)
  ->  Aggregate (actual time=0.013..0.014 rows=1 loops=100)
Execution Time: 421.062 ms
```

`loops=100`, no `loops=50003`. El Seq Scan + top-N sort (~147 ms suelto con
la forma de filtros neutralizados) es inherente a la decision 8 de #86, no
a esta consulta.

### Regresion evitada (LATERAL antes del LIMIT)

Sin mover el LATERAL fuera del UNION, la misma pagina costaba 1.344 ms con
`loops=50003` en el `jsonb_agg` — 2,5× el coste de main sirviendo las 50.003
obras completas (~534 ms).
