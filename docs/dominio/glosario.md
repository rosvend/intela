---
actualizado: 2026-08-10
fuentes: Reglamento de Distribucion IX, Reglamento de Socios, Reglamento de Tarifas VI
---

# Glosario del dominio

Lenguaje ubicuo del proyecto. Los terminos se usan en espanol tal como aparecen en los
reglamentos, tambien en nombres de tablas, entidades y variables, para que el codigo y el
marco legal hablen igual.

Cada entrada cita su fuente. `RD` = Reglamento de Distribucion IX, `RS` = Reglamento de
Socios, `RT` = Reglamento de Tarifas VI, `RA` = Reglamento de Anticipos.

## Actores

**REDES SGC** — Red Colombiana de Escritores Audiovisuales, de Teatro, Radio y Nuevas
Tecnologias, Sociedad de Gestion Colectiva. NIT 901.295.540-1. Actua como mandataria de
sus afiliados para administrar el derecho de remuneracion de la Ley 1835 de 2017 (Ley Pepe
Sanchez). Miembro pleno de CISAC desde 2020. `RD 1`, `RD 3`

**Titular** — Autor y/o titular del derecho de recaudo por comunicacion publica: los autores
y/o coautores **del guion o libreto** de la obra audiovisual. Incluye a sus derechohabientes
por sucesion mortis causa y al conyuge o companero permanente sobreviviente. `RD 7`

**Derechohabiente** — Persona natural o juridica a quien por cualquier titulo se transmiten
derechos reconocidos en la Decision Andina 351 de 1993. `RD 3`

**Socio** — Afiliado con vinculo societario. Es titular originario: guionista, libretista,
autor de teatro, escritor de radio. Se subdivide en honorarios (sin derechos politicos ni
patrimoniales), fundadores y activos. `RS 4.1`

**Titular Administrado** — Afiliado con vinculo contractual, no societario. Son quienes
cumpliendo requisitos optan por esta figura, quienes perdieron la calidad de socio, y los
derechohabientes (herederos). `RS 4.2`

**Usuario** — Quien explota principal o secundariamente las obras protegidas dentro de su
actividad principal. Es quien paga el recaudo. Canales de TV, salas de cine, plataformas
OTT, hoteles, transporte. `RD 3`, `RT 2`

**Sociedad Hermana** — SGC que gestiona los mismos derechos que REDES SGC en otros paises.
Se relacionan por contratos de representacion reciproca o unilateral. `RD 3`

## Obras y derechos

**Obra Audiovisual** — Creacion expresada mediante una serie de imagenes asociadas, con o
sin sonorizacion incorporada, destinada esencialmente a ser mostrada a traves de aparatos de
proyeccion o cualquier otro medio de comunicacion de imagen y sonido. `RD 3`, Decision 351
Art. 3

**ONI (Obra No Identificada)** — Obra cuyo autor se desconoce al momento del reparto. Genera
un listado publico y un tratamiento especial de reserva y prescripcion. `RD 3`, `RD 13.8`

**Declaracion de Obra** — Acto por el cual el autor informa a REDES SGC las obras que ha
escrito y que han sido comunicadas publicamente. Se hace en linea por REDES-SYS. Consigna el
titulo, genero, ano de produccion y **el porcentaje de participacion de cada coautor hasta
completar el 100%**. Es la unica fuente valida de los splits. `RD 3`, `RD 13.1.2`

**Repertorio** — Conjunto de obras que REDES SGC representa, propias y de sociedades
hermanas por contrato de representacion.

## Dinero

**Recaudo** — Proceso de recoleccion de los pagos que hacen los usuarios por la comunicacion
publica de las obras del repertorio. Se divide en Recaudo Nacional y Recaudo Internacional,
que se gestionan y se invierten por separado. `RD 3`, `RD 10.3`

**Tarifa** — Tabla de precios, derechos o cuotas cobradas por la comunicacion publica del
repertorio. Es marco de referencia: por mandato legal debe concertarse con el usuario. `RT 2`

**Deducciones Legales** — Descuentos previos al reparto: hasta 20% para gastos
administrativos (30% durante los dos primeros anos de la sociedad) y hasta 10% para
programas de inversion social. `RD 3`, Ley 44 de 1993 Art. 21

**Reparto / Distribucion** — Asignacion del recaudo neto entre las obras y luego entre los
titulares de cada obra.

**Valor Punto** — Total a repartir de un canal dividido por el total de puntos de ese canal.
Convierte puntos de obra en pesos. `RD 9.1.1`

**Fees in Error** — Recaudos recibidos por error desde una sociedad de gestion con contrato
de representacion. Se devuelven sin deducciones administrativas ni de bienestar social.
`RD 3`, `RD 13.7`

**Anticipo de Derechos de Autor** — Liquidacion adelantada de derechos futuros, otorgada por
necesidad demostrada y descontada automaticamente de repartos posteriores. Solo para Socios.
`RA 2`

## Identificadores externos

**IPI (Interested Parties Information)** — Identificador unico internacional de una persona
fisica o juridica con participacion en una obra, con su rol (compositor, arreglista, editor).
Administrado por SUISA dentro del sistema CISAC. **Es el identificador de los autores.**
`RD 3`

**IDA (International Documentation on Audiovisual Works)** — Base de datos centralizada de
CISAC que identifica obras audiovisuales y sus titulares. Facilitada por CISAC a sus socios.
**Es el identificador de las obras entre sociedades.** `RD 3`, `RD 14.5.6`

Ver `docs/dominio/identificadores.md` para como se relacionan con los IDs de las fuentes de
datos (ID_Ficha, show_id, EIDR, IMDB).

## Sistemas de REDES SGC

**REDES-SYS** — Aplicativo web de declaracion de obra en linea, en redescritores.com. Ahi el
autor declara sus obras y los porcentajes de reparto. `RD 3`, `RD 13.1.2`

**AVSYS** — Sistema donde se carga el neto a repartir, se genera la valorizacion de cada obra
segun la ponderacion, y se producen las liquidaciones parcial y final. `RD 13.5`

Nota de lectura: el Reglamento nombra los dos sistemas pero no explica su relacion ni si son
modulos del mismo producto. **Confirmar con el cliente** si Intela reemplaza a uno, a ambos,
o se integra con ellos. Esta pregunta condiciona todo el alcance del proyecto.

## Organos de REDES SGC

**Consejo Directivo** — Aprueba reglamentos, admite afiliados, fija fechas de distribucion,
aprueba anticipos, revisa declaraciones de obra. `RS 3.1`, `RD 12`

**Comite de Vigilancia** — Vela por el cumplimiento legal y estatutario, aplica sanciones.
`RS 3.2`

**Asamblea General** — Unico organo que puede aprobar el porcentaje de la reserva por errores
tecnicos y autorizar usos distintos de los rendimientos financieros. `RD 14.5.1`, `RD 10.4`

**Revisor Fiscal** — Auditoria periodica del proceso de recaudo y reparto. `RD 13.5`, `RD 16`

**DNDA** — Direccion Nacional de Derecho de Autor. Ente externo de inspeccion y vigilancia.
`RD 16.2.2`

## Marco legal citado

- **Ley 1835 de 2017** (Ley Pepe Sanchez) — crea el derecho de remuneracion para escritores
  de obras audiovisuales.
- **Decision Andina 351 de 1993** — regimen comun de derecho de autor.
- **Ley 44 de 1993** Art. 21 — deducciones; Art. 22 (mod. Ley 1915 de 2018 Art. 34) —
  prescripcion.
- **Decreto 1066 de 2015** — reglamentacion de las SGC.
- **Ley 182 de 1995** Art. 18 a 22 — clasificacion del servicio de television.
- **Ley 1915 de 2018** Art. 34 — prescripciones.
