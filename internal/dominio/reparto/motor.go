package reparto

import (
	"fmt"
	"sort"

	"github.com/rosvend/intela/internal/dominio/repertorio"
	"github.com/shopspring/decimal"
)

type Modalidad string

const (
	TV     Modalidad = "tv"
	Cine   Modalidad = "cine"
	OTT    Modalidad = "ott"
	Hotel  Modalidad = "hotel"
)

type Snapshot struct {
	AdminPct     decimal.Decimal
	SocialPct    decimal.Decimal
	ReservaPct   decimal.Decimal
	PondCine     decimal.Decimal
	PondUnitario decimal.Decimal
	PondSerie    decimal.Decimal
	PondSketch   decimal.Decimal
	Wa           decimal.Decimal
	Wb           decimal.Decimal
	Wc           decimal.Decimal
	UmbralMatch  decimal.Decimal
	Reglamento   string
}

type Uso struct {
	ObraID     string
	Modalidad  Modalidad
	TipoObra   string
	DuracionMin decimal.Decimal
	Emisiones  int64
	Rating     decimal.Decimal
	Taquilla   decimal.Decimal
	Vistas     decimal.Decimal
	MinutosVistos decimal.Decimal
	PB         decimal.Decimal
}

type Bolsa struct {
	UsuarioID string
	Periodo   string
	Circuito  string
	Bruto     decimal.Decimal
}

type LineaObra struct {
	ObraID     string
	Puntos     decimal.Decimal
	Importe    decimal.Decimal
	Retenida   bool
	Motivo     string
}

type LineaTitular struct {
	ObraID     string
	TitularID  string
	IPI        string
	Porcentaje decimal.Decimal
	Importe    decimal.Decimal
}

type Resultado struct {
	Neto       decimal.Decimal
	Admin      decimal.Decimal
	Social     decimal.Decimal
	Reserva    decimal.Decimal
	Obras      []LineaObra
	Titulares  []LineaTitular
	ValorPunto decimal.Decimal
}

func Deducciones(bolsa Bolsa, snap Snapshot) (admin, social, reserva, neto decimal.Decimal) {
	admin = bolsa.Bruto.Mul(snap.AdminPct).Div(decimal.NewFromInt(100)).Round(2)
	social = bolsa.Bruto.Mul(snap.SocialPct).Div(decimal.NewFromInt(100)).Round(2)
	base := bolsa.Bruto.Sub(admin).Sub(social)
	if bolsa.Circuito == "nacional" {
		reserva = base.Mul(snap.ReservaPct).Div(decimal.NewFromInt(100)).Round(2)
	}
	neto = base.Sub(reserva)
	return
}

func ponderacion(tipo string, snap Snapshot) decimal.Decimal {
	switch tipo {
	case "cinematografica":
		return snap.PondCine
	case "unitario":
		return snap.PondUnitario
	case "serie", "telenovela":
		return snap.PondSerie
	case "sketches", "sketch":
		return snap.PondSketch
	default:
		return snap.PondSerie
	}
}

func Puntos(u Uso, snap Snapshot) decimal.Decimal {
	switch u.Modalidad {
	case TV, Hotel:
		pond := ponderacion(u.TipoObra, snap)
		em := decimal.NewFromInt(u.Emisiones)
		if em.IsZero() {
			em = decimal.NewFromInt(1)
		}
		return pond.Mul(u.DuracionMin).Mul(u.Rating).Mul(em)
	case Cine:
		return u.Taquilla
	case OTT:
		return u.PB.Mul(snap.Wa).Add(u.MinutosVistos.Mul(snap.Wb)).Add(u.Vistas.Mul(snap.Wc))
	default:
		return decimal.Zero
	}
}

func DuracionArtistica(reportada decimal.Decimal) decimal.Decimal {
	return reportada.Mul(decimal.NewFromFloat(0.8))
}

func Repartir(bolsa Bolsa, usos []Uso, declaraciones map[string]repertorio.Declaracion, snap Snapshot) Resultado {
	admin, social, reserva, neto := Deducciones(bolsa, snap)
	out := Resultado{Admin: admin, Social: social, Reserva: reserva, Neto: neto}

	type agg struct {
		obra   string
		puntos decimal.Decimal
	}
	porObra := map[string]decimal.Decimal{}
	for _, u := range usos {
		porObra[u.ObraID] = porObra[u.ObraID].Add(Puntos(u, snap))
	}
	ids := make([]string, 0, len(porObra))
	for id := range porObra {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	totalP := decimal.Zero
	for _, id := range ids {
		totalP = totalP.Add(porObra[id])
	}
	if totalP.IsZero() {
		return out
	}
	out.ValorPunto = neto.Div(totalP)

	var lineas []LineaObra
	asignado := decimal.Zero
	for i, id := range ids {
		pts := porObra[id]
		imp := pts.Mul(out.ValorPunto).Round(2)
		if i == len(ids)-1 {
			imp = neto.Sub(asignado)
		} else {
			asignado = asignado.Add(imp)
		}
		l := LineaObra{ObraID: id, Puntos: pts, Importe: imp}
		dec, ok := declaraciones[id]
		if !ok || !dec.Completa() {
			l.Retenida = true
			l.Motivo = "declaracion_incompleta"
		}
		lineas = append(lineas, l)
	}
	out.Obras = lineas

	for _, l := range lineas {
		if l.Retenida {
			continue
		}
		dec := declaraciones[l.ObraID]
		rest := l.Importe
		for i, p := range dec.Partes {
			imp := l.Importe.Mul(p.Porcentaje).Div(decimal.NewFromInt(100)).Round(2)
			if i == len(dec.Partes)-1 {
				imp = rest
			} else {
				rest = rest.Sub(imp)
			}
			out.Titulares = append(out.Titulares, LineaTitular{
				ObraID: l.ObraID, TitularID: p.TitularID, IPI: p.IPI,
				Porcentaje: p.Porcentaje, Importe: imp,
			})
		}
	}
	return out
}

func RepartoInternacional(lineas []LineaTitular) []LineaTitular {
	out := make([]LineaTitular, len(lineas))
	copy(out, lineas)
	return out
}

func DebeFallarPorParametroAusente(u Uso, snap Snapshot) error {
	if u.Modalidad == TV && u.Rating.IsZero() {
		return fmt.Errorf("rating ausente: no se inventa (RD 9.1.1)")
	}
	if u.Modalidad == OTT && (snap.Wa.IsZero() && snap.Wb.IsZero() && snap.Wc.IsZero()) {
		return fmt.Errorf("Wa/Wb/Wc ausentes: no se inventan (RD 9.7)")
	}
	return nil
}
