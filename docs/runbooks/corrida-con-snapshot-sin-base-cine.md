# Runbook: corrida de cine abierta con un snapshot sin `cine_teatro.base`

Una corrida de cine o teatro abierta antes de #194 no sale de `deducciones`.
La corrida conocida es `proc-prueba-motor-procinal-2025-01` (bolsa
`bolsa-procinal-2025-01-nacional`).

## Sintoma

`POST /procesos/{id}/avanzar` hacia `importe_obra` responde:

```
409 avanzar etapa de "<id>": motor de reparto: parametro normativo ausente: base_cine_teatro
(clave cine_teatro.base, P-18): el snapshot "snp1-..." de la corrida no lo trae con un valor
valido y no se vuelve a resolver; con la vigencia correcta cargada, abra una corrida nueva de
la bolsa "<bolsa>"
```

Antes de #194 la misma corrida respondia `500 {"error":"no se pudo avanzar el proceso"}`.

## Causa

El snapshot de parametros se congela al abrir la corrida y no se vuelve a
resolver (ADR 0005). Las corridas abiertas con la version 1 del conjunto de
clausulas (id `snp1-...`) no traen `cine_teatro.base`, y el motor la exige
para cine y teatro. Cargar ahora la vigencia no cambia ese snapshot. Reintentar
tampoco.

## Que hacer

1. Confirmar que `cine_teatro.base` tiene vigencia en la fecha del periodo. En
   una base sembrada con el dataset sintetico, la migracion 00025 la agrega con
   `taquilla` (provisional, P-18). En una base con parametros reales la carga
   el organo competente como una fila nueva de `parametros`.
2. Abrir una corrida nueva de la misma bolsa con otro id, por ejemplo
   `POST /procesos` con `{"id": "proc-procinal-2025-01", "periodo": "2025-01",
   "circuito": "nacional", "bolsa_id": "bolsa-procinal-2025-01-nacional"}`. Su
   `snapshot_id` empieza por `snp2-`.
3. Avanzar la corrida nueva. Valoriza los usos con `canal_id = procinal`, que
   son los del reporte de la fuente `cine`.

## Que pasa con la corrida vieja

Hoy no hay ninguna operacion para anularla, cerrarla o marcarla como
reemplazada. La API expone abrir, avanzar, firmar y rechazar, y rechazar solo
retrocede una etapa (ADR 0008). La corrida vieja se queda en `deducciones`.

- No mueve dinero. Nunca valoriza, asi que no tiene `resultados_proceso`, y la
  liquidacion solo agrega corridas en `liquidacion_final` con las dos firmas
  (`ProcesosListos`, ADR 0019).
- Si aparece en `/procesos` junto a la nueva, con la misma bolsa. La que vale
  es la que tiene `snapshot_id` con prefijo `snp2-`. La de `snp1-` en
  `deducciones` es la reemplazada.
- No se borra a mano en la base. El registro de la corrida y sus asientos son
  trazabilidad (ADR 0006).

Anular o marcar como reemplazada una corrida es una operacion nueva, con su
propia decision sobre estado y auditoria. Queda pendiente, fuera de #194.
