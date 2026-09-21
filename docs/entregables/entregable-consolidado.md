---
actualizado: 2026-09-15
objetivo: 14
cliente: REDES SGC
equipo: Grupo 2 — Emanuel Acevedo, Roy Sandoval, Miguel Legarda, Santiago Mendoza
---

# Entregable consolidado — Intela

Documento de sustentación del **Objetivo 14**: una sección por objetivo
(Tabla 5.A de
[`Intela OKRs, US and ubiquituous language.md`](../Intela%20OKRs,%20US%20and%20ubiquituous%20language.md)),
con evidencia enlazada y **firma del responsable**.

Las firmas se completan al cerrar cada sección (nombre + fecha). Mientras
queden pendientes, el estado de la sección indica qué falta.

## Portada

| Campo | Valor |
| ----- | ----- |
| Proyecto | Intela — reconocimiento de obras y distribución de derechos (REDES SGC) |
| Curso / hito | PATIC II · Sprint 5 — Deployment · presentación ~26 oct |
| Repositorio | `intela/` (este árbol) |
| Presentación | Guion: [`docs/demo-sprint5.md`](../demo-sprint5.md) |
| Matriz de reglas | [`docs/dominio/matriz-reglas.md`](../dominio/matriz-reglas.md) |

### Resumen ejecutivo

Intela automatiza la **ingesta de reportes de uso**, la **identificación de
obras**, el **modelo de declaraciones/splits** y la **trazabilidad** del
reparto para REDES SGC, alineado al Reglamento de Distribución IX y a los
reglamentos de Tarifas, Socios y Anticipos. El dinero se reparte desde
**bolsas de recaudo** (no por fila de reporte); los porcentajes salen solo de
la Declaración de Obra; sin 100% declarado se retiene el total.

---

## Roles y RACI (referencia)

| Integrante | Rol en el proyecto |
| ---------- | ------------------ |
| Emanuel Acevedo | Líder / Product Owner |
| Roy Sandoval | Coordinación externa, dominio, consolidación de entregables, demo |
| Miguel Legarda | Dominio núcleo (catálogo, matching, splits, cálculo, trazabilidad) |
| Santiago Mendoza | Ingesta/adaptadores, resiliencia, despliegue |

Detalle RACI: secciones 4–4.3 del documento de OKRs.

---

## Objetivo 0 — Levantamiento del flujo real y de las fuentes

| | |
| --- | --- |
| **Responsable (R)** | Roy Sandoval |
| **Apoyo (A)** | Miguel Legarda |
| **Estado** | Entregado (fichas en repo; firma cliente pendiente donde aplique) |
| **Evidencia** | [`docs/dominio/fichas-fuente.md`](../dominio/fichas-fuente.md), [`fuentes-datos.md`](../dominio/fuentes-datos.md), [`preguntas-cliente.md`](../dominio/preguntas-cliente.md) |
| **Criterio** | Cada fuente del alcance tiene ficha; no se decide adaptador antes |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 1 — Adquisición autónoma con adaptadores

| | |
| --- | --- |
| **Responsable (R)** | Santiago Mendoza |
| **Apoyo (A)** | Miguel Legarda |
| **Estado** | Parcial (carga/API e idempotencia por huella; portal headless según fuente) |
| **Evidencia** | `internal/aplicacion` ingesta; `internal/infraestructura/ingesta`; bóveda de objetos; OpenAPI `/reportes` |
| **Criterio** | Adquisición programada o carga con original intacto y SHA-256 |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 2 — Normalización a esquema canónico

| | |
| --- | --- |
| **Responsable (R)** | Santiago Mendoza |
| **Apoyo (A)** | Emanuel Acevedo |
| **Estado** | Entregado para fuentes de muestra (Caracol, Netflix, cine) |
| **Evidencia** | Mapas por fuente; log de rechazos (ADR 0016); tests `TestGuardarUsosSeparaLoMalformadoSinDescartarlo`, lectores reales |
| **Criterio** | Inválidos en log con motivo; no se pierden ni se cuelan |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 3 — Catálogo maestro

| | |
| --- | --- |
| **Responsable (R)** | Miguel Legarda |
| **Apoyo (A)** | Santiago Mendoza |
| **Estado** | Entregado (modelo + API + seed) |
| **Evidencia** | Migraciones catálogo; `internal/dominio/repertorio`; HTTP obras; [`identificadores.md`](../dominio/identificadores.md) |
| **Criterio** | Obras con coautores, roles autorales, IDs externos |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 4 — Resolución de entidades (matching)

| | |
| --- | --- |
| **Responsable (R)** | Miguel Legarda |
| **Apoyo (A)** | Santiago Mendoza |
| **Estado** | Entregado en cascada (UI anomalías según #37) |
| **Evidencia** | ADR 0007; `internal/dominio/identificacion`; plan `docs/planes/28-cascada-identificacion/`; tests de resolución e integración |
| **Criterio** | Auto-asociación con umbral; dudoso → revisión manual |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 5 — Modelo de derechos y splits

| | |
| --- | --- |
| **Responsable (R)** | Miguel Legarda |
| **Apoyo (A)** | Roy Sandoval |
| **Estado** | Entregado (declaraciones; seed sintético por P-07) |
| **Evidencia** | `Declaracion` / `Completa()`; `R-03`/`R-04` en [`matriz-reglas.md`](../dominio/matriz-reglas.md); `TestDeclaracionesCubrenLosTresCasos` |
| **Criterio** | Suma vigente = 100%; periodo pasado usa split de entonces |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 6 — Motor de cálculo y distribución

| | |
| --- | --- |
| **Responsable (R)** | Miguel Legarda |
| **Apoyo (A)** | Santiago Mendoza |
| **Estado** | Parcial / bloqueado en tasas (P-04, P-10); vocabulario y bolsas listos |
| **Evidencia** | [`formulas.md`](../dominio/formulas.md); ADR 0005, 0008; `internal/dominio/recaudo`; paquete `reparto` |
| **Criterio** | Dos ejecuciones = mismo resultado; sin pérdida de centavos |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 7 — Orquestación temporal e idempotencia

| | |
| --- | --- |
| **Responsable (R)** | Miguel Legarda |
| **Apoyo (A)** | Santiago Mendoza |
| **Estado** | Parcial (`cmd/scheduler`, `cmd/worker`, cola ADR 0015; calendario) |
| **Evidencia** | `cola_trabajos`; tests de trabajos; seed no corre en `up` |
| **Criterio** | Temporizador; corrida repetida no duplica pago |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 8 — Resiliencia y observabilidad

| | |
| --- | --- |
| **Responsable (R)** | Santiago Mendoza |
| **Apoyo (A)** | Emanuel Acevedo |
| **Estado** | Parcial (`/health`, `/ready`, logs; sonda portal según adaptador) |
| **Evidencia** | `docs/cd.md`, `docs/ci.md`; endpoints de salud en ARRANQUE |
| **Criterio** | Fallo deja evidencia; resumen de corrida legible |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 9 — Conciliación y anomalías

| | |
| --- | --- |
| **Responsable (R)** | Santiago Mendoza |
| **Apoyo (A)** | Emanuel Acevedo |
| **Estado** | Parcial (duplicados por huella; ONI en dominio; UI #37) |
| **Evidencia** | Vista `oni_publico`; `R-18`/`R-27` en matriz; tests de exclusión |
| **Criterio** | ONI en bandeja con estado; sin montos en listado público |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 10 — Dashboard, panel de corridas y reportería

| | |
| --- | --- |
| **Responsable (R)** | Emanuel Acevedo |
| **Apoyo (A)** | Roy Sandoval |
| **Estado** | Parcial (login, RBAC, dashboards; módulos Sprint 5 en construcción) |
| **Evidencia** | `web/`; [`ARRANQUE.md`](../ARRANQUE.md); mockups `docs/mockups/` |
| **Criterio** | Titular filtra y descarga; admin ve corrida por etapa |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 11 — Trazabilidad y auditoría («explicar esta cifra»)

| | |
| --- | --- |
| **Responsable (R)** | Miguel Legarda |
| **Apoyo (A)** | Emanuel Acevedo |
| **Estado** | Parcial (bitácora append-only ADR 0006; UI #42) |
| **Evidencia** | Asientos; OpenAPI explicar; skill `trazabilidad-y-auditoria` |
| **Criterio** | Dado un monto: corrida, reporte, obra, regla, versión de split |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 12 — Reglas de negocio del reglamento

| | |
| --- | --- |
| **Responsable (R)** | Roy Sandoval |
| **Apoyo (A)** | Miguel Legarda |
| **Estado** | Entregado (matriz en repo; cobertura real uneven — ver estados) |
| **Evidencia** | [`matriz-reglas.md`](../dominio/matriz-reglas.md) ↔ [`reglas-negocio.md`](../dominio/reglas-negocio.md) |
| **Criterio** | Cada regla in-scope: fila + módulo + prueba + estado |

**Verificación:** revisor confirma nombres `Test…` contra `go test`.

**Firma R:** ______________________ Fecha: __________

**Firma revisor de pruebas:** ______________________ Fecha: __________

---

## Objetivo 13 — Despliegue reproducible

| | |
| --- | --- |
| **Responsable (R)** | Santiago Mendoza |
| **Apoyo (A)** | Miguel Legarda |
| **Estado** | Entregado localmente (`docker compose`); CD según `docs/cd.md` |
| **Evidencia** | `docker-compose.yml`; `cmd/seed`; [`ARRANQUE.md`](../ARRANQUE.md) |
| **Criterio** | Un comando levanta el sistema con datos de ejemplo; dashboard responde |

**Firma R:** ______________________ Fecha: __________

---

## Objetivo 14 — Documentación y entregables

| | |
| --- | --- |
| **Responsable (R)** | Roy Sandoval |
| **Apoyo (A)** | Todos |
| **Estado** | Este documento + guion de demo + matriz |
| **Evidencia** | Este archivo; [`demo-sprint5.md`](../demo-sprint5.md); OKRs; ADRs |
| **Criterio** | Entregado en fecha, **cada sección firmada por su responsable** |

### Checklist de cierre

- [ ] Secciones 0–13 firmadas por su R
- [ ] Dry-run del guion por alguien que no lo escribió (fecha: ______)
- [ ] Revisión de nombres de test de la matriz (fecha: ______)
- [ ] Presentación / slides alineados al guion
- [ ] Gaps UI (#37, #40, #42, #43) comunicados al PO si siguen abiertos

**Firma R (consolidación):** ______________________ Fecha: __________

**Visto bueno PO (Emanuel Acevedo):** ______________________ Fecha: __________

---

## Apéndice A — Artefactos clave

| Artefacto | Ruta |
| --------- | ---- |
| Registro de reglas | `docs/dominio/reglas-negocio.md` |
| Matriz Obj. 12 | `docs/dominio/matriz-reglas.md` |
| Fórmulas | `docs/dominio/formulas.md` |
| Preguntas al cliente | `docs/dominio/preguntas-cliente.md` |
| Decisiones (ADRs) | `docs/decisiones/` |
| Reglamentos verbatim | `docs/reglamentos/` |
| Guion demo Sprint 5 | `docs/demo-sprint5.md` |
| Arranque local | `docs/ARRANQUE.md` |

## Apéndice B — Alcance consciente (no sorpresas en auditoría)

1. **Tarifas `T-01`…`T-11`:** fuera del motor (P-08); Intela recibe lo cobrado.
2. **P-10:** sin acta de Asamblea, las tasas de deducción/reserva del seed son
   `sintetico` y no citables.
3. **P-04 / P-06:** coeficientes OTT y rating por franja condicionan cifras
   defendibles del motor de valorización.
4. **UI Sprint 5:** varias pantallas del camino de demo pueden seguir en
   construcción; el guion documenta API de respaldo.
