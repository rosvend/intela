package reparto

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// ponderacionTipo resuelve la ponderacion de RD 9.1.1 desde el snapshot.
// serie y telenovela comparten PondSerie.
func ponderacionTipo(tipo string, snap Snapshot) (decimal.Decimal, error) {
	switch tipo {
	case "cinematografica":
		if err := exigirPositivo("pond_cine", snap.PondCine); err != nil {
			return decimal.Zero, err
		}
		return snap.PondCine, nil
	case "unitario":
		if err := exigirPositivo("pond_unitario", snap.PondUnitario); err != nil {
			return decimal.Zero, err
		}
		return snap.PondUnitario, nil
	case "serie", "telenovela":
		if err := exigirPositivo("pond_serie", snap.PondSerie); err != nil {
			return decimal.Zero, err
		}
		return snap.PondSerie, nil
	case "sketches":
		if err := exigirPositivo("pond_sketch", snap.PondSketch); err != nil {
			return decimal.Zero, err
		}
		return snap.PondSketch, nil
	case "":
		return decimal.Zero, fmt.Errorf("%w: tipo_obra vacio", ErrRepartoInvalido)
	default:
		return decimal.Zero, fmt.Errorf("%w: tipo_obra %q", ErrRepartoInvalido, tipo)
	}
}

// puntosTV calcula puntos por obra con la formula de RD 9.1.1:
// Pond(tipo) * Duracion * Rating * Emisiones.
func puntosTV(usos []Uso, snap Snapshot) (map[string]decimal.Decimal, error) {
	out := make(map[string]decimal.Decimal)
	for _, u := range usos {
		pond, err := ponderacionTipo(u.TipoObra, snap)
		if err != nil {
			return nil, err
		}
		emisiones := decimal.NewFromInt(u.Emisiones)
		p := pond.Mul(u.DuracionMin).Mul(u.Rating).Mul(emisiones)
		out[u.ObraID] = out[u.ObraID].Add(p)
	}
	return out, nil
}

// puntosCineTeatro pondera por espectadores o taquilla segun el snapshot (P-01).
func puntosCineTeatro(usos []Uso, snap Snapshot) (map[string]decimal.Decimal, error) {
	switch snap.BaseCineTeatro {
	case BaseEspectadores, BaseTaquilla:
	default:
		return nil, fmt.Errorf("%w: base_cine_teatro", ErrParametroAusente)
	}
	out := make(map[string]decimal.Decimal)
	for _, u := range usos {
		var w decimal.Decimal
		if snap.BaseCineTeatro == BaseEspectadores {
			w = u.Espectadores
		} else {
			w = u.Taquilla
		}
		if w.IsNegative() {
			return nil, fmt.Errorf("%w: medida negativa en obra %q", ErrRepartoInvalido, u.ObraID)
		}
		out[u.ObraID] = out[u.ObraID].Add(w)
	}
	return out, nil
}

// puntosTransporte pondera por exhibiciones. Cero exhibiciones = cero peso.
func puntosTransporte(usos []Uso) map[string]decimal.Decimal {
	out := make(map[string]decimal.Decimal)
	for _, u := range usos {
		w := decimal.NewFromInt(u.Exhibiciones)
		out[u.ObraID] = out[u.ObraID].Add(w)
	}
	return out
}

// puntosOTT aplica Pi = PB*Wa + DU*Wb + V*Wc.
func puntosOTT(usos []Uso, snap Snapshot) (map[string]decimal.Decimal, error) {
	if err := exigirPositivo("ott.wa", snap.Wa); err != nil {
		return nil, err
	}
	if err := exigirPositivo("ott.wb", snap.Wb); err != nil {
		return nil, err
	}
	if err := exigirPositivo("ott.wc", snap.Wc); err != nil {
		return nil, err
	}
	out := make(map[string]decimal.Decimal)
	for _, u := range usos {
		p := u.PB.Mul(snap.Wa).Add(u.MinutosVistos.Mul(snap.Wb)).Add(u.Vistas.Mul(snap.Wc))
		out[u.ObraID] = out[u.ObraID].Add(p)
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

// filtrarRepertorio aplica R-27: excluye usos de canales fuera de repertorio.
func filtrarRepertorio(usos []Uso) []Uso {
	out := make([]Uso, 0, len(usos))
	for _, u := range usos {
		if u.FueraDeRepertorio {
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
