// Package afiliacion gestiona el padron: quien es Socio, quien es Titular
// Administrado, y que IPI le corresponde a cada uno.
//
// Vacio a proposito: es andamiaje.
//
// # Por que importa la distincion
//
// Socio y Titular Administrado no son lo mismo, y la diferencia decide quien
// vota y quien cobra. El reglamento de socios (capitulo 4) fija los tipos de
// afiliado; el RD 4.5 fija quien puede recibir una orden de pago.
//
// De ahi sale el invariante R-01: no existe firma que emita orden de pago a
// quien no sea escritor persona natural. Una productora puede estar en el
// padron; no puede cobrar reparto.
package afiliacion
