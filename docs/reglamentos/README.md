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

## Cuando llegue una version nueva

1. Dejar el PDF nuevo en `fuente/` y **conservar el anterior**: los repartos historicos se
   calcularon con la version vigente en su momento.
2. Registrar el documento en `DOCUMENTS` dentro de `src/scripts/convert_reglamentos.py`, con
   su version, fecha de aprobacion y lista de secciones, usando un `slug` nuevo, por ejemplo
   `distribucion-v10`.
3. Regenerar y revisar el diff.
4. Actualizar las citas afectadas en `docs/dominio/` y anotar el cambio en
   `docs/decisiones/`.

## Limitaciones conocidas de la conversion

`pdftotext` no extrae objetos de ecuacion de Word ni la estructura de los diagramas. La
ecuacion de `RD 9.7` es el caso conocido y esta transcrita a mano en
`docs/dominio/formulas.md`. Cada `00-indice.md` lista las limitaciones de su documento.
