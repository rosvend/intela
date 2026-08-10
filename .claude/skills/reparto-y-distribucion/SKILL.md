---
name: reparto-y-distribucion
description: Usar al implementar o revisar cualquier calculo de reparto de REDES SGC - valorizacion de obras por puntos, valor punto, deducciones legales, reservas, splits entre coautores, liquidaciones, anticipos, ONI o prescripciones. Tambien al modelar las tablas de recaudo, reparto o pago.
---

# Reparto y distribucion

## Modelo mental

Bolsa, no transacciones. El flujo completo:

```
recaudo del usuario (tarifa concertada)
  - deducciones legales (hasta 20% admin + hasta 10% social)
  - reserva errores tecnicos (hasta 5%, solo recaudo nacional)
  = neto a repartir del usuario/periodo
      -> se reparte entre obras por puntos
          -> se reparte entre autores por porcentaje declarado
```

Nunca sumar una columna de dinero por obra. Ningun reporte de uso trae importes.

## Antes de escribir el calculo

Leer `docs/dominio/formulas.md` completo. Tiene las cuatro formulas (TV, cine y teatro,
suscripcion, OTT), la tabla de ponderacion, el ejemplo numerico del reglamento y la tabla de
que insumos existen hoy y cuales no.

Luego `docs/dominio/reglas-negocio.md` para las reglas que rodean el calculo.

## Errores que hay que evitar

**Repartir parcialmente cuando falta declaracion.** `R-04` / `RD 13.1.3`: si los porcentajes
no suman exactamente 100%, se retiene el **total** de esa obra. No prorratear, no pagar lo
declarado y dejar el resto. Modelar `declaracion_incompleta` como estado, no como excepcion.

**Pagar a quien no es escritor.** `R-01` y `R-02`. Solo autores del guion o libreto, personas
naturales. Los campos de director, actores, conductores y productores de los reportes sirven
para identificar la obra, no para pagar.

**Usar la duracion reportada tal cual.** `RD 9.1.1(c)`: la duracion artistica es el **80%** de
la reportada, y la hora de emision televisiva se computa como **48 minutos**.

**Aplicar la reserva del 5% al recaudo internacional.** `R-07` / `RD 14.5.4`: solo aplica al
nacional.

**Deducir sobre los Fees in Error.** `R-16` / `RD 13.7`: se devuelven integros, sin
deducciones administrativas ni de bienestar social.

**Modificar montos del recaudo internacional.** `R-14` / `RD 7.4`: se distribuye exactamente
como lo discrimina la sociedad hermana.

## Datos que hoy no existen

No inventar valores por defecto para estos. Si el calculo los necesita, dejarlo explicitamente
bloqueado y anotarlo:

- Rating por franja horaria (bloquea TV completo)
- Coeficientes `Wa`, `Wb`, `Wc` (bloquean OTT)
- Porcentajes de la Declaracion de Obra (bloquean todo pago)
- Base de calculo de salas de cine: el reglamento se contradice, ver `T-02`

## Parametros que cambian en el tiempo

Deben ser datos, no constantes en codigo: SMMLV por ano (`R-11`, `R-31`), porcentaje de
reserva aprobado por Asamblea (`R-07`), tarifas por convenio con cada usuario (`T-11`), TRM
para tarifas en dolares (`T-05`), y la version del reglamento vigente en el periodo que se
liquida.

## Trazabilidad

`RD 13` y `RD 16` exigen que toda cifra sea explicable y auditable, con registros conservados
diez anos (`R-13`). Cada importe calculado debe poder responder: de que recaudo salio, que
reporte de uso lo pondero, que version del reglamento y que regla se aplico, y que declaracion
de obra fijo el porcentaje.
