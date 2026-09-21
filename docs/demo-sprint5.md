---
actualizado: 2026-09-15
objetivo: 14
audiencia: Product Owner / presentación Sprint 5 (antes del 26 oct)
prerrequisito: sistema sembrado según docs/ARRANQUE.md
---

# Guion de demo — Sprint 5 (PO)

Camino exacto para la presentación: **cargar un reporte → resolver un match →
reparto Nacional con sus compuertas → explicar una cifra → exportar una
liquidación**.

Duración objetivo: **12–15 minutos**. Quién no escribió el guion debe hacer un
dry-run antes del día de la demo.

## 0. Preparación (antes de la sala)

En una máquina limpia o en el entorno desplegado:

```bash
docker compose up -d --build
docker compose run --rm seed          # una vez; o SEED_RESET=true si hace falta
curl -fsS http://localhost/api/ready  # debe responder listo
```

Cuentas (claves por defecto de `ARRANQUE.md`):

| Correo | Rol | Para qué en la demo |
| ------ | --- | ------------------- |
| `admin@redes.co` / `admin-local` | administrador | Ingesta, catálogo, visión completa |
| `distribucion@redes.co` / `distribucion-local` | distribución | Avanzar etapas / firmar Verificación |
| `contabilidad@redes.co` / `contabilidad-local` | contabilidad | Segunda firma en Pago y Registro |
| `ana@redes.co` / `ana-local` | titular | Explicar cifra y descarga |

**Nota de honestidad (andamiaje UI).** Varias rutas del tablero (`/ingesta`,
`/anomalias`, `/distribucion`, `/reportes`) pueden mostrar
*«Esta pantalla llega en un PR posterior»* hasta que aterrizen los issues
[#37](https://github.com/rosvend/intela/issues/37),
[#40](https://github.com/rosvend/intela/issues/40),
[#42](https://github.com/rosvend/intela/issues/42),
[#43](https://github.com/rosvend/intela/issues/43). El guion marca, en cada
paso, **UI** vs **API de respaldo**. En dry-run: si la pantalla es placeholder,
usar la API; si ya está cableada, seguir solo UI.

Archivo de muestra sugerido (repo): un extracto Caracol/Netflix bajo
`data/files/` (el que ya pase `TestCaracolXLSXRealEntraEntero` /
`TestNetflixXLSXRealEntraEntero`).

---

## 1. Entrar y mostrar autorización (1 min)

1. Abrir <http://localhost> → `/login`.
2. **Ingresar** como `admin@redes.co`.
3. Mostrar el sidebar: nueve ítems (Principal + Configuración).
4. Decir en una frase: *la autorización real es del servidor (#17); el menú
   solo es cosmético*.

Opcional rápido: salir → entrar como `ana@redes.co` → sidebar reducido a
Inicio → intentar `/catalogo` → **No autorizado** → volver a admin.

---

## 2. Cargar un reporte (ingesta) (2–3 min)

### UI (cuando `/ingesta` esté lista)

1. Desde Inicio: CTA **Nueva ingesta** o nav **Ingesta** → `/ingesta`.
2. Elegir fuente (p. ej. Caracol), periodo (`2025-01` o el del seed), archivo.
3. Confirmar carga. Mostrar acuse: huella SHA-256, conteo de filas, periodo.
4. Volver al listado de cargas / KPI **Cargas pendientes**.

### API de respaldo

```bash
# Sesión: POST /api/sesiones con correo/clave de admin; guardar cookie
# Luego POST multipart a /api/reportes (ver OpenAPI tag ingesta)
curl -fsS -b cookies.txt -F "fuente=caracol" -F "periodo=2025-01" \
  -F "archivo=@ruta/al/archivo.xlsx" \
  http://localhost/api/reportes
```

**Qué decir:** los reportes **no traen dinero**; solo ponderan el reparto de la
bolsa (`AGENTS.md` punto 1). La evidencia cruda queda inmutable.

---

## 3. Resolver un match / ONI (2–3 min)

### UI (cuando `/anomalias` esté lista)

1. KPI **ONI** → **Ir a anomalías**, o nav **Anomalías** → `/anomalias`.
2. Abrir un uso pendiente / ONI del seed.
3. Resolver manualmente (asociar a obra del catálogo) o mostrar exclusión fuera
   de repertorio (`R-27`).
4. Confirmar que queda asiento / estado auditable (no asignación a ciegas;
   ADR 0007).

### API / evidencia de código si la UI no está

1. Mostrar en catálogo una obra del seed y un uso pendiente vía API o SQL.
2. Citar la cascada: alias → ID global → fuzzy → cola manual
   (`docs/planes/28-cascada-identificacion/`).
3. Mencionar prueba: `TestResolverUsosExcluyeSinSondearYGuardaLaExclusion`
   (excluido ≠ ONI público).

**Qué decir:** un falso positivo paga a quien no corresponde; por eso lo dudoso
cae a revisión humana (`R-05`).

---

## 4. Reparto Nacional con compuertas (4–5 min)

Modelo de proceso (ADR 0008 / `RD 13.5`):

```
Recaudo → Deducciones → Importe de la Obra → Importe por Titular →
Liquidación Parcial → Verificación → Liquidación Final →
Pago y Registro → Auditoría
```

Compuertas de doble firma: **Verificación** y **Pago y Registro**.
Roles: `distribucion` y `contabilidad` **no se solapan** a propósito.

### UI (cuando `/distribucion` esté lista)

1. Entrar como `distribucion@redes.co`.
2. Nav **Distribución** → `/distribucion`.
3. Abrir el proceso Nacional del periodo sembrado.
4. Avanzar etapas visibles hasta **Liquidación Parcial**.
5. En **Verificación**: firmar como distribución.
6. Salir → entrar como `contabilidad@redes.co`.
7. Intentar abrir `/distribucion` → debe fallar (**No autorizado**): contabilidad
   no opera el panel de distribución; firma en la compuerta de pago/registro
   desde el flujo que el módulo exponga (o panel de aprobaciones).
8. Completar **Pago y Registro** con la segunda firma.
9. Mostrar que el proceso queda en **Auditoría** / cerrado.

### Si el motor de proceso aún no está cableado en UI

1. Proyectar el diagrama ADR 0008 (etapas Nacional vs Internacional).
2. Mostrar bolsas Nacional e Internacional del mismo periodo conviviendo
   (`R-35`, `TestNacionalEInternacionalDelMismoPeriodoConviven`).
3. Advertir en voz alta: **P-10** — tasas de deducción/reserva son `sintetico`;
   *esta cifra del demo no se cita en auditoría*.

**Qué decir:** el cálculo es determinista (ADR 0005); el proceso mete humanos
donde el reglamento exige firmas.

---

## 5. Explicar una cifra (2 min)

1. Entrar como `ana@redes.co`.
2. Inicio → **Mi liquidación** / tarjeta **Última liquidación** → **Ver detalle**.
3. Elegir un monto → acción **explicar esta cifra** (misma página; issue #42).
4. Narrar la cadena: corrida → reporte → obra → declaración/split → regla →
   versión de parámetros (Objetivo 11 / ADR 0006).

### Respaldo

Si el botón aún no existe: abrir un asiento de bitácora vía API
(`/api/asientos/...` según OpenAPI) o mostrar en código/tests de bitácora que
cada cifra tiene origen append-only.

---

## 6. Exportar liquidación (1–2 min)

### UI (cuando esté lista)

1. Como titular, desde el detalle de liquidación: **Exportar** PDF/Excel, o
2. Como admin/auditor: nav **Reportes** → `/reportes` → descargar liquidación
   del periodo.

### Respaldo

Mostrar el contrato OpenAPI / issue #43 y, si hay endpoint, `curl` de descarga.
Si solo hay placeholder: declarar el gap y el criterio de aceptación del
Objetivo 10 (*titular descarga su liquidación*).

---

## 7. Cierre (30 s)

Checklist verbal para el PO:

- [ ] Ingesta con evidencia y periodo
- [ ] Match/ONI sin asignación ciega
- [ ] Nacional con dos firmas distintas
- [ ] Cifra explicable hasta origen
- [ ] Export (o gap explícito + fecha de cierre)
- [ ] Matriz Objetivo 12: `docs/dominio/matriz-reglas.md`

Recordatorio final: cifras del seed con parámetros `sintetico` **no** son
evidencia de auditoría hasta P-10.

---

## Dry-run (verificación del AC)

| Quién | Qué hace |
| ----- | -------- |
| Persona distinta al autor del guion | Ejecuta pasos 0–6 en el entorno sembrado |
| Revisor de matriz | Confirma que cada `Test…` de `matriz-reglas.md` existe y pasa |

Registrar fecha, entorno (local / staging) y desviaciones UI→API en la sección
14 del [`entregable-consolidado.md`](entregables/entregable-consolidado.md).
