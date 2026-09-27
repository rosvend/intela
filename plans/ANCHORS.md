# Decision Anchor Manifest
*Committed, append-only. One line per anchored decision, so a `# DECISION <plan-id>/D-NNN` anchor still resolves after its plan directory is gone.*
*Format: `<plan-id>/D-NNN | YYYY-MM-DD | one-line rationale`. Never edited, never reordered, never trimmed.*
*Written by ip-archivist at CLOSE. Read by validate-plan.mjs as the durable anchor-resolution tier.*
plan-2026-09-20T043343-f3785d3e/D-016 | 2026-09-20 | La union de errores tipados es UNA regla: se centraliza en el predicado `esErrorDeApi` exportado desde `web/src/api.ts`, y `useApi` y `tablero/useDashboard` dejan de mantener dos copias en lockstep.
