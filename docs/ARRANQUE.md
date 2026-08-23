# Arranque local

```bash
docker compose up --build
```

UI: http://localhost  
API: http://localhost/api  
Clave de todos los usuarios seed: `intela`

| Email | Rol |
| --- | --- |
| admin@intela.local | administrador |
| titular@intela.local | titular (Ana) |
| auditor@intela.local | auditoria |
| distribucion@intela.local | distribucion (firma) |
| contabilidad@intela.local | contabilidad (contrafirma) |

## Que es real y que es sintetico

Real (muestras de cliente, incompletas): `data/files/` si existe, mas `data/muestras/*.csv` de demo.

Sintetico, documentado como tal:

- Declaraciones de obra (el export de REDES-SYS no llego)
- Bolsas de recaudo (no hay facturas)
- Rating de franja
- Coeficientes OTT `Wa/Wb/Wc` (ADR 0004: parametros con vigencia; valores seed etiquetados `RD-IX-seed-sintetico`)

Ver bloqueos en `docs/dominio/fuentes-datos.md`.
