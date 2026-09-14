---
actualizado: 2026-09-13
evidencia: uv run --script src/scripts/sample.py sobre data/
---

# Fichas de fuente

Una ficha por fuente de datos del sistema, segun el Objetivo 0 del entregable ("levantamiento
del flujo real y de las fuentes"). Cierra la issue #21.

Que hay en cada ficha: nombre, formato, ruta de acceso, periodicidad, dueno manual actual y
estado de perfilado. El **perfil tecnico** de las fuentes ya medidas -columnas, cardinalidades,
trampas de tipo- no se repite aqui: vive en [`fuentes-datos.md`](fuentes-datos.md), y esta ficha
enlaza.

Las preguntas abiertas de cada fuente se rastrean en
[`preguntas-cliente.md`](preguntas-cliente.md).

## Resumen

| Fuente | Formato | Estado | Alimenta |
| ------ | ------- | ------ | -------- |
| Parrilla de television CARACOL | Excel | Perfilada | Puntos TV (`RD 9.1.1`) |
| Reporte OTT Netflix | Excel | Perfilada | Puntos OTT (`RD 9.7`) |
| Padron IPI | Excel `.xls` | **En el repo, sin perfilar** | Titulares, IPI |
| Declaraciones de Obra (REDES-SYS) | Web, sin acceso | **No disponible** | Splits (`R-03`) |
| Feed de rating por franja | Desconocido | **No disponible** | Puntos TV (`RD 9.1.1`) |
| Reportes de recaudo | Desconocido | **No disponible** | La bolsa |

Tres de seis no estan disponibles, y son justo las que producen dinero: los splits, el rating y
el recaudo. Por eso el demo corre sobre fixtures marcadas -- ver [`fixtures.md`](fixtures.md).

## F-01 Parrilla de television CARACOL

| Campo | Valor |
| ----- | ----- |
| Nombre | `CARACOL_REDES-SGC_(COLOMBIA)_20250202.xlsx` |
| Formato | Excel, una hoja, 59 filas x 48 columnas |
| Ruta de acceso | `data/files/` en el repo. Entrega manual del cliente. |
| Periodicidad | Desconocida. La muestra cubre **4 dias** (20241231 a 20250103). |
| Dueno manual | REDES SGC lo recibe del canal. Sin contacto tecnico identificado. |
| Estado | **Perfilada.** `uv run --script src/scripts/sample.py` |

Alimenta `Duracion` y el conteo de emisiones de `RD 9.1.1`. **No trae rating**, que es una
tercera fuente (F-05).

Lo que condiciona el diseno: la granularidad es **la emision, no la obra** -29 `ID_Ficha`
distintos en 59 filas-, asi que hay que agrupar antes de valorizar. Perfil completo, trampas de
tipo y columnas vacias en [`fuentes-datos.md`](fuentes-datos.md).

Preguntas abiertas: P-13 (campos de episodio, hoy vacios al 100%), P-14 (extractos mas grandes).

## F-02 Reporte OTT Netflix

| Campo | Valor |
| ----- | ----- |
| Nombre | `Modulo identificacion de Obras - Parrilla Netflix.xlsx` |
| Formato | Excel, 49 filas x 19 columnas |
| Ruta de acceso | `data/files/` en el repo. Entrega manual del cliente. |
| Periodicidad | Desconocida. `term_end_date` es constante en la muestra. |
| Dueno manual | REDES SGC lo recibe de la plataforma. Sin contacto tecnico identificado. |
| Estado | **Perfilada.** `uv run --script src/scripts/sample.py` |

Alimenta `V` (`stream_starts`) y `DU` de `RD 9.7`.

Lo que condiciona el diseno: la granularidad es **el episodio**, mientras que la obra de REDES es
el show; y `Id_Ntx` **es un contador de fila del export**, no un identificador -se renumera en
cada entrega y no debe persistirse como clave-. La clave de alias es `show_id` (ADR 0018).

Preguntas abiertas: P-12 (`eidr` vacia en 49 de 49, era la columna que habria resuelto el cruce),
P-14.

## F-03 Padron IPI

| Campo | Valor |
| ----- | ----- |
| Nombre | `IPI - form to report members to IPI 01-03-24.xls` |
| Formato | Excel `.xls`. Es el formato con que se reportan miembros al sistema IPI de SUISA. |
| Ruta de acceso | `data/` en el repo (no en `data/files/`). |
| Periodicidad | Desconocida. |
| Dueno manual | REDES SGC. |
| Estado | **En el repo, sin perfilar.** `sample.py` solo recorre `data/files/`. |

Alimenta `titulares` y el IPI de cada parte declarada. Con P-11 resuelta, **el IPI sale de aqui**,
no de la Declaracion de Obra: el formulario de REDES-SYS no tiene campo de IPI.

Accion pendiente: extender `sample.py` para perfilarlo -no escribir un script paralelo- y
determinar que le falta antes de pedirle nada al cliente. Pregunta abierta: P-15.

## F-04 Declaraciones de Obra (REDES-SYS)

| Campo | Valor |
| ----- | ----- |
| Nombre | REDES-SYS, "Registro de Obra" |
| Formato | Aplicativo web ASP.NET. `https://redes.declaraciondeobra.org/declaracionobras/` |
| Ruta de acceso | **Ninguna.** El equipo no tiene credenciales ni export. |
| Periodicidad | Continua: el autor declara cuando quiere. |
| Dueno manual | El propio autor. REDES SGC administra el aplicativo. |
| Estado | **No disponible.** Perfilado solo a partir de una captura del formulario. |

Es la unica fuente valida de los splits (`R-03`). Intela **no se integra con ella**: asume que el
autor ya declaro (P-07).

Lo que se sabe del formulario, verificado sobre `RegistroObraCine.aspx` (REDES-SYS v1.0.0.7):

- Es **una declaracion jurada por autor**, no un formulario que llenen los coautores juntos. El
  100% se arma sumando N declaraciones independientes -- por eso `declaracion_incompleta` (`R-04`)
  es el estado intermedio **normal**, no un error.
- El autor logueado pone su propio porcentaje en `Corresponde (%)`.
- **`Otros autores` es un unico campo de texto libre**, con el formato sugerido
  `Nombre1 Apellido1 (30%), Nombre2 Apellido2 (10%)`. No hay campos estructurados por coautor.
- **No pide IPI** en ningun campo. De ahi P-11.
- **No hay identificador de obra**: el autor escribe el titulo. Tampoco pide `IDA`, `EIDR` ni
  `IMDB`, asi que la declaracion no aporta nada al escalon 2 de la cascada (#28).
- Metadatos que si captura y que solapan con `obras`: titulo original y otros titulos,
  Original/Remake/Adaptacion, directores, actores principales, sala y fecha de estreno,
  productora, paises de rodaje, idiomas, formato (largo/medio/corto), documental o ficcion,
  duracion en minutos, y datos tecnicos de proyeccion y color.
- La URL vista es de **obra cinematografica**. Debe existir un formulario hermano para television
  y series que no se ha visto.

Consecuencia para el matching: una declaracion se ata a una obra del catalogo **por titulo**, y
sus coautores al padron **por nombre**. Es el mismo problema difuso de #32, aplicado tambien a
personas.

## F-05 Feed de rating por franja horaria

| Campo | Valor |
| ----- | ----- |
| Nombre | Sin identificar |
| Formato | Desconocido |
| Ruta de acceso | **Ninguna.** |
| Periodicidad | Anual, segun `formulas.md`. |
| Dueno manual | Proveedor especializado contratado por REDES SGC, sin nombrar. |
| Estado | **No disponible.** |

Bloquea `RD 9.1.1` completo: sin rating no hay puntos de TV, y TV es el grueso del reparto.

Lo unico decidido (P-06): el dato se indexa por **(canal, franja horaria, ano)**. Un rating unico
por franja para todos los canales haria ponderar igual a dos canales con audiencias distintas, y
`RD 9.1.1` calcula el valor punto **por canal**.

Pregunta abierta: P-06 -- quien es el proveedor y en que formato entrega.

## F-06 Reportes de recaudo

| Campo | Valor |
| ----- | ----- |
| Nombre | Sin identificar |
| Formato | Desconocido. **Ningun archivo de la muestra trae importes.** |
| Ruta de acceso | **Ninguna.** |
| Periodicidad | Por periodo de liquidacion. |
| Dueno manual | REDES SGC, area de recaudo. |
| Estado | **No disponible.** |

Es la bolsa: sin esto no hay nada que repartir.

Alcance decidido (P-08): Intela **recibe** lo cobrado por usuario y periodo; no calcula lo que el
usuario debe ni factura. Eso saca el calculo de tarifas del alcance de #27 y deja `T-01` a `T-11`
como documentacion de referencia.

Pregunta abierta: P-08 -- en que formato entrega REDES el recaudo, con un ejemplo real.
