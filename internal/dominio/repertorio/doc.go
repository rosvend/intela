// Package repertorio mantiene el catalogo maestro de obras y la declaracion
// de autoria de cada una: quien escribio que, y en que porcentaje.
//
// # La frontera
//
// Reparto lee de Repertorio. Repertorio no conoce a Reparto, y depguard lo
// impide (regla modulos-repertorio de .golangci.yml).
//
// # Dos tipos, dos responsabilidades que no se mezclan
//
// [Obra] es el catalogo maestro: identidad y metadata. [Declaracion] es el
// reparto declarado: quien cobra y cuanto. Estan separados a proposito y la
// separacion es normativa, no estetica.
//
// [Obra] lleva coautores con su rol autoral, y NINGUNO de ellos tiene
// porcentaje. Los nombres del catalogo -igual que las columnas Autor* y
// Guionista* de una parrilla- sirven para IDENTIFICAR la obra; el porcentaje
// sale solo de la Declaracion de Obra (`R-02`, `R-03`, `RD 7.3.1`). Si el
// catalogo pudiera declarar porcentajes habria dos caminos hasta un pago, y
// el segundo no lo firma nadie.
//
// # El invariante que vive aqui
//
// [Declaracion.Completa] es R-04 en codigo: una declaracion vale si sus
// partes suman exactamente 100% y todas traen IPI. Si no suma, se retiene el
// TOTAL en reserva -no hay reparto parcial (RD 13.1.3).
//
// Que la comprobacion sea un metodo del tipo y no un if en el motor es
// deliberado: el motor no puede repartir sin haber preguntado.
//
// La suma se compara con decimal exacto, nunca con float: 33.33 + 33.33 +
// 33.34 tiene que dar 100 y dar 100 siempre.
package repertorio
