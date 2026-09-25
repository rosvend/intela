---
actualizado: 2026-09-25
estado: respuestas provisionales del equipo, sin confirmar con REDES SGC
---

# Preguntas abiertas al cliente

Registro unico de lo que hace falta preguntarle a REDES SGC, con dueno, estado y respuesta.
Cierra la issue #21.

Antes existian **dos listas distintas** que se contradecian: la de `reglas-negocio.md` (seis
preguntas, incluida la relacion REDES-SYS/AVSYS) y la de `fuentes-datos.md` (nueve items,
incluidos el export de declaraciones y los reportes de recaudo, que no estaban en la otra).
Este archivo las unifica. Las dos originales ahora apuntan aqui.

## Como leer el estado

| Estado | Significa |
| ------ | --------- |
| **Confirmada** | Respondida por REDES SGC. Se puede citar en auditoria. |
| **Provisional** | Respondida por el equipo con la informacion de negocio disponible, **sin confirmar con REDES**. Sirve para desbloquear el codigo; no sirve para defender una cifra. |
| **Abierta** | Sin respuesta. Lo que dependa de ella se modela como ausente (ADR 0004), nunca con un valor por defecto. |

La distincion Provisional/Confirmada es la misma que el sembrador ya hace entre `sintetico(...)`
y `publicado(...)` en `internal/infraestructura/semilla/dataset.go`, y por la misma razon: un
reparto calculado sobre un supuesto del equipo no se puede defender citando un acto que nadie
expidio.

**Todas las respuestas Provisionales de este archivo se dieron el 2026-09-13**, en sesion de
trabajo interna, a la espera de la reunion con REDES. Si una cambia, cambia la regla de negocio
que la cita -- ese es el punto de tener el dominio aislado.

## Tabla

| ID | Pregunta | Dueno | Estado |
| -- | -------- | ----- | ------ |
| P-01 | `T-02` Base de calculo para salas de cine | @rosvend | Provisional |
| P-02 | `T-06` Tarifa hotelera del rango sin cubrir | @rosvend | Provisional |
| P-03 | Relacion entre REDES-SYS, AVSYS e Intela | @rosvend | Provisional |
| P-04 | Coeficientes `Wa`, `Wb`, `Wc` de `RD 9.7` | @rosvend | **Abierta** |
| P-05 | Mapeo de generos de parrilla a tipos de obra | @rosvend | Provisional |
| P-06 | Proveedor y formato del rating por franja | @rosvend | Parcial |
| P-07 | Export de Declaraciones de Obra desde REDES-SYS | @rosvend | Provisional |
| P-08 | Reportes de recaudo por usuario y periodo | @rosvend | Parcial |
| P-09 | `T-05` TRM aplicable a las tarifas en dolares | @rosvend | Provisional |
| P-10 | Tasas de deduccion y reserva aprobadas por Asamblea | @rosvend | **Abierta** |
| P-11 | De donde sale el IPI de cada parte declarada | @rosvend | Provisional |
| P-12 | `eidr` poblado por Netflix, o acceso a IDA | @rosvend | **Abierta** |
| P-13 | Campos de episodio en la parrilla de Caracol | @rosvend | **Abierta** |
| P-14 | Extractos del mismo periodo y de mayor volumen | @rosvend | **Abierta** |
| P-15 | Padron de titulares con IPI poblado y al dia | @rosvend | **Abierta** |
| P-16 | Alcance del 80% artistico de `RD 9.1.1` | @rosvend | **Abierta** |
| P-17 | Proveedor y formato del feed de quintil de audiencia (`RD 9.5.4`) | @rosvend | **Abierta** |
| P-18 | Base de ponderacion cine/teatro: taquilla vs espectadores (`RD 9.2`/`9.3`) | @rosvend | **Abierta** |
| P-19 | Destino del recaudo de un grupo de suscripcion sin obras (`RD 9.5` / chapeau `RD 15`) | @rosvend | **Abierta** |
| P-20 | Quien puebla `usos.canal_id` en produccion (`MapaCaracol`/`MapaNetflix`/`MapaCine` no lo mapean) | @rosvend | **Abierta** |
| P-21 | Clave de registro por fuente para el detector de duplicados (#37): `cine`, `rcn`, `expreso-bolivariano` | @rosvend | Provisional (`cine`) / **Abierta** (`rcn`, transporte) |

## Respuestas

### P-01 `T-02` Base de calculo para salas de cine
Estado: **Provisional**.
Pregunta: `RT 3.2` dice 4% de los ingresos netos de explotacion del exhibidor -taquilla,
publicidad y restauracion-. `RT 4`, la tabla resumen, dice que el 4% se calcula sobre el **50%
del recaudo de taquilla**, porque el otro 50% va al distribuidor. Ninguna referencia a la otra.
Respuesta: aplica **`RT 4`: 4% sobre el 50% de la taquilla**. Es la base mas estrecha y la que
un exhibidor no discute.
Consecuencia: cierra el conflicto de `T-02` en `reglas-negocio.md`. **No desbloquea codigo**: con
P-08 resuelta como esta, Intela no calcula tarifas.

### P-02 `T-06` Tarifa hotelera del rango sin cubrir
Estado: **Provisional**.
Pregunta: en 71 a 100 habitaciones la tabla define Categoria 4 hasta $42.000 y Categoria 5 desde
$160.001, y deja sin tarifa el rango $42.001 a $160.000.
Respuesta: es una **errata**. Categoria 5 arranca en **$42.001**. Lo confirma el tramo *100 en
adelante*, que solo define desde $42.000: el $160.001 es el error tipografico, no un rango real
sin tarifa.
Consecuencia: igual que P-01, cierra el conflicto sin desbloquear codigo.

### P-03 Relacion entre REDES-SYS, AVSYS e Intela
Estado: **Provisional**. El glosario marcaba esta pregunta como la que "condiciona todo el
alcance del proyecto".
Respuesta:
- **REDES-SYS** es el aplicativo de declaracion de obra en linea, en
  `https://redes.declaraciondeobra.org/`. Se verifico contra una captura del formulario real
  (`RegistroObraCine.aspx`, REDES-SYS v1.0.0.7): la miga de pan dice literalmente
  `REDES-SYS > Menu Principal > Registro de Obra`. **Intela no lo reemplaza y no se integra con
  el.** Se asume que el autor ya declaro ahi.
- **AVSYS** es, segun el glosario y `RD 13.5`, donde se carga el neto a repartir, se valoriza
  cada obra por ponderacion y se producen las liquidaciones parcial y final. Eso es exactamente
  el alcance de Intela (#33 y #36), asi que **Intela ocupa el lugar de AVSYS**.
Cautela: esto ultimo es una **inferencia del equipo**, no una frase del cliente. Confirmar.

### P-04 Coeficientes `Wa`, `Wb`, `Wc`
Estado: **Abierta**. No estan publicados en `RD 9.7`; la nota al pie 14 dice que la ponderacion
del puntaje base se determina por simulaciones.
Decision provisional mientras siguen abiertos: el sembrador usa `0.50 / 0.30 / 0.20`, marcados
`sintetico`, sumando 1, con cifras redondas a proposito para que nadie las confunda con un valor
aprobado. El motor de #33 debe lanzar **error tipado** si el coeficiente no esta en el snapshot,
nunca calcular con cero.

### P-05 Mapeo de generos de parrilla a tipos de obra
Estado: **Provisional**. Los `SubGenero` de la parrilla no mapean 1:1 a las cuatro categorias de
`RD 9.1.1`, y varios probablemente no son repertorio (`R-27`).
Respuesta: ver la tabla en [`fixtures.md`](fixtures.md#mapeo-de-generos). En corto: Telenovela y
Drama son `serie`; Magazine, Noticiero, Agro, Entretenimientos y Religioso **no son repertorio**;
la columna `TIPO` da `cinematografica`, `unitario` y `sketches`.
El mas dudoso es **Entretenimientos**: si son shows con libretista de planta, si son repertorio y
hay que asignarles ponderacion. Preguntarlo explicitamente.

### P-06 Proveedor y formato del rating por franja
Estado: **Parcial**. El formato quedo definido; el proveedor no.
Respuesta: el dato se indexa por **(canal, franja horaria, ano)**. Es lo que encaja con
`RD 9.1.1`, que calcula el valor punto **por canal**: un rating unico por franja para todos los
canales haria ponderar igual a dos canales con audiencias muy distintas.
Sigue abierto: **quien es el proveedor**, en que formato entrega y con que periodicidad real
-el glosario dice anual-.

### P-07 Export de Declaraciones de Obra desde REDES-SYS
Estado: **Provisional**.
Respuesta: **no hay export y no hay acceso al sitio.** Intela asume que el autor ya declaro su
parte en REDES-SYS. Por ahora los porcentajes entran a Intela como **fixtures sinteticas**; el
camino real -digitacion por el staff, o un Excel periodico- se decide con el cliente.
Consecuencia: no se abre issue de importador. El endpoint de #23 detras del rol `administrador`
queda como unica entrada de escritura.

### P-08 Reportes de recaudo por usuario y periodo
Estado: **Parcial**. El alcance quedo definido; el archivo no.
Respuesta: **Intela solo recibe lo cobrado**, por usuario y periodo. No calcula lo que cada
usuario debe y no factura.
Consecuencia: **#27 se reduce.** Su titulo dice "Usuario/Convenio/Tarifa/Recaudo + tariff calc
per user category", y el calculo de tarifa deja de estar en alcance. La tabla `T-01` a `T-11`
pasa a ser documentacion de referencia, no logica de dominio.
Sigue abierto: en que formato entrega REDES el recaudo cobrado. Ningun archivo de muestra trae
importes.

### P-09 `T-05` TRM aplicable
Estado: **Provisional**.
Respuesta: **TRM de la fecha de facturacion**, fuente Banco de la Republica.
Nota: es la opcion menos reproducible de las tres consideradas -reliquidar anos despues obliga a
conservar la TRM usada, no a recalcularla-. Con P-08 como esta, hoy no afecta a ningun calculo:
Intela recibe el importe ya convertido.

### P-10 Tasas de deduccion y reserva aprobadas por Asamblea
Estado: **Abierta**. `R-06` y `R-07` fijan **techos** -hasta 20% administrativo, hasta 10%
social, hasta 5% de reserva-, no tasas. La tasa la aprueba la Asamblea General (`RD 14.5.1`).
Decision provisional: el sembrador usa los techos como si fueran las tasas, marcados
`sintetico`. El demo calcula, pero **ninguna cifra que salga de ahi se puede citar en
auditoria**: se estaria invocando un acta de Asamblea que no existe.
Lo que hace falta: el acta con la tasa vigente, su fecha y el organo que la aprobo.

### P-11 De donde sale el IPI de cada parte declarada
Estado: **Provisional**.
Pregunta: el formulario de REDES-SYS **no pide IPI en ningun campo**, pero el constructor de
`Declaracion` lo exige en cada parte.
Respuesta: el IPI **viene del padron, por `titular_id`**, no de la declaracion. Encaja con `R-24`
-solo reclama quien estaba afiliado en el periodo- y evita que el mismo dato viva en dos tablas y
se desincronice.
Consecuencia: **afecta a la PR #106.** Exigir IPI en la entrada es incorrecto: el documento
fuente no lo tiene. Si conviene conservar una instantanea del IPI para reproducibilidad
(ADR 0005) es una decision aparte.

### P-12 a P-19
Abiertas, sin decision provisional, tomadas de `fuentes-datos.md`, del cableado de #26 y del alcance de #120:
- **P-12** `eidr` poblado por Netflix, o acceso a IDA. Sin uno de los dos, el escalon 2 de la
  cascada (#28) solo funciona sobre lo que ya tenga el catalogo.
- **P-13** Campos de episodio en la parrilla de Caracol, para identificar capitulos de series.
- **P-14** Extractos del mismo periodo en ambas fuentes y de mayor volumen. Hoy no se pueden
  cruzar de verdad: cero coincidencias de titulo entre las dos muestras.
- **P-15** Padron de titulares con IPI poblado y al dia. `data/IPI - form to report members to
  IPI 01-03-24.xls` ya esta en el repo pero **no esta perfilado**.
- **P-16** Alcance del 80% artistico de `RD 9.1.1`: ¿aplica a los minutos de la parrilla del
  canal o solo a la cifra del proveedor especializado de audiencia? El codigo hoy asume lo
  primero para toda fila de TV/hotel sin `unidad_duracion`. Si es lo segundo, hay que dejar de
  transformar la parrilla y esperar el feed de audiencia.
- **P-17** Proveedor especializado, formato y periodicidad del **feed de quintil de audiencia**
  para clasificar canales cerrados como *lideres en rating* (`RD 9.5.4`). La clasificacion
  usa el ano inmediatamente anterior al periodo que se reparte y queda congelada en
  `canales_clasificacion` para poder reejecutar un periodo pasado (ADR 0005). Sin este feed
  no se puede poblar `9.5.4` de forma defendible; `9.5.5` (estandar) absorberia el resto
  solo por exclusion.
- **P-18** Base de ponderacion de cine/teatro: `RD 9.2` dice "ingresos de taquilla" y el
  ejemplo calcula sobre espectadores; `RD 9.3` remite a ese ejemplo. No es P-01 (base
  tarifaria). Hasta confirmar, el motor lee `Snapshot.BaseCineTeatro`.
- **P-19** Destino del recaudo de un **grupo de suscripcion sin obras** (y de importes
  enteros excluidos por R-27). `RD 9.5` no contempla el caso; `RD 14.5.3` cierra la reserva
  a reclamaciones administrativas; ONI (`RD 13.8`) es autor desconocido. El chapeau de
  `RD 15` ("seran preservados") es el unico anclaje. El motor los deja en
  `Resultado.NoDistribuido` con motivo, no en el residuo de redondeo.
- **P-20** Quien puebla `usos.canal_id` en produccion. Los adaptadores de ingesta reales
  (`MapaCaracol`, `MapaNetflix`, `MapaCine`) no mapean ninguna columna del archivo del
  cliente a esa columna: hoy solo la puebla el sembrador sintetico. Sin una columna del
  cliente que identifique el canal -- o una tabla de correspondencia (fuente, canal) que
  alguien mantenga -- toda fila real de TV llega con `canal_id` vacio, y `RD 9.1` no se
  puede repartir por canal sobre datos de produccion. `aplicacion.Reparto.UsosSinCanal`
  cuenta el hueco mientras la respuesta no llega; no lo cierra.

### P-21 Clave de registro por fuente (detector `duplicado_registro`, #37)

`duplicado_registro` es una anomalia **critica**: bloquea la salida de `deducciones` de toda
corrida del periodo (ADR 0021). Su clave por fuente vive en `aplicacion.clavesDeRegistro`.

- **`cine: {id_pelicula}` es Provisional.** El formato de salas es sintetico
  (`MapaCine`). Con esa clave, dos exhibiciones legitimas de la misma pelicula en dos entregas
  del mismo periodo (dos salas, o dos cortes de taquilla) saldrian como duplicado critico.
  Hay que confirmar con REDES que identifica una fila de cine (sala, funcion, fecha) antes de
  tratar ese detector como defendible para cine.
- **`rcn` y `expreso-bolivariano` no tienen clave, a proposito.** No hay formato del cliente
  para ninguna de las dos: solo existen en el sembrador. Declararles una clave seria inventar el
  criterio de un detector que bloquea el pago. Mientras tanto sus filas cuentan en
  `usos_sin_cotejar` (en el seed, 4 de 10), que es el tamano del punto ciego y viaja en la
  respuesta de `POST /alertas/evaluacion` y en su asiento.

## Agenda para la reunion con REDES

Ordenada por lo que mas desbloquea. Las cuatro primeras son las que hoy impiden producir una
cifra defendible.

1. **Acta de Asamblea con las tasas de deduccion y reserva** (P-10). Sin esto ninguna cifra del
   reparto es citable, por correcto que sea el calculo.
2. **Coeficientes `Wa`, `Wb`, `Wc`** (P-04), o confirmacion de que aun no existen. Si no existen,
   acordar que OTT no reparte en vez de repartir con un supuesto.
3. **Proveedor, formato y periodicidad del rating** (P-06). Bloquea la formula de TV completa,
   que es el grueso del reparto.
4. **Formato del reporte de recaudo cobrado** (P-08), con un ejemplo real de un periodo.
5. **Confirmar P-03**: que Intela ocupa el lugar de AVSYS y que REDES-SYS sigue como esta.
6. **Confirmar P-01, P-02, P-05, P-07, P-09 y P-11**, que hoy van con respuesta del equipo.
7. **Entretenimientos**: es o no repertorio (P-05).
8. Pedir P-12 a P-17: `eidr`/IDA, campos de episodio, extractos mas grandes, padron con IPI,
   alcance del 80% artistico, feed de quintil `RD 9.5.4`.
9. **P-20**: que columna del archivo -- si existe alguna -- identifica el canal que pago,
   por fuente. Sin esto, RD 9.1 no se puede repartir por canal sobre datos reales.
