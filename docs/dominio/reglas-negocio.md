---
actualizado: 2026-08-10
fuentes: Reglamento de Distribucion IX, Reglamento de Tarifas VI, Reglamento de Socios, Reglamento de Anticipos
---

# Registro de reglas de negocio

Cada regla es operativa: se puede implementar y se puede verificar. La columna de fuente
apunta a la seccion exacta del reglamento, en `docs/reglamentos/`, para que cualquier cifra
del sistema sea explicable.

Convencion de estado:

- **Firme** — la regla esta escrita sin ambiguedad en el reglamento.
- **Confirmar** — el reglamento la implica pero falta un dato o una decision del cliente.
- **Conflicto** — dos partes del reglamento dicen cosas distintas.

## Titularidad y reparto entre autores

### R-01 Solo se paga a escritores, personas naturales
REDES SGC representa a los autores o derechohabientes **del guion o libreto**. Gestiona
derecho de autor y por tanto representa personas naturales unicamente. Los productores no
pueden ostentar porcentaje alguno.
Estado: Firme. Fuente: `RD 7.1`, `RD 4.5`

### R-02 Los aportes no creativos no generan derecho de autor
Revisiones, comentarios y propuestas de modificacion del guion hechas por productores
ejecutivos, revisores, ejecutivos de cadena, actores o directores de casting **no** se
consideran creacion y no generan derechos, salvo que hayan participado directamente en la
escritura.
Estado: Firme. Fuente: `RD 7.3.3`
Implementacion: los campos `Autor*`, `Guionista*`, `Director*` de los reportes de uso son
**evidencia para identificar la obra**, nunca insumo de pago. Ver `R-03`.

### R-03 Los splits vienen solo de la Declaracion de Obra
El porcentaje de cada autor es el que el propio autor declaro en la Declaracion de Obra en
REDES-SYS, acordado entre coautores antes de diligenciarla. Ningun reporte de usuario ni
contrato de escritura determina el reparto.
Estado: Firme. Fuente: `RD 7.3.1`, `RD 7.3.4`, `RD 13.1.4`

### R-04 Sin 100% declarado no hay reparto parcial
Si al momento del reparto no existe la declaracion discriminada que sume 100%, REDES SGC
retiene **el total** del importe de esa obra en reserva hasta que se declare el 100%. Una vez
completa, entra en el siguiente reparto.
Estado: Firme. Fuente: `RD 13.1.3`
Implementacion: `declaracion_incompleta` es un estado de primera clase de la obra, no un
error. El motor debe poder retener, acumular y liberar. Nunca prorratear el faltante.

### R-05 Buena fe y no intervencion en disputas
REDES SGC presume la buena fe de las declaraciones y solo paga cuando hay acuerdo entre
coautores sobre los porcentajes. Las reclamaciones posteriores se resuelven entre las partes
sin vincular a REDES SGC.
Estado: Firme. Fuente: `RD 13.1` paragrafo

## Deducciones y reservas

### R-06 Deducciones legales antes del reparto
Hasta **20%** para gastos administrativos (hasta 30% durante los dos primeros anos de la
sociedad, beneficio disponible hasta 2019) y hasta **10%** para programas de inversion social.
Estado: Firme. Fuente: `RD 3` (Deducciones Legales), Ley 44 de 1993 Art. 21

### R-07 Reserva por errores tecnicos: hasta 5% del recaudo nacional
Se puede retener hasta 5% del recaudo nacional para corregir errores tecnicos y atender
reclamaciones. El porcentaje lo aprueba la Asamblea General. El monto se extrae
semestralmente. **No aplica al recaudo recibido del extranjero.**
Estado: Firme. Fuente: `RD 14.1`, `RD 14.5.1`, `RD 14.5.2`, `RD 14.5.4`

### R-08 Destino del remanente de la reserva
Cumplidos los terminos de prescripcion, el dinero no usado se distribuye a los autores en la
misma proporcion en que se distribuyo el recaudo del que se tomo la reserva.
Estado: Firme. Fuente: `RD 14.4`

## Ciclo de liquidacion y pago

### R-09 Frecuencia minima de distribucion
Al menos una vez por ano calendario. El Consejo Directivo fija los rangos de fechas.
Estado: Firme. Fuente: `RD 12`

### R-10 Liquidacion aceptada por silencio a los 15 dias calendario
Si el titular no objeta la liquidacion dentro de los quince dias calendario siguientes al
envio, se entiende aceptada.
Estado: Firme. Fuente: `RD 13.2`

### R-11 Distribuciones de menor cuantia
Si el valor a distribuir es igual o menor al **2% de un SMMLV**, se liquida y se informa al
titular. Si no responde en 15 dias indicando que desea el pago, el monto se acumula al
siguiente periodo de distribucion, siempre que alli supere ese 2%.
Estado: Firme. Fuente: `RD 13.3`
Implementacion: requiere el SMMLV vigente por ano como dato parametrizado, y saldos que se
arrastran entre periodos.

### R-12 Documentos que el titular debe aportar para cobrar
RUT actualizado y certificacion bancaria. Si el pago va a un tercero, autorizacion escrita y
firmada mas la certificacion bancaria de ese tercero.
Estado: Firme. Fuente: `RD 13.1.6`

### R-13 Conservacion de registros: minimo 10 anos
Registros y evidencias de transferencias y pagos, y la documentacion de recaudo y
liquidacion, se conservan al menos diez anos.
Estado: Firme. Fuente: `RD 13.2`, `RD 13.4`, Codigo de Comercio Art. 60

## Recaudo internacional

### R-14 El reporte de la sociedad hermana manda
Cuando el pago viene acompanado de reporte, se distribuye en la proporcion que la sociedad
hermana discrimina. Los montos **no** pueden modificarse, asignarse a otra persona ni
alterarse para beneficiar a otro autor.
Estado: Firme. Fuente: `RD 7.4`

### R-15 Pago a sociedades extranjeras solo con contrato vigente
Estado: Firme. Fuente: `RD 13.6.1`

### R-16 Fees in Error se devuelven integros
En el menor tiempo posible, **sin deducciones administrativas ni de bienestar social**, con
soporte que indique obras y montos reintegrados.
Estado: Firme. Fuente: `RD 13.7`

### R-17 Trato nacional
Los titulares extranjeros reciben el mismo trato que los nacionales.
Estado: Firme. Fuente: `RD 4.2`, `RD 13.5`

## Obras no identificadas y prescripcion

### R-18 Publicacion del listado ONI
Se publica en la web de REDES SGC con titulos e informacion identificatoria, **sin indicar
los montos**. La informacion economica se mantiene en reserva. Debe indicarse fecha del
proceso, periodo, y direccion fisica y electronica para allegar documentacion.
Estado: Firme. Fuente: `RD 13.8.1` a `RD 13.8.4`

### R-19 Prescripcion ONI: 3 anos
Contados desde la publicacion del listado. Prescribe a favor de REDES SGC.
Estado: Firme. Fuente: `RD 13.8.7`, `RD 15.2`

### R-20 Prescripcion general: 10 anos
Las remuneraciones no cobradas prescriben a los diez anos desde la notificacion al interesado
del proyecto de reparticion.
Estado: Firme. Fuente: `RD 15.1`, Ley 1915 de 2018 Art. 34

### R-21 Notificacion valida
Se entiende notificado el envio del proyecto de reparto al correo que el socio informo, o su
puesta a disposicion en la pagina web de la sociedad.
Estado: Firme. Fuente: `RD 13.8.8`

## Reclamaciones

### R-22 Plazo de respuesta: 15 dias habiles
Cada caso se analiza y responde individualmente por escrito.
Estado: Firme. Fuente: `RD 14.3`

### R-23 No se atienden reclamos por errores en la declaracion
Los procesos de distribucion se basan en la documentacion de IDA o en lo declarado por los
autores. Los errores de declaracion los resuelve quien declaro.
Estado: Firme. Fuente: `RD 14.5.6`

### R-24 Solo reclama quien estaba afiliado en el periodo
No se atiende el reclamo de un escritor que no estuviera afiliado a REDES SGC o a una sociedad
con contrato de representacion al cierre del proceso de distribucion.
Estado: Firme. Fuente: `RD 14.5.5`, `RD 14.5.8`

### R-25 Los autores de sociedades hermanas reclaman por su sociedad
No se reciben reclamaciones directas de esos autores.
Estado: Firme. Fuente: `RD 14.5.7`

## Alcance del recaudo

### R-26 Excepciones: no es comunicacion publica
Uso con fines estrictamente educativos dentro de institutos de educacion sin cobro de
entrada; y establecimientos abiertos al publico que usan la obra para entretenimiento de sus
trabajadores o cuya finalidad no sea entretener al publico consumidor con animo de lucro.
Estado: Firme. Fuente: `RD 8.2`, `RT 1`, Ley 1835 de 2017 paragrafo 2

### R-27 Se excluyen canales sin contenido del catalogo
Para television por suscripcion, se excluyen del reparto los canales que no transmitan
contenido del catalogo de REDES SGC.
Estado: Firme. Fuente: `RD 9.5`
Implementacion: hace falta un filtro de repertorio a nivel de canal **y** a nivel de programa.
Los noticieros y magazines de la parrilla de muestra probablemente no son repertorio. Ver
`docs/dominio/fuentes-datos.md`.

## Afiliacion

### R-28 Exclusividad de sociedad
No se acepta como afiliado a quien pertenezca a otra SGC del mismo genero, en el pais o en el
exterior, sin renuncia previa y expresa.
Estado: Firme. Fuente: `RS 1`, `RS 4.1`, Decision Andina 351 Art. 45 literal k

### R-29 Requisito minimo para ser socio activo
Ser titular originario de al menos una obra literaria explotada publicamente.
Estado: Firme. Fuente: `RS 4.1`

## Anticipos

### R-30 Solo los Socios pueden pedir anticipo
Los Titulares Administrados quedan excluidos porque el anticipo se cubre con obra futura y
los herederos o titulares derivados no seguiran creando.
Estado: Firme. Fuente: `RA 2.2`

### R-31 Topes del anticipo
No puede superar el **25%** de los derechos generados a favor del socio en los repartos
ordinarios de los **ultimos dos anos** (nacional e internacional), y en ningun caso puede
superar **5 SMMLV**.
Estado: Firme. Fuente: `RA 3.1.g`, `RA 3.1.h`

### R-32 Un solo anticipo vigente
No se otorga anticipo nuevo mientras exista saldo pendiente, ni a afiliados con embargos
reportados.
Estado: Firme. Fuente: `RA 3.1.c`, `RA 3.1.d`, `RA 3.1.e`

### R-33 Descuento automatico
El anticipo se cubre automaticamente con cargo a los derechos futuros del afiliado en los
procesos de recaudo nacional e internacional.
Estado: Firme. Fuente: `RA 2.1`

## Rendimientos financieros

### R-34 Fechas de corte
Recaudo Nacional: **20 de octubre**. Recaudo Internacional: **30 de septiembre**. Los
rendimientos engrosan las cantidades de reparto de cada autor proporcionalmente y con base en
los porcentajes declarados. Corresponden a la misma vigencia del periodo a distribuir,
entendida como el ano en que ocurrio la comunicacion publica.
Estado: Firme. Fuente: `RD 10.1`, `RD 10.2.1`

### R-35 Inversiones nacional e internacional separadas
Para poder identificar a que tipo de reparto corresponden los rendimientos.
Estado: Firme. Fuente: `RD 10.3`

## Tarifas

### T-01 Television abierta y cerrada: 4%
Sobre los ingresos vinculados a la utilizacion del repertorio.
Estado: Firme. Fuente: `RT 3.1.1`, `RT 3.1.2`

### T-02 Salas de cine: 4% con base contradictoria
Estado: **Conflicto**.
`RT 3.2` dice: 4% de los ingresos netos de explotacion del exhibidor, comprendiendo taquilla,
publicidad y servicios de restauracion.
`RT 4` (tabla resumen) dice: el 4% se calcula **a partir del 50% del recaudo de taquilla**,
porque el 50% restante va al distribuidor.
Las dos bases dan resultados muy distintos y ninguna referencia a la otra.
Accion: **preguntar al cliente cual aplica** antes de implementar el calculo de cine.

### T-03 Transporte aereo: $1.492 COP por plaza utilizada
Estado: Firme. Fuente: `RT 3.3`

### T-04 Transporte terrestre: tarifa mensual por vehiculo segun capacidad
1 a 20 pasajeros $20.600; 21 a 30 $33.212; 31 a 40 $45.824; 41 a 50 $58.436; 51 en adelante
$74.621.
Estado: Firme. Fuente: `RT 3.4.1`

### T-05 Transporte fluvial: USD por aparcada en puerto
1 a 500 pasajeros USD 100; 501 a 1000 USD 200; 1001 a 2000 USD 400; 2001 a 3000 USD 600;
3001 a 4000 USD 800; 4001 a 5000 USD 1000; 5001 en adelante USD 1200.
Estado: Firme. Fuente: `RT 3.4.2`
Implementacion: tarifa en dolares dentro de un sistema en pesos. Definir fuente y fecha de
la TRM aplicable.

### T-06 Hoteles: tarifa mensual por habitacion, con un vacio
La tabla cruza numero de habitaciones con categoria de precio de la habitacion estandar.
Estado: **Confirmar**. Fuente: `RT 3.5`
El tramo **71 a 100 habitaciones** define Categoria 4 hasta $42.000 y Categoria 5 desde
$160.001, y deja **sin tarifa el rango $42.001 a $160.000**. El tramo *100 en adelante* solo
define desde $42.000. Accion: pedir al cliente la tabla corregida.

### T-07 Establecimientos de salud: tarifa mensual por habitacion
1 a 20 habitaciones $1.150; 21 a 50 $2.410; 51 a 70 $3.417; 71 a 100 $4.545; 100 en adelante
$6.888.
Estado: Firme. Fuente: `RT 3.5`

### T-08 Medios digitales: 4%
Sobre los ingresos netos por pago mensual de suscripcion. Si no es posible determinarlos, se
acuerda un pago periodico con parametros objetivos.
Estado: Firme. Fuente: `RT 3.6`

### T-09 Otros establecimientos abiertos al publico: $15.000 por receptor al mes
Estado: Firme. Fuente: `RT 3.7`

### T-10 Recargo por incumplimiento: 50%
Aplica en todas las categorias de usuario.
Estado: Firme. Fuente: `RT 3.1.1` a `RT 3.7`

### T-11 Las tarifas son marco de negociacion
Por mandato legal deben concertarse con el usuario. El valor efectivamente cobrado puede
diferir del publicado.
Estado: Firme. Fuente: `RT 1`, `RT` Presentacion, Decreto 1066 de 2015 Art. 2.6.1.2.5
Implementacion: el sistema debe guardar la tarifa **pactada por convenio con cada usuario**,
no asumir la tarifa publicada.

## Preguntas abiertas para el cliente

1. `T-02` Base de calculo real para salas de cine.
2. `T-06` Tarifa hotelera para el rango sin cubrir en 71 a 100 habitaciones.
3. Relacion entre REDES-SYS, AVSYS e Intela. Ver `glosario.md`.
4. Coeficientes `Wa`, `Wb`, `Wc` de la formula OTT. Ver `formulas.md`.
5. Mapeo entre los tipos de obra del reglamento y los generos de las parrillas. Ver
   `fuentes-datos.md`.
6. Proveedor y formato del dato de rating por franja horaria.
