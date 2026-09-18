// Package afiliacion gestiona el padron: quien es Socio, quien es Titular
// Administrado, y que IPI le corresponde a cada uno.
//
// # Por que importa la distincion
//
// Socio y Titular Administrado no son lo mismo, y la diferencia decide quien
// vota y quien cobra. El reglamento de socios (capitulo 4) fija los tipos de
// afiliado; el RD 4.5 fija quien puede recibir una orden de pago.
//
// Socio es un vinculo societario (`RS 4.1`). Titular Administrado es un
// vinculo contractual (`RS 4.2`). Solo el Socio puede pedir un anticipo
// (`R-30`, `RA 2.2`): el anticipo se cubre con obra futura, y quien no va a
// seguir creando no lo pide.
//
// # Admision
//
// Nadie entra al padron por existir una fila. El Consejo Directivo estudia
// cada solicitud (`RS 5.2`, `RS 5.3`) y la deja en pendiente, la admite o la
// rechaza. Mientras este pendiente, no hay vigencia: la pregunta del
// reglamento no es "es afiliado" sino "estaba afiliado entonces" (`R-24`).
//
// # Exclusividad
//
// No se acepta a quien pertenezca a otra SGC del mismo genero sin renuncia
// previa y expresa (`R-28`, `RS 4.1`). La evidencia es el documento de
// renuncia, no una casilla.
package afiliacion
