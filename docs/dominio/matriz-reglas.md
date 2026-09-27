---
actualizado: 2026-09-15
objetivo: 12
fuente_reglas: docs/dominio/reglas-negocio.md
---

# Matriz regla ↔ artículo ↔ implementación ↔ prueba

Entregable del **Objetivo 12**: cada regla del registro de negocio que aplica al
alcance tiene una fila con numeral del reglamento, módulo responsable, prueba que
la verifica y estado.

Cruzada contra [`reglas-negocio.md`](reglas-negocio.md). Toda regla marcada
**Firme** (o **Resuelto provisional** en tarifas) aparece aquí. Las tarifas
`T-01`…`T-11` quedan en la matriz como referencia de auditoria; el cálculo de
tarifa salió del alcance con **P-08** (`preguntas-cliente.md`).

## Convención de estado

| Estado | Significa |
| ------ | --------- |
| **done** | Regla aplicada en código y cubierta por al menos una prueba nombrada |
| **partial** | Hay modelo, esquema, ADR o andamiaje; falta motor, flujo o prueba dedicada |
| **blocked-on-client-data** | No se puede cerrar una cifra defendible sin un dato o acta del cliente |
| **fuera-de-alcance** | Conservada en el registro; Intela no la ejecuta (P-08: recibe lo cobrado) |

Cuando no hay prueba dedicada, la columna **Prueba** lleva `—`. El revisor del
Objetivo 12 debe confirmar que cada nombre de prueba resuelve a un test real y
pasando (`go test ./...`).

## Resumen

| Estado | Conteo | Reglas |
| ------ | ------ | ------ |
| done | 5 | R-02, R-03, R-04, R-27, R-35 |
| partial | 26 | R-01, R-05, R-08–R-10, R-12–R-26, R-28–R-30, R-32–R-34 |
| blocked-on-client-data | 4 | R-06, R-07, R-11, R-31 |
| fuera-de-alcance | 11 | T-01…T-11 |

## Matriz

| Regla | Numeral | Módulo / paquete | Prueba | Estado |
| ----- | ------- | ---------------- | ------ | ------ |
| R-01 Solo se paga a escritores, personas naturales | `RD 7.1`, `RD 4.5` | `internal/dominio/repertorio`; CHECK/trigger en migraciones; `liquidacion` (andamiaje) | `TestTitularesNaturalesTienenIPI`; `TestLaBaseRechazaUnCoautorConRolNoAutoral` | partial |
| R-02 Aportes no creativos no generan derecho de autor | `RD 7.3.3` | `internal/dominio/repertorio`; `internal/infraestructura/postgres` (ingesta/catálogo) | `TestElCatalogoNoTieneDondeGuardarDinero`; `TestLaTablaDeCoautoresNoTieneColumnaDeDinero`; `TestLaTablaDeUsosNoTieneNingunaColumnaDeImporte` | done |
| R-03 Splits solo de la Declaración de Obra | `RD 7.3.1`, `RD 7.3.4`, `RD 13.1.4` | `internal/dominio/repertorio`; `internal/aplicacion` (declaraciones) | `TestNuevaDeclaracion`; `TestCompletaSoloConCienEIPI`; `TestDeclaracionDeObraCompleta`; `TestDeclaracionesCubrenLosTresCasos` | done |
| R-04 Sin 100% declarado no hay reparto parcial | `RD 13.1.3` | `internal/dominio/repertorio`; postgres repertorio; vocabulario `reparto` | `TestCompletaSoloConCienEIPI`; `TestPartesNegativasNoSonCompletas`; `TestNuevaDeclaracion`; `TestListarObrasDerivaElEstadoDeLaDeclaracion`; `TestDeclaracionDeObraIncompletaRoundTrip` | done |
| R-05 Buena fe y no intervención en disputas | `RD 13.1` (párrafo) | Política en ADR 0007 / matching; sin módulo de disputas | — | partial |
| R-06 Deducciones legales antes del reparto | `RD 3`; Ley 44/1993 Art. 21 | Parámetros sembrados (`deduccion.*`); motor en `reparto` pendiente | `TestParametrosSinteticosVanEtiquetados` | blocked-on-client-data |
| R-07 Reserva por errores técnicos (≤5% nacional) | `RD 14.1`, `RD 14.5.1`, `RD 14.5.2`, `RD 14.5.4` | Parámetros sembrados (`reserva.errores_tecnicos`); circuito nacional | `TestParametrosSinteticosVanEtiquetados` | blocked-on-client-data |
| R-08 Destino del remanente de la reserva | `RD 14.4` | Previsto en `prescripcion` / `reparto` | — | partial |
| R-09 Frecuencia mínima de distribución | `RD 12` | `calendario`; `cmd/scheduler`; ADR 0004 | — | partial |
| R-10 Liquidación aceptada por silencio (15 días) | `RD 13.2` | Esquema `notificaciones`; reloj de dominio | — | partial |
| R-11 Distribuciones de menor cuantía (2% SMMLV) | `RD 13.3` | ADR 0004 (SMMLV); `liquidacion` (andamiaje) | — | blocked-on-client-data |
| R-12 Documentos para cobrar (RUT, cert. bancaria) | `RD 13.1.6` | Skill afiliación; sin dominio de documentos aún | — | partial |
| R-13 Conservación de registros (≥10 años) | `RD 13.2`, `RD 13.4` | Bitácora append-only (ADR 0006) | — | partial |
| R-14 Reporte de la sociedad hermana manda | `RD 7.4` | Vocabulario `reparto` internacional; ADR 0008 | — | partial |
| R-15 Pago a sociedades extranjeras solo con contrato vigente | `RD 13.6.1` | ADR 0008 / proceso; sin chequeo de contrato | — | partial |
| R-16 Fees in Error se devuelven íntegros | `RD 13.7` | Etapa `fees_in_error`; CHECK en migración | — | partial |
| R-17 Trato nacional | `RD 4.2`, `RD 13.5` | Principio; afiliación (andamiaje) | — | partial |
| R-18 Publicación del listado ONI (sin montos) | `RD 13.8.1`–`RD 13.8.4` | Vista `oni_publico`; identificación | `TestGuardarMatchExcluidoNoEsONI`; `TestResolverUsosIntegracionCriterio4Repertorio` | partial |
| R-19 Prescripción ONI: 3 años | `RD 13.8.7`, `RD 15.2` | `internal/dominio/prescripcion` (andamiaje) | — | partial |
| R-20 Prescripción general: 10 años | `RD 15.1` | `prescripcion` + `notificaciones` | — | partial |
| R-21 Notificación válida (correo o portal) | `RD 13.8.8` | Esquema `notificaciones.via` | — | partial |
| R-22 Plazo de respuesta a reclamos: 15 días hábiles | `RD 14.3` | Tabla / paquete `reclamaciones` | — | partial |
| R-23 No se atienden reclamos por errores de declaración | `RD 14.5.6` | `reclamaciones` (andamiaje) | — | partial |
| R-24 Solo reclama quien estaba afiliado en el periodo | `RD 14.5.5`, `RD 14.5.8` | `afiliacion` (andamiaje); P-11 | — | partial |
| R-25 Autores de sociedades hermanas reclaman por su sociedad | `RD 14.5.7` | `reclamaciones` (andamiaje) | — | partial |
| R-26 Excepciones: no es comunicación pública | `RD 8.2`, `RT 1` | Documentado en skill tarifas; sin filtro de dominio | — | partial |
| R-27 Se excluyen canales/programas fuera de repertorio | `RD 9.5` | `internal/dominio/identificacion`; `internal/aplicacion` | `TestResolverUsosExcluyeSinSondearYGuardaLaExclusion`; `TestGuardarMatchExcluidoNoEsONI`; `TestResolverUsosIntegracionCriterio4Repertorio` | done |
| R-28 Exclusividad de sociedad | `RS 1`, `RS 4.1` | `internal/dominio/afiliacion` (andamiaje) | — | partial |
| R-29 Requisito mínimo para socio activo | `RS 4.1` | Clase `titulares`; afiliación (andamiaje) | — | partial |
| R-30 Solo los Socios pueden pedir anticipo | `RA 2.2` | Tabla / `internal/dominio/anticipos` (andamiaje) | — | partial |
| R-31 Topes del anticipo (25% / 5 SMMLV) | `RA 3.1.g`, `RA 3.1.h` | `anticipos`; requiere SMMLV | — | blocked-on-client-data |
| R-32 Un solo anticipo vigente | `RA 3.1.c`–`e` | `anticipos` (andamiaje) | — | partial |
| R-33 Descuento automático del anticipo | `RA 2.1` | `anticipos` + `liquidacion` (andamiaje) | — | partial |
| R-34 Fechas de corte de rendimientos | `RD 10.1`, `RD 10.2.1` | `internal/dominio/recaudo` (circuitos / periodos) | `TestNuevaBolsaRechaza`; `TestCircuitosSonLosDosDelReglamento` | partial |
| R-35 Inversiones nacional e internacional separadas | `RD 10.3` | `internal/dominio/recaudo`; UNIQUE bolsa (usuario, periodo, circuito) | `TestCircuitosSonLosDosDelReglamento`; `TestNuevaBolsaRechaza`; `TestNacionalEInternacionalDelMismoPeriodoConviven` | done |
| T-01 Televisión abierta y cerrada: 4% | `RT 3.1.1`, `RT 3.1.2` | `CategoriaUsuario` en `recaudo` (etiqueta) | `TestNuevoUsuarioAceptaTodasLasCategorias` | fuera-de-alcance |
| T-02 Salas de cine: 4% sobre 50% taquilla | `RT 4` (P-01 provisional) | Idem (`cine`) | `TestNuevoUsuarioAceptaTodasLasCategorias` | fuera-de-alcance |
| T-03 Transporte aéreo | `RT 3.3` | Idem (`transporte_aereo`) | `TestNuevoUsuarioAceptaTodasLasCategorias` | fuera-de-alcance |
| T-04 Transporte terrestre | `RT 3.4.1` | Idem (`transporte_terrestre`) | `TestNuevoUsuarioAceptaTodasLasCategorias` | fuera-de-alcance |
| T-05 Transporte fluvial (USD / TRM) | `RT 3.4.2` (P-09 provisional) | Idem (`transporte_fluvial`) | `TestNuevoUsuarioAceptaTodasLasCategorias` | fuera-de-alcance |
| T-06 Hoteles | `RT 3.5` (P-02 provisional) | Idem (`hotel`) | `TestNuevoUsuarioAceptaTodasLasCategorias` | fuera-de-alcance |
| T-07 Establecimientos de salud | `RT 3.5` | Idem (`salud`) | `TestNuevoUsuarioAceptaTodasLasCategorias` | fuera-de-alcance |
| T-08 Medios digitales: 4% | `RT 3.6` | Idem (`medios_digitales`) | `TestNuevoUsuarioAceptaTodasLasCategorias` | fuera-de-alcance |
| T-09 Otros establecimientos | `RT 3.7` | Idem (`otros_establecimientos`) | `TestNuevoUsuarioAceptaTodasLasCategorias` | fuera-de-alcance |
| T-10 Recargo por incumplimiento: 50% | `RT 3.1.1`–`RT 3.7` | No modelado | — | fuera-de-alcance |
| T-11 Tarifas como marco de negociación | `RT 1`; Decreto 1066 Art. 2.6.1.2.5 | Procedencia de bolsa (`convenio`/`tarifa`/`factura`) | `TestCadaBolsaTieneProcedencia` | fuera-de-alcance |

## Notas de alcance (auditor)

1. **P-08 / P-10.** Intela recibe el recaudo ya cobrado; no liquida tarifas (`T-*`).
   Las tasas de deducción/reserva aprobadas por Asamblea (`R-06`, `R-07`) siguen
   abiertas: el seed usa techos marcados `sintetico` y **ninguna cifra del demo
   es citable en auditoría** hasta que exista el acta.
2. **R-04 done vs motor de reparto.** El estado `declaracion_incompleta` y la
   prohibición de prorratear están en dominio y tests; retener/liberar en el
   motor de reparto sigue el ADR 0008 / paquete `reparto`.
3. **R-01 partial.** Catálogo, roles autorales e IPI de persona natural están
   acotados; la orden de pago en `liquidacion` aún es andamiaje.
4. **R-27 done** sobre la cascada y exclusión ≠ ONI. El listado productivo de
   fuentes fuera de repertorio depende de confirmación con REDES (P-05).

## Cómo verificar las pruebas

```bash
# Ejemplos puntuales citados en la matriz
go test ./internal/dominio/repertorio/ -run 'TestCompletaSoloConCienEIPI|TestNuevaDeclaracion|TestElCatalogoNoTiene'
go test ./internal/dominio/recaudo/ -run 'TestCircuitos|TestNuevaBolsa|TestNuevoUsuario'
go test ./internal/aplicacion/ -run 'TestResolverUsosExcluye'
go test ./internal/infraestructura/semilla/ -run 'TestParametrosSinteticos|TestTitularesNaturales|TestCadaBolsaTieneProcedencia|TestDeclaracionesCubren'
```

## Mantenimiento

Al cerrar una regla: actualizar **Prueba** y **Estado** en la misma PR que
introduce el test. No dejar la matriz por detrás del código. Si una regla sale
del alcance (como `T-*` con P-08), marcar `fuera-de-alcance` y citar la
pregunta o ADR; no borrar la fila.
