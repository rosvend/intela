<!--
  Antes de abrir:

  1. Nombre de rama. El check `Branch naming convention` exige
     ^(feature|fix|hotfix|docs|chore|refactor)/[a-z0-9._/-]+$
     Ojo: el prefijo es `feature`, no `feat`.
     Si no cumple, el PR no se puede mergear. Renombrar:
       git branch -m feature/mi-cambio
       git push origin -u feature/mi-cambio
       git push origin --delete <rama-vieja>

  2. Rama al dia. `main` exige que la rama este sincronizada antes del merge:
       git fetch origin && git merge origin/main    (o rebase)

  3. `ci` en verde y una aprobacion. Son obligatorios, no sugerencias.

  Borra los comentarios que no apliquen. Lo que no se conteste, se pregunta en
  la revision, asi que sale mas barato contestarlo aqui.
-->

## Resumen

<!-- Que cambia y por que. Si hay una decision de diseno detras, enlaza el ADR
     de docs/decisiones/. Si la decision es nueva, escribe el ADR: no se
     decide en la descripcion de un PR. -->

## Tipo de cambio

- [ ] `feature` — funcionalidad nueva
- [ ] `fix` — correccion de un defecto
- [ ] `hotfix` — correccion urgente sobre `main`
- [ ] `docs` — documentacion, ADR, reglamentos, base de conocimiento
- [ ] `chore` — tooling, CI, dependencias
- [ ] `refactor` — cambio interno sin cambio de comportamiento

<!-- Marca tambien si aplica: -->

- [ ] Cambia una **cifra o regla normativa** (requiere cita al reglamento)
- [ ] Cambia el **esquema de datos** (requiere migracion)
- [ ] Cambia un **contrato de API** (requiere regenerar los tipos del frontend)

## Cumplimiento de Clean Architecture

<!-- ADR 0002 y 0003 dan por asumido un riesgo: que la frontera se erosione
     sola y que nadie lo note hasta la auditoria. `ci` verifica lo mecanico;
     esta lista cubre lo que ninguna herramienta ve. Marcar "N/A" es una
     respuesta valida en un PR de solo documentacion. -->

**Regla de dependencia** — ADR [0002](../docs/decisiones/0002-arquitectura-hexagonal.md)

- [ ] Ningun tipo de `internal/dominio/` ni de `internal/aplicacion/` nombra infraestructura
- [ ] Lo que cruza la frontera cruza por un puerto; los adaptadores nuevos **implementan** un puerto existente en vez de anadir una entrada al nucleo
- [ ] El tiempo entra por `PuertoReloj`. No hay `time.Now()` en el nucleo (ADR [0005](../docs/decisiones/0005-reparto-determinista-y-reproducible.md))

**Fronteras de modulo** — ADR [0003](../docs/decisiones/0003-monolito-modular.md)

- [ ] Ninguna dependencia entre modulos que no este en la pagina 2 de `docs/diagrams/PATIC2 - Arquitectura.drawio`
- [ ] Si la dependencia entre modulos cambio, **el diagrama se actualizo en este mismo PR**. Ya nada lo comprueba automaticamente (ADR [0012](../docs/decisiones/0012-la-frontera-se-verifica-sobre-el-codigo.md)): si no se actualiza aqui, miente
- [ ] Ningun modulo escribe en la trazabilidad de otro (ADR [0006](../docs/decisiones/0006-trazabilidad-como-asiento-append-only.md))

**Los cuatro invariantes del dominio** — los cuatro producen codigo plausible y equivocado, y el error no se manifiesta como excepcion: produce un numero, el numero se paga, y aparece en una auditoria de `RD 16` anos despues.

- [ ] No se suman importes por fila. `ReporteDeUso` sigue sin campo de dinero: los reportes **ponderan** la bolsa, no la aportan
- [ ] Ningun camino de tipos lleva de una parrilla a un porcentaje de pago. Las columnas `Autor*` y `Guionista*` son evidencia de matching, jamas insumo de reparto (`R-02`, `R-03`)
- [ ] Si lo declarado no suma 100%, se retiene el **total** en reserva. No hay reparto parcial (`R-04`, `RD 13.1.3`)
- [ ] No existe firma que emita orden de pago a quien no sea escritor persona natural (`R-01`, `RD 4.5`)

**Trazabilidad** — `RD 16`

- [ ] Toda cifra nueva es explicable hasta su origen: fuente, reporte y regla aplicada
- [ ] Todo parametro normativo nuevo entra como **dato con vigencia y organo aprobador**, no como constante en el codigo (ADR [0004](../docs/decisiones/0004-parametros-normativos-como-dato.md))

## Evidencia de pruebas

<!-- Que se corrio y que salio. Pega la salida, no la resumas: "pasa todo" no
     es evidencia. -->

```
# p. ej.
go test -race -count=1 ./...
```

- [ ] `ci` en verde en este PR
- [ ] Pruebas nuevas para el comportamiento nuevo (o justificacion de por que no aplican)

<!-- Si el PR toca el motor de reparto, ADR 0005 exige mas que pruebas de
     unidad. Marca lo que aplique: -->

- [ ] Reproducibilidad: la misma entrada produce el mismo resultado bit a bit
- [ ] Los ejemplos numericos del propio reglamento se usan como caso de prueba
- [ ] Aritmetica decimal exacta: ningun `float` en un camino de dinero

## Issues vinculados

<!-- `Closes #12` cierra el issue al mergear. `Refs #12` solo enlaza. -->

Closes #

## Riesgos y notas para quien revise

<!-- Que mirar con lupa, que quedo fuera de alcance, que hay que hacer despues.
     Un "nada" explicito tambien sirve. -->
