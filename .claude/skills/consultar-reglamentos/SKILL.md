---
name: consultar-reglamentos
description: Usar cuando haga falta el texto exacto o la cita de un reglamento de REDES SGC - verificar una regla contra la fuente, buscar un numeral, resolver una duda legal o de negocio, o comprobar que dice el reglamento sobre afiliacion, tarifas, plazos, prescripciones o auditoria.
---

# Consultar los reglamentos

## Como buscar

El texto verbatim esta en `docs/reglamentos/`, partido por seccion. Buscar por numeral es lo
mas rapido:

```
grep -rn "13.1.3" docs/reglamentos/distribucion-v9/
grep -rni "prescripcion" docs/reglamentos/
```

Cada carpeta tiene un `00-indice.md` con la tabla de secciones y las paginas del PDF original,
para verificar contra el documento firmado.

**No usar busqueda semantica para esto.** Un reglamento se consulta por numeral exacto: hace
falta `RD 13.1.3`, no algo parecido a "reservas".

## Que documento tiene que

| Tema | Documento |
| ---- | --------- |
| Como se calcula y se reparte el dinero, plazos, ONI, prescripciones, auditoria | `distribucion-v9/` |
| Cuanto se le cobra a cada tipo de usuario | `tarifas-v6/` |
| Quien puede afiliarse, categorias, requisitos, organos | `socios/` |
| Anticipos sobre derechos futuros | `anticipos/` |

Abreviaturas de cita: `RD` Distribucion IX, `RT` Tarifas VI, `RS` Socios, `RA` Anticipos.

## Mapa del Reglamento de Distribucion IX

| Seccion | Contenido |
| ------- | --------- |
| 3 | Definiciones: IPI, IDA, ONI, Fees in Error, Declaracion de Obra, deducciones |
| 7 | Quien es titular y como se fijan los porcentajes entre coautores |
| 8 | Tipos de usuario y excepciones a la comunicacion publica |
| 9 | **Formulas de valorizacion por tipo de usuario** |
| 10 | Rendimientos financieros y fechas de corte |
| 12 | Fechas de distribucion |
| 13 | Sistema de distribucion: documentos, menor cuantia, ONI, sociedades extranjeras |
| 14 | Reserva del 5% y procedimiento de reclamaciones |
| 15 | Prescripciones |
| 16 | Auditoria |

## Jerarquia de fuentes

1. El PDF original en `docs/reglamentos/fuente/` manda siempre.
2. El markdown de `docs/reglamentos/` es conversion automatica del PDF.
3. `docs/dominio/` es interpretacion nuestra y puede quedar desactualizada.

Si `docs/dominio/` contradice al reglamento, **corregir `docs/dominio/`** y anotar el cambio.

## Limitaciones de la conversion

`pdftotext` no extrae objetos de ecuacion de Word ni la estructura de los diagramas. La
ecuacion OTT de `RD 9.7` no aparece en el markdown; esta transcrita a mano en
`docs/dominio/formulas.md`. Cada `00-indice.md` lista las limitaciones de su documento.

Ante cualquier cifra o formula critica, verificar contra el PDF usando el rango de paginas del
indice.

## Versiones

Los reglamentos se versionan y se sustituyen. Distribucion va en IX (27-05-2026) y Tarifas en
VI (30-06-2026). Un reparto historico se calculo con la version vigente en su momento, asi que
al revisar calculos pasados hay que confirmar contra que version se hicieron.
