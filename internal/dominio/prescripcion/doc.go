// Package prescripcion decide cuando un importe no reclamado deja de ser
// exigible y que se hace con el.
//
// Vacio a proposito: es andamiaje.
//
// # Por que el tiempo entra inyectado
//
// Los plazos de RD 15 se cuentan en anos. Probar que un importe prescribe a
// los tres o a los diez no puede depender de esperar una decada, asi que el
// instante entra por el puerto de reloj y nunca por time.Now().
//
// depguard deniega el paquete time entero dentro de internal/dominio (regla
// dominio-sin-reloj-ni-azar). Los TIPOS time.Time se reciben como parametro;
// el instante no se obtiene aqui.
package prescripcion
