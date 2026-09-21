package identificacion

import "github.com/shopspring/decimal"

// Entrada es una fila de reporte lista para identificar.
//
// IDA, EIDR e IMDB son el escalon 2 de la cascada: si quien construye la
// Entrada no los rellena, ese escalon no puede casar y todo lo que no tenga
// alias cae directo a difuso o a ONI.
type Entrada struct {
	Fuente     string
	TipoID     string
	ValorID    string
	IDA        string
	EIDR       string
	IMDB       string
	Titulo     string
	TituloOrig string
}

// MaxCandidatos es cuantos candidatos se conservan por fila: una bandeja de
// revision con veinte filas parecidas no se revisa, se cierra. Lo respeta el
// adaptador al recuperar y [UnirCandidatos] al juntar los de varios titulos.
const MaxCandidatos = 5

// Candidato es una obra propuesta por el motor de similitud, con su puntaje.
type Candidato struct {
	ObraID  string
	Puntaje decimal.Decimal

	// TituloConsultado es contra que titulo de la fila se puntuo -el emitido o el
	// original-. Es evidencia: dice de donde sale el puntaje, y no entra en el
	// orden ni en la decision.
	TituloConsultado string
}

// Resultado de la cascada. No tiene campo de dinero y no lo tendra:
// identificacion no toca dinero (ADR 0003).
//
// Evidencia dice COMO se reconocio -que alias, que identificador, contra que
// titulo-, no solo por que escalon paso. Es lo que permite responder a una
// auditoria de RD 16, y tiene que llegar hasta la bitacora.
type Resultado struct {
	ObraID    string
	Escalon   string
	Puntaje   decimal.Decimal
	ONI       bool
	Evidencia string

	// Candidatos solo viaja en la banda ambigua: es lo que revisa #39 (D2).
	Candidatos []Candidato
}
