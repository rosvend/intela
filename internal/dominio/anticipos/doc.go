// Package anticipos gestiona las solicitudes de anticipo de derechos de autor
// y su descuento contra liquidaciones futuras (Reglamento de Anticipos a
// Afiliados V7, capitulos 2 y 3).
//
// # Que es un anticipo
//
// RA 2.1: "la liquidacion adelantada de cantidades por concepto de Derecho de
// Autor ... que sera cubierta de manera automatica con cargo a los derechos
// generados a futuro por el afiliado en los procesos de recaudo nacional e
// internacional". No es un prestamo: no hay intereses ni cuotas, se cubre con
// lo que el socio siga generando. Esa naturaleza explica todas las reglas.
//
// # Las reglas
//
//   - R-30 (RA 2.2): solo un Socio pide anticipo. Un Titular Administrado no,
//     porque "no seguiran creando obras" y el anticipo se cubre con obra
//     futura.
//   - R-31 (RA 3.1.g y 3.1.h): el monto no supera el menor de dos topes, el
//     25% de los derechos de los repartos ordinarios de los ultimos dos anos y
//     5 SMMLV ([Tope]).
//   - R-32 (RA 3.1.c, 3.1.d y 3.1.e): un solo anticipo vigente, ninguno nuevo
//     con saldo pendiente, y ninguno a quien tenga embargos reportados.
//   - R-33 (RA 2.1): el saldo se descuenta solo de las liquidaciones que siguen
//     ([Anticipo.Recuperar]).
//
// Las mismas reglas se aplican al pedir y al aprobar: "La solicitud de un
// anticipo no supone su autorizacion" (RA 3.1.b), y lo que el reglamento
// prohibe es el desembolso.
//
// # La frontera
//
// Un anticipo es un adelanto sobre reparto que aun no ha ocurrido, asi que el
// modulo tiene que poder responder cuanto queda por descontar sin recalcular
// ninguna corrida: el saldo es estado propio, no una suma sobre resultados.
//
// No importa reparto ni recaudo (depguard, regla `modulos-anticipos`): el
// descuento se aplica sobre el importe ya liquidado del titular, nunca sobre la
// valorizacion de la obra ni sobre el valor punto. Si los tocara, el anticipo de
// un autor cambiaria lo que cobran sus coautores. Si importa afiliacion, que es
// el modulo del que dependen todos y que no depende de ninguno: la clase del
// padron es la que decide R-30.
//
// El paquete no lee el reloj: la fecha de la decision la resuelve aplicacion y
// entra ya convertida en una [Situacion] (derechos de la ventana y SMMLV
// vigente). Las decisiones que el reglamento deja abiertas -de donde sale la
// base del 25%, que SMMLV, quien reporta un embargo, como convive el descuento
// con la menor cuantia de R-11- estan en el ADR 0027.
package anticipos
