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
// # Operacion
//
// [Despachador] y [Planificador] son los dos casos de uso que mueven la cola
// de trabajos: uno toma y despacha, el otro encola lo que el calendario
// declara vencido. Estan aqui, y no en cmd/, porque lo que deciden es
// politica -que se reintenta, cuando, con que clave natural- y eso se tiene
// que poder probar sin levantar un proceso.
//
// Ninguno de los dos escribe en la bitacora, y es deliberado. El ADR 0006
// separa observabilidad de trazabilidad: "el worker tomo el trabajo 7" es
// operacion, va al log y se rota; el asiento existe para explicar por que una
// CIFRA es la que es. El asiento lo escribe el manejador que mueve dinero
// -los issues #33 y #34-, no el mecanismo que lo transporta. Meter aqui un
// asiento por trabajo llenaria de ruido operativo el libro que un auditor
// tiene que poder leer dentro de diez anos.
//
// # Que falta aqui
//
// Los casos de uso. Este paquete declara los contratos y el predicado de
// titularidad. Cada PR de seguimiento trae los suyos, y con ellos el
// asiento en bitacora, que el ADR 0006 declara "parte de la definicion de
// hecho de cada caso de uso".
package aplicacion
