// Package aplicacion declara los puertos que el nucleo necesita y orquesta
// los casos de uso, fijando los limites de transaccion.
//
// Sigue siendo nucleo: no nombra infraestructura. Los adaptadores satisfacen
// estos puertos sin ser importados desde aqui (ADR 0002).
//
// # Por que los puertos estan partidos
//
// Habia una sola interfaz de 39 metodos que reunia usuarios, sesiones, obras,
// declaraciones, alias, reportes, usos, bolsas, parametros normativos,
// procesos, firmas, resultados, la bitacora, una cola de trabajos, el
// calendario, alertas, anticipos, reclamaciones y el seed de la base.
//
// Eso dejaba sin efecto el ADR 0003, que pide que cada modulo tenga su propia
// interfaz publica. Y volvia INAPLICABLE POR CONSTRUCCION la regla "ningun
// modulo escribe en la trazabilidad de otro": si Asentar esta en el contrato
// compartido, lo llama cualquiera y no hay forma de saber quien deberia.
//
// Ahora hay un puerto por modulo y por rol. Cada caso de uso declara solo lo
// que necesita, que es lo que hace exigible la frontera en revision. El
// adaptador de PostgreSQL puede seguir siendo un unico tipo que los satisfaga
// todos; eso es asunto suyo, no del nucleo.
//
// # Autorizacion
//
// El middleware de roles vive en el adaptador HTTP. Aqui vive el predicado
// que no depende del transporte: [SoloPropiasObras] (OE-6). Los casos de
// uso que aterrizen lo aplican; no es un filtro SQL.
//
// # Que falta aqui
//
// El resto de los casos de uso. Este paquete declara los contratos, el
// predicado de titularidad y los casos que ya aterrizaron (autenticacion,
// liquidacion exportable). Cada PR de seguimiento trae los suyos, y con
// ellos el asiento en bitacora, que el ADR 0006 declara "parte de la
// definicion de hecho de cada caso de uso".
package aplicacion
