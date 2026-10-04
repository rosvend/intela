---
issue: 37
actualizado: 2026-09-27
---

# Issues de seguimiento de #37

La revision de la PR #158 pidio abrir estas issues. Quedan aqui con su texto original y su
estado, para que el codigo y la PR que las citan tengan donde remitir.

| Texto | Issue | Estado |
| ----- | ----- | ------ |
| A | #164 | Resuelta en la rama `feature/164-resolver-critica-corrige-dato` (R-37, ADR 0021 actualizado) |
| B | #165 | Cerrada por la PR #169 |
| — | #166 (limite TOCTOU entre evaluar y valorizar; ver la PR #171) | Abierta |

## A. Accion correctiva al resolver una anomalia critica (ligada a #39) — #164

**Titulo:** `[Backend] Resolver una anomalia critica tiene que corregir el dato, no solo cerrar la alerta (#39)`

**Cuerpo:**

> Hoy `POST /alertas/{id}/resolver` exige una nota y deja asiento, pero **no cambia los datos que
> pondera el reparto** (ADR 0021, "Que significa resuelta para el dinero"). Un
> `duplicado_registro` o `duplicado_archivo` resuelto abre la compuerta de `Procesos.AvanzarEtapa`
> y la fila duplicada **sigue ponderando**: el doble conteo se paga igual. Un
> `tipo_obra_sin_mapear` resuelto sigue abortando el motor (`ErrRepartoInvalido`).
>
> Pedido: que la resolucion de una critica lleve una accion correctiva explicita, con su propio
> asiento, dentro de la bandeja de #39:
>
> - `duplicado_registro`: excluir la fila (`escalon = excluido`, con motivo) o marcar cual de las
>   dos entregas manda.
> - `duplicado_archivo`: excluir la entrega entera del periodo.
> - `tipo_obra_sin_mapear`: asignar el tipo de obra (o corregir el mapa de la fuente).
>
> Criterios de aceptacion:
> - Resolver una critica sin accion correctiva deja de ser posible, o queda marcado como
>   "aceptada tal cual" con rol y nota, y la compuerta lo distingue.
> - Prueba de integracion: duplicado resuelto con exclusion -> el reparto pondera la fila una vez.
> - ADR 0021 actualizado.
>
> Contexto: revision de rosvend a la PR #158, punto 4.

## B. `tipo_obra`, `canal_id` y `rating` sin mapear en las fuentes reales — #165, cerrada

Cerrada por la PR #169: identificar copia `obras.tipo` cuando la fuente no trae el tipo
(`GuardarMatch` y el sembrador), y la ingesta rechaza la fila sin `canal_id` y, en TV,
suscripcion y hotel, la de `rating` cero. Lo que queda para el detector `tipo_obra_sin_mapear`
esta en `diseno.md`, D6.

**Titulo:** `[Backend] tipo_obra/canal_id/rating vacios en los mapas de fuente: el motor aborta o pondera mal`

**Cuerpo (tal como se abrio):**

> Hallazgo de #37 (detector `tipo_obra_sin_mapear`): el mapa de columnas de Caracol deja
> `tipo_obra` vacio a proposito hasta que el cliente conteste P-05, y tampoco trae `canal_id` ni
> `rating`. Desde #120 (`ponderacionTipo`, rama `case "":`) una fila sin `tipo_obra` en una
> modalidad que pondera por tipo **aborta la corrida** con `ErrRepartoInvalido`; sin `canal_id` la
> fila no entra en `UsosDeCanal` y sin `rating` el peso de audiencia queda en cero.
>
> Pedido:
> - Decidir de donde sale cada campo por fuente (mapa de ingesta, catalogo de obras o respuesta
>   del cliente) y registrarlo en `docs/dominio/fichas-fuente.md` y `preguntas-cliente.md`.
> - Poblar `tipo_obra` desde `obras.tipo` cuando la fila ya esta identificada y la fuente no lo trae.
> - Que la ingesta rechace (o marque) la fila sin `canal_id` en vez de dejarla fuera del reparto
>   en silencio.
>
> Criterios de aceptacion:
> - Sobre el seed, `tipo_obra_sin_mapear` = 0 para filas identificadas de Caracol.
> - Prueba que cubra una fila sin `canal_id` y una sin `rating`.
>
> Enlazar desde el godoc de `tipoObraSinMapear` (`internal/dominio/anomalias/detectar.go`) y desde
> la PR #158. Contexto: revision de rosvend a la PR #158, punto 11.
