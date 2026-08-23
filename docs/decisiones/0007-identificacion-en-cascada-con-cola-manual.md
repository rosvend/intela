# 0007 Identificacion de obras en cascada, con cola manual como ultimo escalon

Fecha: 2026-08-10
Estado: Vigente

## Contexto

`docs/dominio/identificadores.md` documenta un hallazgo medido sobre la muestra del cliente que
condiciona todo el diseno de esta parte: **los IDs que traen los reportes de uso no cruzan entre
fuentes**. `ID_Ficha` de Caracol tiene 5 o 6 digitos y pertenece al catalogo del proveedor de
parrillas; los `show_id`, `series_id` y `netflix_id` tienen 8 y pertenecen al catalogo interno de
Netflix. La interseccion es de cero valores. Ademas estan en granularidades distintas: programa,
show, temporada y episodio. Y `Id_Ntx` ni siquiera es un identificador, es un contador de fila que se
renumera en cada entrega.

Tampoco hay un identificador global que sirva hoy: `eidr` viene vacia en 49 de 49 filas de Netflix,
e `IMDB` existe solo del lado de Caracol, poblada en 32 de 59 filas. No hay ni un identificador
global poblado en comun entre las dos fuentes.

El matching difuso por titulo, medido sobre la muestra, da **0 coincidencias exactas y 0 candidatos
por encima de 0.6 de similitud**, por razones que no son defectos del algoritmo: no hay solape
temporal (Netflix es 2018, Caracol es diciembre de 2024), los catalogos son distintos, Netflix usa
titulos en ingles y Caracol en espanol, y el titulo localizado difiere del original en 16 de 59 filas.

Sobre todo eso pesa el aviso de tamano de `docs/dominio/fuentes-datos.md`: son 59 y 49 filas, que
sirven para disenar esquemas y **no sirven para ajustar un motor de matching ni para medir tasas de
acierto**.

Y el error de modelo que hay que evitar esta identificado: no hay que cruzar Caracol contra Netflix.
Hay que cruzar cada fuente contra el catalogo maestro de obras declaradas, que es lo unico que tiene
autores y porcentajes (`R-03`).

## Decision

**Una cascada de cuatro escalones contra el catalogo maestro, cada uno mas caro y menos confiable
que el anterior, cayendo al siguiente solo si el anterior no resuelve.**

1. **Alias conocido.** Consulta exacta en `alias_obra` por (fuente, tipo de id, valor). Costo cero,
   confianza maxima.
2. **Identificador global.** IDA, EIDR o IMDB cuando esten poblados.
3. **Coincidencia difusa sobre titulo**, usando como features `Titulo`, `Titulo_original`, `Ano`,
   `NacionalidadOrigen`, `Genero`, `Duracion_total` y los campos de creditos.
4. **Cola de resolucion manual**, con la evidencia adjunta para que un humano decida.

Cuatro cosas mas que son parte de la decision y no detalles:

**El alias se aprende.** Resuelto una vez que `show_id 80141259` es la obra X, la correspondencia se
guarda con su confianza, quien la resolvio y cuando. Todo reporte futuro de esa fuente con ese id
entra por el escalon 1. El costo baja con el tiempo y queda la trazabilidad que exigen `RD 13` y
`RD 16`.

**Los campos de creditos son features, nunca insumo de pago.** `Autor*`, `Guionista*` y `Director*`
entran a este modulo tipados como evidencia de identificacion. Los porcentajes salen exclusivamente
de la Declaracion de Obra (`R-03`, `R-02`, `RD 7.3.3`). El modulo de Identificacion no escribe
titulares ni porcentajes, y no toca dinero.

**El umbral difuso es un parametro normativo, no una constante**, con vigencia y responsable, igual
que el resto (`0004-parametros-normativos-como-dato.md`). Y arranca **deliberadamente conservador**:
con la muestra actual no hay forma de calibrarlo, asi que el sesgo por defecto es mandar a la cola
manual antes que decidir mal. Un falso positivo paga a quien no corresponde, y `R-05` deja a REDES
SGC fuera de las disputas entre coautores: el error no se corrige solo.

**Lo no identificado es un estado, no un fallo.** Lo que la cascada no resuelve se marca ONI y sigue
el tratamiento de `RD 13.8`: listado publico con titulos e informacion identificatoria y **sin
montos** (`R-18`), reserva del dinero, y prescripcion a los tres anos desde la publicacion (`R-19`).

Falta ademas el filtro de repertorio, y va **antes** de la cascada: `R-27` excluye los canales sin
contenido del catalogo, y `docs/dominio/fuentes-datos.md` advierte que en la parrilla hay noticieros
y magazines que casi con seguridad no son repertorio. Intentar identificar la obra de un noticiero es
gastar la cola manual en algo que no se va a pagar.

## Alternativas consideradas

**Cruzar las fuentes entre si.** Descartada porque el modelo es equivocado, no porque sea dificil.
Las fuentes nunca necesitan coincidir: cada una se resuelve contra el catalogo maestro, que es lo
unico que tiene autores y porcentajes. Son dos problemas de matching independientes contra un mismo
cubo.

**Solo matching difuso, sin tabla de alias.** Descartada: repetiria el trabajo caro en cada entrega,
volveria a preguntar a un humano lo que ya decidio, y no dejaria el rastro de quien resolvio que,
que es lo que pide la auditoria.

**Un modelo de aprendizaje automatico entrenado para la resolucion de entidades.** Es lo que sugiere
el planteamiento academico de `docs/context.md` y lo que sabe hacer el equipo. Descartada **por
ahora**, no por principio: no hay con que entrenarlo. 59 y 49 filas sin solape temporal ni de
catalogo no son un conjunto de entrenamiento, y un modelo ajustado sobre eso daria una metrica
falsa. Cuando lleguen extractos del mismo periodo y de mayor volumen, el escalon 3 es exactamente el
punto donde entra, sin tocar el resto de la cascada: por eso el motor de similitud esta detras de
`PuertoMotorDeSimilitud`.

**Bloquear el reparto hasta que todo este identificado.** Descartada porque contradice el reglamento:
`RD 13.8` prevee explicitamente que haya obras no identificadas y define su tratamiento. Las ONI son
parte del funcionamiento normal, no una condicion de error.

**Exigir el `eidr` a Netflix antes de construir nada.** Es la peticion de mas valor para la reunion
con el cliente y esta en la lista de `docs/dominio/fuentes-datos.md`, pero no puede ser un requisito
bloqueante: la cascada tiene que funcionar sin el, y funcionar mejor cuando llegue.

## Consecuencias

Positivas: el sistema es util desde el primer dia aunque el escalon difuso acierte poco, porque la
cola manual convierte cada decision humana en un alias reutilizable. El coste operativo decrece
solo. Y la ruta que siguio cada obra queda registrada, asi que se puede medir la tasa de acierto real
de cada escalon cuando por fin haya datos suficientes.

A cambio: al principio la cola manual va a ser larga, y eso es trabajo de una persona de REDES SGC.
La consola administrativa deja de ser un accesorio y pasa a ser una herramienta de produccion que hay
que disenar en serio.

Riesgo asumido: la tentacion de bajar el umbral difuso para vaciar la cola. Reduce trabajo visible
hoy y produce pagos incorrectos que aparecen como reclamaciones meses despues, cuando el dinero ya
salio. La mitigacion es que el umbral sea un parametro con responsable y vigencia, que cada
resolucion automatica guarde su puntaje, y que la tasa de reclamaciones por obra mal asignada se
vigile contra el umbral que estaba vigente cuando se decidio.
