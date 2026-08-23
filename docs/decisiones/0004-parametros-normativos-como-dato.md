# 0004 Los parametros normativos son dato versionado, no constantes

Fecha: 2026-08-10
Estado: Vigente

## Contexto

El calculo del reparto esta lleno de cifras que parecen constantes y no lo son. Cada una tiene un
organo que la aprueba, una vigencia, y una historia:

| Cifra | Quien la fija | Fuente |
| ----- | ------------- | ------ |
| Deducciones legales: hasta 20% administrativo, hasta 10% social | Ley y Asamblea General | `R-06`, Ley 44/1993 Art. 21 |
| Reserva por errores tecnicos: hasta 5% del recaudo nacional | **Asamblea General** | `R-07`, `RD 14.5.1` |
| Ponderacion por tipo de obra: 5.0 / 2.8 / 1.3 / 0.8 | Reglamento de Distribucion | `RD 9.1.1` |
| Reparto entre grupos de canales: 50/20/10/10/10 | Reglamento de Distribucion | `RD 9.5` |
| Coeficientes `Wa`, `Wb`, `Wc` de la formula OTT | **sin publicar todavia** | `RD 9.7` |
| SMMLV del ano, para el umbral de minima cuantia | Gobierno Nacional | `R-11`, `RD 13.3` |
| TRM, para la tarifa fluvial en dolares | Banco de la Republica | `T-05`, `RT 3.4.2` |
| Tarifas por categoria de usuario | Consejo Directivo, version aprobada el 30/06/2026 | `RT 5` |
| Tarifa efectivamente pactada con cada usuario | negociacion con el usuario | `T-11`, `RT 1` |
| Fechas del proceso de distribucion | Consejo Directivo, modificables por fuerza mayor | `RD 12` |
| Rating por franja horaria | proveedor especializado, anual | `RD 9.1.1.d` |

Dos hechos deciden esta ADR. El primero: `docs/reglamentos/README.md` deja escrito que cuando llega
una version nueva de un reglamento hay que conservar el PDF anterior, **porque los repartos
historicos se calcularon con la version vigente en su momento**. El segundo: `T-11` dice que las
tarifas publicadas son un marco de negociacion y que lo que se cobra puede diferir, asi que ni
siquiera hay una tarifa por categoria: hay una por convenio.

Si estas cifras viven en el codigo, cambiar el porcentaje de reserva que aprobo la Asamblea es un
despliegue, recalcular un reparto de hace cinco anos es imposible sin revertir commits, y explicarle
a un auditor de la DNDA de donde salio un numero obliga a leer historia de git.

## Decision

**Todo parametro normativo es una fila con vigencia y organo aprobador, leida por
`PuertoParametrosNormativos`. Ninguno es una constante en el codigo.**

Forma minima de cada parametro: clave, valor, vigencia desde, vigencia hasta, organo que lo aprobo,
acto que lo aprobo, y version del reglamento a la que corresponde.

De ahi salen tres consecuencias que son parte de la decision:

**Una corrida de reparto fija un snapshot inmutable.** Al abrir el proceso se resuelven todos los
parametros a su valor vigente en la fecha del periodo, y ese conjunto queda congelado y referenciado
por la corrida. El motor de calculo no consulta parametros: los recibe. Es lo que hace posible
`0005-reparto-determinista-y-reproducible.md`.

**La tarifa pactada vence a la tarifa publicada.** El sistema guarda el valor acordado por convenio
con cada usuario. La tabla del `RT` es el valor por defecto cuando no hay convenio, no la verdad.

**El calendario de distribucion es dato.** El Consejo Directivo fija los rangos y puede modificarlos
por fuerza mayor con re-notificacion (`RD 12`). El planificador consulta el calendario del dominio;
no es dueno de las fechas. Los trabajos con reloj propio y fijo por reglamento (reserva semestral
`RD 14.5.2`, cortes de rendimientos el 20 de octubre y el 30 de septiembre `RD 10.1` y `RD 10.2.1`,
padron de socios en marzo `RS 5.2`) tambien entran como parametros, no como expresiones cron
escritas a mano.

Los parametros que faltan hoy (`Wa`, `Wb`, `Wc`, la tarifa hotelera del rango sin cubrir de `T-06`,
la base de calculo de cine de `T-02`) se modelan como **ausentes**, no como cero ni como un valor
inventado. Un reparto que los necesite y no los encuentre falla ruidosamente en vez de producir una
cifra falsa.

## Alternativas consideradas

**Constantes en el codigo.** Es lo mas simple y lo que se hace por defecto. Descartada: cada cambio
aprobado por la Asamblea seria un despliegue, y recalcular un periodo antiguo exigiria reconstruir
el binario de aquella epoca.

**Archivo de configuracion versionado en git** (YAML o TOML por ano). Mejor que constantes, y
tentadora porque encaja con la decision `0001` de tener el conocimiento en el repositorio. Descartada
para estos datos concretos: el SMMLV y la TRM cambian por decision externa, las tarifas pactadas son
una por usuario y las aprueba el Consejo Directivo, y ninguna de esas cosas puede depender de que
alguien haga un commit. La configuracion en git sirve para el comportamiento del sistema, no para
cifras que un organo social aprueba en acta.

**Tabla plana sin vigencia**, actualizada en sitio. Descartada de plano: destruye la capacidad de
recalcular un reparto historico, que es justamente el requisito que origina esta ADR.

**Guardar solo el valor y deducir la vigencia de la fecha del registro.** Descartada: mezcla cuando
se escribio el dato con desde cuando rige, y son cosas distintas. La Asamblea puede aprobar en marzo
un porcentaje que rige desde enero.

## Consecuencias

Positivas: cambiar un porcentaje aprobado por la Asamblea es cargar una fila. Recalcular un reparto
de hace anos es leer su snapshot. Y cuando un auditor pregunta por un numero, la respuesta incluye
quien aprobo cada cifra que entro en el, que es lo que pide `RD 16`.

A cambio: mas tablas, mas pantallas de administracion y una capa de resolucion de vigencias que hay
que probar bien. Ninguna cifra se puede escribir en linea, ni siquiera el 100% de un split, sin
preguntarse si es normativa.

Riesgo asumido: la tentacion de escribir un numero directo cuando corre prisa. Una ponderacion de
`RD 9.1.1` incrustada en el motor no rompe nada visible hoy y hace irreproducible el reparto dentro
de cinco anos. La mitigacion es que el motor de calculo reciba los parametros como argumento y no
tenga acceso al puerto: si el valor no viene en el snapshot, el codigo no compila.
