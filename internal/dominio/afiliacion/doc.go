// Package afiliacion gestiona el padron: quien es Socio, quien es Titular
// Administrado, y que IPI le corresponde a cada uno.
//
// Lo que hoy vive aqui es [Titular] -una entrada del padron, con su invariante
// de construccion- y, con el, la mitad del nucleo de R-01: la pregunta de
// quien puede recibir reparto, [Titular.PuedeRecibirReparto]. La otra mitad es
// el trigger `exigir_persona_natural` del esquema, que es la ultima barrera y
// la unica que cubre lo que entre por SQL crudo.
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
