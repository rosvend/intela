---
actualizado: 2026-08-10
fuente: Reglamento de Distribucion IX, seccion 9
---

# Modelos de calculo del reparto

## El modelo mental correcto

El dinero **no llega por fila**. Ningun reporte de uso trae importes. El flujo es:

1. REDES SGC cobra a cada usuario segun el Reglamento de Tarifas. Eso produce una **bolsa**
   por usuario y por periodo.
2. Se aplican las Deducciones Legales (`R-06`) y las reservas (`R-07`).
3. Los reportes de uso sirven para **ponderar** esa bolsa entre las obras comunicadas.
4. El importe de cada obra se reparte entre sus autores segun los porcentajes de la
   Declaracion de Obra (`R-03`).

Es un asignador de bolsa, no un atribuidor de ingresos transaccionales. Sumar una columna de
dinero por obra es el modelo equivocado y no hay datos para hacerlo.

## 9.1 Television abierta y radiodifundida

Los recaudos de cada canal se distribuyen **de forma independiente por canal**, conforme al
pago que ese canal haya hecho en el periodo, una vez practicadas las Deducciones Legales.

### Valorizacion de una obra

```
Puntos obra = Ponderacion(tipo de obra) * Duracion * Rating(franja horaria)
```

### Tabla de ponderacion por tipo de obra

| Tipo de obra    | Ponderacion |
| --------------- | ----------- |
| Cinematografica | 5.0         |
| Unitario        | 2.8         |
| Serie / Telenovela | 1.3      |
| Sketches        | 0.8         |

Las obras cinematograficas reciben mas puntos por minuto por su naturaleza no episodica,
para igualarlas frente a series y telenovelas que se emiten durante periodos extensos.
Los puntajes salen de promediar la Encuesta Socioeconomica de Bienestar Social de 2020 con
los reportes anuales del proveedor especializado de parrillas.

### Duracion

- Es el tiempo estimado en minutos durante el cual la obra es emitida.
- **La hora de emision televisiva se computa como 48 minutos**, salvo prueba en contrario.
- **La duracion artistica es el 80% de la reportada por el proveedor especializado.**
- No computan los avances publicitarios de obras audiovisuales emitidos para promocionar la
  programacion propia del canal.

### Rating por franja horaria

Lo suministra un proveedor especializado contratado por REDES SGC. Se actualiza anualmente.
**No viene en los reportes de parrilla**: es una tercera fuente de datos.

### Conversion a dinero

```
Valor Punto  = Total reparto del canal / Total puntos del canal
Reparto obra = Puntos obra * Valor Punto
```

### Ejemplo del reglamento

Canal Z, $1.000.000 disponibles tras deducciones.

| Programa   | Emisiones | Ponderacion | Duracion | Rating | Total puntos |
| ---------- | --------- | ----------- | -------- | ------ | ------------ |
| Pelicula X | 1         | 5.0         | 70       | 4.5    | 1.575        |
| Serie Y    | 10        | 1.3         | 48.0     | 9.0    | 5.616        |
| **Total**  |           |             |          |        | **7.191**    |

Valor Punto = 1.000.000 / 7.191 = 139,1

- Pelicula X = 1.575 * 139,1 = $219.024
- Serie Y = 5.616 * 139,1 = $780.976

Nota de lectura: en el ejemplo, `Total puntos` de la Serie Y incorpora las 10 emisiones
(1,3 * 48 * 9 = 561,6 por emision, * 10 = 5.616). Las emisiones multiplican, aunque la
formula escrita en `RD 9.1.1` no incluye el factor de emisiones de forma explicita.

## 9.2 Exhibidores cinematograficos y salas de cine

Reparto proporcional a los **ingresos de taquilla** de cada obra en el ejercicio, con base en
la informacion suministrada por el usuario.

Ejemplo del reglamento: bolsa de $1.000.000, Pelicula X con 10.000 espectadores (67%) recibe
$666.667 y Pelicula Y con 5.000 (33%) recibe $333.333.

## 9.3 Teatros

Mismo criterio que salas de cine: proporcional a ingresos de taquilla por obra.

## 9.4 Medios de transporte publico

REDES SGC recauda los datos de las obras exhibidas y el **numero de exhibiciones** de cada
una, solicitandolos a las companias de distribucion de obras a medios de transporte y a las
empresas de transporte. Si hay varias bases de datos, se gestionan informaticamente para
obtener una unica fuente sobre la cual articular el reparto.

## 9.5 Operadores de television por suscripcion

Primero se reparte el recaudo del usuario entre **grupos de canales**:

| Grupo                                        | Porcentaje del recaudo |
| -------------------------------------------- | ---------------------- |
| Canales privados nacionales                   | 50%                    |
| Canales regionales, locales y de operacion publica | 20%               |
| Premium                                       | 10%                    |
| Lideres en rating                             | 10%                    |
| Estandar                                      | 10%                    |

Definiciones de los grupos en `RD 9.5.1` a `RD 9.5.5`. *Lideres en rating* son canales
cerrados fuera de los tres primeros grupos que esten en el **quintil con mas audiencia** del
ano inmediatamente anterior.

Dentro de cada grupo, el porcentaje se distribuye entre las obras aplicando **la misma formula
de 9.1.1**.

Se excluyen los canales que no transmitan contenido del catalogo de REDES SGC (`R-27`).

## 9.6 Establecimientos hoteleros y otros abiertos al publico

Se aplican las disposiciones de 9.5. La cantidad a repartir se distribuye en funcion de la
base de datos con la que se efectue el reparto del mismo ejercicio, referida al derecho de
comunicacion publica en operadores de cable, satelite y redes de telecomunicacion.

## 9.7 Plataformas OTT y nuevas tecnologias

### Formula

```
Pi = PB * Wa + DU * Wb + V * Wc
```

Aviso de procedencia: esta ecuacion es un objeto de ecuacion de Word en el PDF y
`pdftotext` no la extrae, por lo que **no aparece en la version markdown de layer 1**. Esta
transcrita aqui desde la pagina 23 del PDF original. Verificar contra
`docs/reglamentos/fuente/20260527-Version-IX-Reglamento-de-Distribucion-Aprobada-por-el-Consejo.pdf`
antes de implementarla.

| Variable | Significado |
| -------- | ----------- |
| `Pi` | Cantidad de puntos asignados a la obra audiovisual |
| `PB` | Puntaje base para el tipo de obra |
| `DU` | Tiempo en minutos durante los cuales la obra **es vista** |
| `V`  | Cantidad de veces que una obra ha sido vista |
| `Wa`, `Wb`, `Wc` | Ponderaciones definidas para cada variable |

El modelo considera el tipo de obra, la cantidad de visualizaciones y su duracion. El
reglamento tambien menciona el ano de estreno, el numero de capitulos y variables basadas en
metricas de interaccion, sin integrarlos a la formula publicada.

### Coeficientes

Los valores de `Wa`, `Wb` y `Wc` **no estan publicados en el reglamento**. La nota al pie 14
indica que la ponderacion del puntaje base se determina por simulaciones y que da como
resultado una asignacion del **90% para obras seriadas o episodicas y 10% para unitarias o
no episodicas**. Eso describe `PB`, no `Wa`/`Wb`/`Wc`.

Accion: pedir los tres coeficientes al cliente. Sin ellos la formula OTT no es implementable.

### Plataformas de terceros

Para obras puestas a disposicion en plataformas de internet que pertenecen a canales de TV,
operadores de suscripcion o exhibidores, **cuando esa plataforma no es la ventana principal
de difusion del usuario**, la asignacion es el **5% del valor recibido del usuario**, salvo
pacto en contrario. Se distribuye en la misma proporcion que se definio para la obra en el
canal, la retransmision por cable o la exhibicion en sala. El Consejo Directivo puede adoptar
un porcentaje distinto segun el caso.
Fuente: `RD 9.7`

## Reparto entre autores

Comun a todos los modelos anteriores. Una vez determinado el importe de la obra:

```
Importe autor = Importe obra * porcentaje declarado del autor
```

Sujeto a `R-04`: si los porcentajes declarados no suman exactamente 100%, no se reparte nada
de esa obra y el total queda en reserva.

## Cobertura de datos actual

Contraste entre lo que exigen las formulas y lo que traen los archivos de muestra. Detalle en
`docs/dominio/fuentes-datos.md`.

| Formula | Insumo | Disponible en la muestra |
| ------- | ------ | ------------------------ |
| 9.1.1 | Tipo de obra | Parcial. `TIPO` y `SubGenero` no mapean 1:1 a las cuatro categorias |
| 9.1.1 | Duracion | Si, `Duracion_total`, con la transformacion del 80% |
| 9.1.1 | Rating por franja | **No**. Fuente externa ausente |
| 9.1.1 | Emisiones | Si, por conteo de filas por obra y periodo |
| 9.7 | `V` | Si, `stream_starts` |
| 9.7 | `PB` | Parcial, derivable de `content_type` y `genre` con tabla de mapeo |
| 9.7 | `DU` | **No**. Hay `episode_runtime`, que no es tiempo visto |
| 9.7 | `Wa`, `Wb`, `Wc` | **No**. No publicados |
| Todos | Porcentajes de autor | **No**. Requiere export de Declaraciones de Obra |
| Todos | Bolsa a repartir | **No**. Requiere reportes de recaudo por usuario y periodo |
