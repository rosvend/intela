// Package normalizacion lleva una fila ya mapeada por el adaptador de formato
// al esquema canonico que consume el resto del sistema.
//
// # Que hay aqui y que no
//
// Funciones puras: una [Fila] mas un snapshot de parametros produce un [Uso]
// canonico o una [Revision] tipada. Sin E/S, sin reloj, sin azar. depguard
// deniega `time` y `os` en todo internal/dominio (ADR 0002, ADR 0005); las
// fechas se parsean a [Fecha] civil, no a time.Time.
//
// El adaptador de ingesta (#25) SOLO mapea columnas. Las reglas de duracion,
// moneda y fecha viven aqui para poder probarse sin un parser y sin un
// archivo. Meterlas en el adaptador las haria inalcanzables para una prueba
// de dominio.
//
// # Lo que no se descarta
//
// Una fila que no se puede normalizar NO desaparece y NO se pone a cero. Se
// devuelve con su [Revision]: el motivo nombra el campo, que es lo que pide
// OE-1 y lo que permite volver a pedirle al cliente exactamente eso. Quien
// orquesta (aplicacion) la mueve a la cola de revision; este paquete no
// persiste nada.
//
// # Las reglas de RD 9.1.1
//
// La hora de emision televisiva se computa como 48 minutos y la duracion
// artistica es el 80% de la reportada por el proveedor. Los coeficientes
// llegan en [Parametros], no estan escritos en el codigo (ADR 0004). Las dos
// transformaciones NO se encadenan: 80% de 60 minutos ES 48 minutos, que es
// la hora televisiva del ejemplo de `formulas.md` 9.1.
//
// Los avances publicitarios de programacion propia no computan: la fila
// queda canonica con duracion cero, no va a revision. Descartarla seria
// perder la evidencia de que el canal la reporto.
package normalizacion
