package oni

import "strings"

// AnclarFecha registra la fecha de publicacion una sola vez.
//
// R-19 cuenta tres anos desde la publicacion del listado. Si una segunda
// llamada pudiera sustituir la fecha, el reloj de prescripcion se resetearia
// y un recaudo que ya debia haber prescrito seguiria exigible -o al reves,
// uno vigente aparecería prescrito-. Por eso el valor que ya esta anclado
// gana siempre, aunque el candidato sea distinto.
//
// El instante llega como texto (RFC 3339) y no como time.Time: el paquete
// time esta denegado en el dominio. Quien llama formatea; aqui solo se
// decide si se acepta.
func AnclarFecha(actual, candidato string) (string, error) {
	candidato = strings.TrimSpace(candidato)
	if candidato == "" {
		return actual, ErrFechaAusente
	}
	if actual != "" {
		return actual, nil
	}
	return candidato, nil
}
