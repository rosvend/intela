// Package repertorio mantiene el catalogo maestro de obras y la declaracion
// de autoria de cada una: quien escribio que, y en que porcentaje.
//
// # La frontera
//
// Reparto lee de Repertorio. Repertorio no conoce a Reparto, y depguard lo
// impide (regla modulos-repertorio de .golangci.yml).
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
