package reparto

// Estados de una corrida en una palabra, para el tablero; la etapa sigue siendo la verdad.
const (
	EstadoCorridaEnCurso = "en_curso"
	EstadoCorridaEnFirma = "en_firma"
	EstadoCorridaCerrada = "cerrada"
)

// EstadoDeCorrida resume la etapa: cerrada si es la terminal del circuito, en_firma si es compuerta (RD 13.5).
func EstadoDeCorrida(c Circuito, e Etapa) string {
	secuencia := secuenciaEtapas(c)
	switch {
	case e == secuencia[len(secuencia)-1]:
		return EstadoCorridaCerrada
	case etapaEsCompuerta(e):
		return EstadoCorridaEnFirma
	default:
		return EstadoCorridaEnCurso
	}
}
