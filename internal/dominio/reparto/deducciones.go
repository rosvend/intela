package reparto

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// deducciones aplica R-06 y R-07 sobre el bruto. La reserva solo aplica al
// circuito nacional (R-07, RD 14.5.4). SinDeducciones salta el pipeline
// (Fees in Error / R-16; el proceso lo decide fuera).
type deducciones struct {
	Admin   decimal.Decimal
	Social  decimal.Decimal
	Reserva decimal.Decimal
	Neto    decimal.Decimal
}

func aplicarDeducciones(bolsa Bolsa, snap Snapshot, sinDeducciones bool) (deducciones, error) {
	bruto := bolsa.Bruto
	if sinDeducciones {
		return deducciones{Neto: bruto}, nil
	}

	// Cero = ausente (ADR 0004), igual que pctGrupo. SinDeducciones cubre el
	// salto legitimo de Fees in Error (R-16); no se inventa un 0% silencioso.
	if err := exigirPositivo("admin_pct", snap.AdminPct); err != nil {
		return deducciones{}, err
	}
	if err := exigirPositivo("social_pct", snap.SocialPct); err != nil {
		return deducciones{}, err
	}
	if err := exigirPositivo("reserva_pct", snap.ReservaPct); err != nil {
		return deducciones{}, err
	}

	admin := pctDe(bruto, snap.AdminPct)
	social := pctDe(bruto, snap.SocialPct)
	reserva := decimal.Zero
	if bolsa.Circuito == recaudo.Nacional {
		reserva = pctDe(bruto, snap.ReservaPct)
	}
	neto := bruto.Sub(admin).Sub(social).Sub(reserva)
	if neto.IsNegative() {
		return deducciones{}, fmt.Errorf("%w: deducciones superan el bruto", ErrRepartoInvalido)
	}
	return deducciones{Admin: admin, Social: social, Reserva: reserva, Neto: neto}, nil
}

// indiceDeclaraciones indexa por obra. Una obra sin entrada se trata como
// incompleta (R-04).
func indiceDeclaraciones(decls []repertorio.Declaracion) map[string]repertorio.Declaracion {
	out := make(map[string]repertorio.Declaracion, len(decls))
	for _, d := range decls {
		out[d.ObraID] = d
	}
	return out
}

func obrasOrdenadas(puntos map[string]decimal.Decimal) []string {
	ids := make([]string, 0, len(puntos))
	for id := range puntos {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func aplicarDeclaraciones(
	obras []LineaObra,
	decls map[string]repertorio.Declaracion,
) (titulares []LineaTitular, retenido decimal.Decimal, obrasOut []LineaObra) {
	obrasOut = make([]LineaObra, len(obras))
	copy(obrasOut, obras)
	for i := range obrasOut {
		o := &obrasOut[i]
		d, ok := decls[o.ObraID]
		if !ok || !d.Completa() {
			o.Retenida = true
			o.Motivo = "declaracion_incompleta"
			retenido = retenido.Add(o.Importe)
			continue
		}
		for _, p := range d.Partes {
			// R-01: solo se emite linea con IPI (persona natural ya validada
			// al construir la declaracion). Sin IPI no hay orden.
			if p.IPI == "" {
				continue
			}
			imp := redondearDinero(o.Importe.Mul(p.Porcentaje).Div(decimal.NewFromInt(100)))
			titulares = append(titulares, LineaTitular{
				ObraID:     o.ObraID,
				TitularID:  p.TitularID,
				IPI:        p.IPI,
				Porcentaje: p.Porcentaje,
				Importe:    imp,
			})
		}
	}
	sort.Slice(titulares, func(i, j int) bool {
		if titulares[i].ObraID != titulares[j].ObraID {
			return titulares[i].ObraID < titulares[j].ObraID
		}
		return titulares[i].TitularID < titulares[j].TitularID
	})
	return titulares, retenido, obrasOut
}

// sumaTitulares agrega importes de titular. El residuo de redondeo de esas
// lineas contra el importe repartido (no retenido) se calcula en Reparto y
// queda en Residuo, no se empuja a la ultima linea.
func sumaTitulares(tt []LineaTitular) decimal.Decimal {
	s := decimal.Zero
	for _, t := range tt {
		s = s.Add(t.Importe)
	}
	return s
}

func sumaObrasNoRetenidas(oo []LineaObra) decimal.Decimal {
	s := decimal.Zero
	for _, o := range oo {
		if !o.Retenida {
			s = s.Add(o.Importe)
		}
	}
	return s
}
