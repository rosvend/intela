# Reglamentos de REDES SGC

Texto verbatim de los documentos oficiales, partido por seccion para poder citarlo. Es la
fuente de verdad: cuando el resumen de `docs/dominio/` y el reglamento discrepen, **manda el
reglamento** y hay que corregir el resumen.

Nadie lee estos archivos de principio a fin. Se llega a ellos por cita desde
`docs/dominio/reglas-negocio.md`, o buscando el numeral (`grep -r "13.1.3" docs/reglamentos/`).

## Documentos

| Documento | Version | Aprobado | Carpeta |
| --------- | ------- | -------- | ------- |
| Reglamento de Distribucion | IX | 2026-05-27 | [distribucion-v9/](distribucion-v9/00-indice.md) |
| Reglamento de Tarifas | VI | 2026-06-30 | [tarifas-v6/](tarifas-v6/00-indice.md) |
| Reglamento de Socios | V5 | sin fecha en el documento | [socios/](socios/00-indice.md) |
| Reglamento de Anticipos a Afiliados | V7 | sin fecha en el documento | [anticipos/](anticipos/00-indice.md) |

Los PDF y el formato de afiliacion originales estan en [fuente/](fuente/).

## Abreviaturas usadas en las citas

`RD` Reglamento de Distribucion IX, `RT` Reglamento de Tarifas VI, `RS` Reglamento de Socios,
`RA` Reglamento de Anticipos. Asi, `RD 13.1.3` es la seccion 13.1.3 del de Distribucion.

## Regenerar

```
uv run --script src/scripts/convert_reglamentos.py
```

Reconstruye todas las carpetas desde los PDF de `fuente/`. Requiere `pdftotext`
(poppler-utils). El script imprime las secciones detectadas y avisa si no encuentra todas las
esperadas.

## Indice semantico para el asistente (#67)

La herramienta `buscar_reglamento` del asistente busca por significado sobre estos `.md`,
partidos por numeral (`RD 9.1.1`), y devuelve siempre el texto verbatim **con su cita**. Si
ninguna seccion supera el piso de similitud responde "no encontrado"; nunca fuerza una
coincidencia debil (ADR 0004).

```
DATABASE_URL=... EMBEDDINGS_PROVEEDOR=falso make indexar-reglamento
```

Reindexar es deliberado, como regenerar: se corre cada vez que cambian estos `.md`, nunca al
arrancar la API. Reemplaza entero el indice del modelo en una transaccion.

- `EMBEDDINGS_PROVEEDOR`: vacio = la herramienta no se ofrece al modelo; `falso` = hashing de
  palabras, determinista y sin red (desarrollo y pruebas: solo coincidencia lexica);
  `bedrock` = Amazon Titan Text Embeddings V2 (`EMBEDDINGS_MODELO`, por defecto
  `amazon.titan-embed-text-v2:0`, 1024 dimensiones, credenciales y region de la cadena
  estandar de AWS). **El proveedor de produccion no esta decidido.**
- El indice se guarda por modelo: cambiar de proveedor exige reindexar con el nuevo. La API y
  el indexador tienen que usar el mismo `EMBEDDINGS_PROVEEDOR`/`EMBEDDINGS_MODELO`.
- Pisos de similitud (`internal/infraestructura/embeddings`): 0.2 falso, 0.35 Titan.
  **Provisionales** hasta calibrarlos con preguntas reales del staff.
- Requiere la extension pgvector (migracion 00029). Local y CI usan
  `pgvector/pgvector:0.8.1-pg16`. En RDS PostgreSQL 16 la extension `vector` viene incluida y
  la crea la migracion con el usuario maestro (`rds_superuser`). Con `bedrock`, el rol de la
  Lambda necesita ademas `bedrock:InvokeModel` sobre el modelo y acceso al modelo habilitado
  en la consola de Bedrock; nada de eso esta en `infra/` todavia.

## Cuando llegue una version nueva

1. Dejar el PDF nuevo en `fuente/` y **conservar el anterior**: los repartos historicos se
   calcularon con la version vigente en su momento.
2. Registrar el documento en `DOCUMENTS` dentro de `src/scripts/convert_reglamentos.py`, con
   su version, fecha de aprobacion y lista de secciones, usando un `slug` nuevo, por ejemplo
   `distribucion-v10`.
3. Regenerar y revisar el diff. Despues, `make indexar-reglamento` para que el asistente lo vea.
4. Actualizar las citas afectadas en `docs/dominio/` y anotar el cambio en
   `docs/decisiones/`.

## Limitaciones conocidas de la conversion

`pdftotext` no extrae objetos de ecuacion de Word ni la estructura de los diagramas. La
ecuacion de `RD 9.7` es el caso conocido y esta transcrita a mano en
`docs/dominio/formulas.md`. Cada `00-indice.md` lista las limitaciones de su documento.
