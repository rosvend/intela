package normalizacion

import (
	"strings"

	"github.com/shopspring/decimal"
)

// DuracionArtistica aplica RD 9.1.1(c): la duracion artistica es el
// porcentaje normativo de la reportada por el proveedor especializado.
//
// pct llega del snapshot. Un 0.80 escrito aqui haria irreproducible un
// reparto el dia que la Asamblea cambie el coeficiente (ADR 0004).
func DuracionArtistica(reportada, pct decimal.Decimal) decimal.Decimal {
	return reportada.Mul(pct)
}

// MinutosDeHoraTelevisiva aplica RD 9.1.1(c): una hora de emision se computa
// como los minutos que fije el snapshot (48 en la version vigente).
func MinutosDeHoraTelevisiva(horas, minutosPorHora decimal.Decimal) decimal.Decimal {
	return horas.Mul(minutosPorHora)
}

// duracionTV elige UNA de las dos transformaciones, nunca las dos.
//
// Encadenarlas seria contar los anuncios dos veces: 80% de 60 minutos YA es
// 48, que es la hora televisiva del ejemplo de formulas.md 9.1 (Serie Y).
// La unidad declara cual de las dos aplicar; si no viene, la parrilla
// reporta minutos y se aplica el porcentaje artistico.
func duracionTV(reportada decimal.Decimal, unidad string, p Parametros) (decimal.Decimal, *Revision) {
	if !p.DuracionArtisticaPct.GreaterThan(decimal.Zero) || !p.MinutosHoraTV.GreaterThan(decimal.Zero) {
		return decimal.Zero, &Revision{
			Codigo:  CodigoParametroAusente,
			Campo:   "duracion",
			Detalle: "faltan duracion.artistica_pct o duracion.minutos_hora_tv en el snapshot",
		}
	}
	switch strings.ToLower(strings.TrimSpace(unidad)) {
	case unidadHoras:
		return MinutosDeHoraTelevisiva(reportada, p.MinutosHoraTV), nil
	case "", unidadMinutos:
		return DuracionArtistica(reportada, p.DuracionArtisticaPct), nil
	default:
		return decimal.Zero, &Revision{
			Codigo:  CodigoMedidaInvalida,
			Campo:   "unidad_duracion",
			Detalle: "unidad_duracion " + unidad + ": se esperaba minutos u horas",
		}
	}
}
