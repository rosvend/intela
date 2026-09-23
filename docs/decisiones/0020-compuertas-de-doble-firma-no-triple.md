# 0020 Las compuertas de RD 13.5 firman con dos roles, no con los tres del texto

Fecha: 2026-09-21
Estado: Vigente, con deuda declarada

## Contexto

`RD 13.5` nombra tres responsables para las etapas de Verificacion y Pago y
Registro, identicos en los dos circuitos:

> Corresponde a contabilidad, gerencia y tesoreria verificar la liquidacion
> parcial preparada por el area de gerencia y distribucion y autorizar o
> pedir correcciones. Responsable: Contabilidad, Gerencia y Tesoreria.

El esquema de `firmas` (`00001_init.sql`) solo admite dos roles:

```sql
rol TEXT NOT NULL CHECK (rol IN ('distribucion','contabilidad'))
```

y `firmas` tiene PK `(proceso_id, rol, revision)` mas
`firma_actor_unico_por_revision`, que es exactamente el mecanismo de doble
firma -dos roles, dos actores distintos- que la issue #34 original describia
("a gate needs two signatures from distinct reglamento roles").

Mientras tanto, el PR #104 (panel de corridas, abierto antes de que #34
aterrizara) ya escribio su frontend, su navegacion y sus pruebas de contrato
contra ese mismo modelo de dos roles: `distribucion` y `contabilidad` como
las dos firmas de la compuerta.

## Decision

Las compuertas de `verificacion` y `pago_registro` exigen las firmas de
**`distribucion` y `contabilidad`**, no las tres del texto verbatim de
`RD 13.5`. `internal/dominio/reparto/proceso.go` (`RolAcompuerta`) solo
admite esos dos valores, y `firmas.rol` no se migra.

Es una desviacion deliberada del reglamento, no un descuido: se elige por
compatibilidad con el contrato que #104 ya tiene escrito y probado, y porque
ampliar a tres roles es una migracion de `firmas.rol` que no estaba
presupuestada en esta PR. Se declara aqui para que sea explicable en
auditoria -que es justo lo que `RD 16` exige de cualquier cifra o control
del sistema- y no una discrepancia descubierta despues entre lo que el
reglamento dice y lo que el sistema hace.

## Alternativas consideradas

**Migrar `firmas.rol` a los tres roles del texto (`gerencia` incluido) en
esta misma PR.** Descartada por ahora: rompe el contrato que #104 ya escribio
(su navegacion asigna la segunda firma de la compuerta a `contabilidad`
especificamente), y esta PR no puede arreglar #104 sin salirse de su alcance.

**Modelar un tercer rol de aval solo para estas dos compuertas, distinto del
resto de `firmas.rol`.** Descartada por sobreingenieria: el sistema ya tiene
un patron de N avales de rol fijo (`reparto.RolAvalReclamacion`,
`RD 14.5.10-12`), y bifurcar el modelo de firmas del proceso en dos formas
distintas -dos roles aqui, tres alla- para una sola tabla es la clase de
indireccion que `CLAUDE.md` pide evitar cuando el sistema mueve dinero de
terceros.

## Consecuencias

La compuerta de Verificacion y la de Pago y Registro pueden avanzar sin que
Gerencia haya firmado nada, aunque el reglamento la nombra responsable. Es
una brecha de cumplimiento conocida, no un bug oculto.

**Deuda declarada:** subir a los tres roles del texto verbatim -y, aparte,
modelar `RD 13.6.3` (documentacion, distribucion y contabilidad para pagos a
sociedades extranjeras, que hoy no tiene ni etapa en el esquema)- son
trabajo pendiente, coordinado con quien termine #104 para no volver a
divergir del contrato que el frontend consume. Issue de seguimiento: #160.
