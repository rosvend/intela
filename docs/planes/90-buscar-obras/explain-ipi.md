# EXPLAIN del filtro por IPI — issue #90

Medido contra PostgreSQL 16.6 con **50.003 obras** y **50.000 coautores**,
cache caliente, buscando `IPI-00000001` (una sola obra).

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

## Despues (UNION ALL con EXISTS suelto + LIMIT)

```
Limit
  ->  Sort
        ->  Nested Loop
              ->  HashAggregate
                    ->  Index Scan using obra_coautores_ipi on obra_coautores c
                          Index Cond: (ipi = 'IPI-00000001'::text)
              ->  Index Only Scan using obras_pkey on obras o
                    Index Cond: (id = c.obra_id)
Execution Time: 0.130 ms
```

`obra_coautores_ipi` guia el Nested Loop hacia `obras`. La rama del UNION ALL
sin IPI queda vacia (`WHERE $5 = ''` es falso) y no se materializa.
