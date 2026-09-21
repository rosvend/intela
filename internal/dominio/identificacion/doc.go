// Package identificacion cruza cada fila de un reporte de uso contra el
// catalogo maestro de obras, y emite a la cola manual (ONI) lo que no
// reconoce con certeza suficiente.
//
// Identificacion escribe alias y emite ONI, y NO TOCA DINERO (ADR 0003). Por
// eso depguard le deniega importar reparto y recaudo: el vinculo entre un
// titulo de una parrilla y una obra es evidencia de matching, jamas insumo de
// reparto (R-02, R-03).
//
// # Que hay aqui y que no
//
// La cascada entera, pura: [Resolver] decide, y quien llama le trae los
// sondeos ya resueltos en una [Consulta]. La similitud y la normalizacion de
// titulos viven en el adaptador, detras de un puerto (D1 del diseno de #32).
//
// # La cascada del ADR 0007
//
// Cuatro escalones, en orden, parando en el primero que acierta:
//
//  1. Alias aprendido: (fuente, tipo de id, valor) ya resuelto antes a mano.
//  2. Identificador global: IDA, EIDR o IMDB. Requiere que la entrada los
//     traiga; si llegan vacios el escalon no puede casar nunca.
//  3. Difuso sobre el titulo normalizado, por encima de umbral_match, que es
//     un parametro normativo con vigencia y no una constante (ADR 0004).
//  4. ONI: a la cola manual. No es un fallo, es el diseno.
//
// El escalon 3 tiene DOS cortes (ver [Umbrales]). Las decisiones de diseno
// estan en docs/planes/32-difuso/diseno.md.
//
// Un error de base de datos al consultar un escalon NO es "no hay match":
// reclasificar en silencio un uso como ONI por un fallo de red es un error
// que luego hay que desenredar a mano.
//
// [Resultado.Evidencia] tiene que persistirse. El ADR 0006 pide poder
// responder COMO se reconocio una obra, no solo por que escalon paso.
package identificacion
