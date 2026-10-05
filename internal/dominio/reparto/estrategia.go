package reparto

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// ponderacionTipo resuelve la ponderacion de RD 9.1.1 desde el snapshot.
// serie y telenovela comparten PondSerie.
func ponderacionTipo(tipo, obraID string, snap Snapshot) (decimal.Decimal, error) {
	switch tipo {
	case "cinematografica":
		if err := exigirPositivo("pond_cine", snap.PondCine); err != nil {
			return decimal.Zero, fmt.Errorf("%w (obra %q)", err, obraID)
		}
		return snap.PondCine, nil
	case "unitario":
		if err := exigirPositivo("pond_unitario", snap.PondUnitario); err != nil {
			return decimal.Zero, fmt.Errorf("%w (obra %q)", err, obraID)
		}
		return snap.PondUnitario, nil
	case "serie", "telenovela":
		if err := exigirPositivo("pond_serie", snap.PondSerie); err != nil {
			return decimal.Zero, fmt.Errorf("%w (obra %q)", err, obraID)
		}
		return snap.PondSerie, nil
	case "sketches":
		if err := exigirPositivo("pond_sketch", snap.PondSketch); err != nil {
			return decimal.Zero, fmt.Errorf("%w (obra %q)", err, obraID)
		}
		return snap.PondSketch, nil
	case "":
		return decimal.Zero, fmt.Errorf("%w: tipo_obra vacio (obra %q)", ErrRepartoInvalido, obraID)
	default:
		return decimal.Zero, fmt.Errorf("%w: tipo_obra %q (obra %q)", ErrRepartoInvalido, tipo, obraID)
	}
}

// puntosTV suma los terminos de [DesglosarUso] (RD 9.1.1): un solo camino
// para el motor y el recibo.
func puntosTV(usos []Uso, snap Snapshot) (map[string]decimal.Decimal, error) {
	out := make(map[string]decimal.Decimal)
	for _, u := range usos {
		ts, err := terminosTV(u, snap)
		if err != nil {
			return nil, err
		}
		out[u.ObraID] = out[u.ObraID].Add(sumar(ts))
	}
	return out, nil
}

// puntosCineTeatro suma los terminos de [DesglosarUso] (P-18): un solo camino
// para el motor y el recibo.
func puntosCineTeatro(usos []Uso, snap Snapshot) (map[string]decimal.Decimal, error) {
	out := make(map[string]decimal.Decimal)
	for _, u := range usos {
		ts, err := terminosCineTeatro(u, snap)
		if err != nil {
			return nil, err
		}
		out[u.ObraID] = out[u.ObraID].Add(sumar(ts))
	}
	return out, nil
}

// puntosTransporte suma los terminos de [DesglosarUso]: un solo camino para
// el motor y el recibo.
func puntosTransporte(usos []Uso) (map[string]decimal.Decimal, error) {
	out := make(map[string]decimal.Decimal)
	for _, u := range usos {
		ts, err := terminosTransporte(u)
		if err != nil {
			return nil, err
		}
		out[u.ObraID] = out[u.ObraID].Add(sumar(ts))
	}
	return out, nil
}

// puntosOTT suma los terminos de [DesglosarUso] (Pi = PB*Wa + DU*Wb + V*Wc):
// un solo camino para el motor y el recibo.
func puntosOTT(usos []Uso, snap Snapshot) (map[string]decimal.Decimal, error) {
	out := make(map[string]decimal.Decimal)
	for _, u := range usos {
		ts, err := terminosOTT(u, snap)
		if err != nil {
			return nil, err
		}
		out[u.ObraID] = out[u.ObraID].Add(sumar(ts))
	}
	return out, nil
}

func pctGrupo(g GrupoCanal, snap Snapshot) (decimal.Decimal, error) {
	var (
		v    decimal.Decimal
		name string
	)
	switch g {
	case GrupoPrivadosNacionales:
		v, name = snap.GrupoPrivadosPct, "grupo_privados_pct"
	case GrupoRegionalesPublicos:
		v, name = snap.GrupoRegionalesPct, "grupo_regionales_pct"
	case GrupoPremium:
		v, name = snap.GrupoPremiumPct, "grupo_premium_pct"
	case GrupoLideresRating:
		v, name = snap.GrupoLideresPct, "grupo_lideres_pct"
	case GrupoEstandar:
		v, name = snap.GrupoEstandarPct, "grupo_estandar_pct"
	default:
		return decimal.Zero, fmt.Errorf("%w: grupo %q", ErrRepartoInvalido, g)
	}
	if err := exigirNoNegativo(name, v); err != nil {
		return decimal.Zero, err
	}
	if v.IsZero() {
		return decimal.Zero, fmt.Errorf("%w: %s", ErrParametroAusente, name)
	}
	return v, nil
}

// Pondera dice si un uso entra en la valorizacion de su corrida. Es la unica
// definicion: el motor (filtrarRepertorio) y el asiento del recibo la
// comparten. R-27 (RD 9.5): en suscripcion y hotel un uso fuera de repertorio
// no suma; en las demas modalidades la marca no aplica.
func Pondera(u Uso) bool {
	return !u.FueraDeRepertorio || (u.Modalidad != Suscripcion && u.Modalidad != Hotel)
}

// filtrarRepertorio aplica R-27: excluye usos de canales fuera de repertorio.
func filtrarRepertorio(usos []Uso) []Uso {
	out := make([]Uso, 0, len(usos))
	for _, u := range usos {
		if !Pondera(u) {
			continue
		}
		out = append(out, u)
	}
	return out
}

func agruparPorGrupo(usos []Uso) map[GrupoCanal][]Uso {
	out := make(map[GrupoCanal][]Uso)
	for _, u := range usos {
		out[u.Grupo] = append(out[u.Grupo], u)
	}
	return out
}
