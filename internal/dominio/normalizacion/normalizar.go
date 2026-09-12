package normalizacion

import (
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// Normalizar lleva una fila cruda al esquema canonico, o explica por que no
// puede. Nunca descarta: el llamador recibe o un [Uso] o una [Revision], y
// en los dos casos los campos de identidad viajan para que la cola de
// revision sepa de que linea se trata.
func Normalizar(f Fila, p Parametros) (Uso, *Revision) {
	u := Uso{
		ID:         strings.TrimSpace(f.ID),
		Fuente:     strings.TrimSpace(f.Fuente),
		Titulo:     strings.TrimSpace(f.Titulo),
		TituloOrig: strings.TrimSpace(f.TituloOrig),
		IDsFuente:  strings.TrimSpace(f.IDsFuente),
		TipoObra:   strings.TrimSpace(f.TipoObra),
		Autopromo:  f.Autopromo,
	}

	mod := strings.ToLower(strings.TrimSpace(f.Modalidad))
	switch mod {
	case ModalidadTV, ModalidadCine, ModalidadOTT, ModalidadHotel:
		u.Modalidad = mod
	default:
		return u, &Revision{
			Codigo:  CodigoModalidadInvalida,
			Campo:   "modalidad",
			Detalle: "modalidad " + strconv.Quote(f.Modalidad) + " fuera de tv|cine|ott|hotel",
		}
	}

	fecha, err := ParsearFecha(f.Fecha)
	if err != nil {
		return u, &Revision{
			Codigo:  CodigoFechaInparseable,
			Campo:   "fecha",
			Detalle: err.Error(),
		}
	}
	u.Fecha = fecha

	horaCruda := f.Hora
	if strings.TrimSpace(horaCruda) == "" {
		// Un serial de Excel con fraccion (45657.8333) es fecha Y hora en
		// la misma celda. Si Hora no vino aparte, la fraccion es el objeto
		// de tiempo de la parrilla.
		if _, frac, ok := partirNumero(strings.TrimSpace(f.Fecha)); ok && frac > 0 {
			horaCruda = strconv.FormatFloat(frac, 'f', -1, 64)
		}
	}
	hora, err := ParsearHora(horaCruda)
	if err != nil {
		return u, &Revision{
			Codigo:  CodigoFechaInparseable,
			Campo:   "hora",
			Detalle: err.Error(),
		}
	}
	u.Hora = hora

	duracion, rev := medidaNoNegativa(f.Duracion, "duracion")
	if rev != nil {
		return u, rev
	}
	rating, rev := medidaNoNegativa(f.Rating, "rating")
	if rev != nil {
		return u, rev
	}
	taquilla, rev := medidaNoNegativa(f.Taquilla, "taquilla")
	if rev != nil {
		return u, rev
	}
	vistas, rev := medidaNoNegativa(f.Vistas, "vistas")
	if rev != nil {
		return u, rev
	}
	minutos, rev := medidaNoNegativa(f.MinutosVistos, "minutos_vistos")
	if rev != nil {
		return u, rev
	}
	pb, rev := medidaNoNegativa(f.PB, "pb")
	if rev != nil {
		return u, rev
	}

	emisiones, rev := parsearEmisiones(f.Emisiones)
	if rev != nil {
		return u, rev
	}
	u.Emisiones = emisiones
	u.Rating = rating
	u.Vistas = vistas
	u.MinutosVistos = minutos
	u.PB = pb

	if f.Autopromo && u.Modalidad == ModalidadTV {
		// RD 9.1.1: los avances de programacion propia no computan. La fila
		// queda, con duracion cero: perderla seria borrar la evidencia.
		u.DuracionMin = decimal.Zero
		return u, nil
	}

	if u.Modalidad == ModalidadTV {
		min, rev := duracionTV(duracion, f.UnidadDuracion, p)
		if rev != nil {
			return u, rev
		}
		u.DuracionMin = min
	} else {
		u.DuracionMin = duracion
	}

	if u.Modalidad == ModalidadCine {
		enBase, rev := taquillaEnBase(taquilla, f.Moneda, p)
		if rev != nil {
			return u, rev
		}
		u.Taquilla = enBase
	} else if moneda := normalizarCodigoMoneda(f.Moneda); moneda != "" {
		// Un reporte de TV/OTT no trae importes. Si aparece una moneda, o el
		// mapeo se equivoco de columna o la fuente mando un dato que este
		// esquema no puede ponderar: a revision, no a cero.
		return u, &Revision{
			Codigo:  CodigoMonedaDesconocida,
			Campo:   "moneda",
			Detalle: "moneda " + moneda + " en una fila " + u.Modalidad + " que no trae importes",
		}
	}

	return u, nil
}

func medidaNoNegativa(s, campo string) (decimal.Decimal, *Revision) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, nil
	}
	n, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, &Revision{
			Codigo:  CodigoMedidaInvalida,
			Campo:   campo,
			Detalle: campo + " " + strconv.Quote(s) + ": no es un numero",
		}
	}
	if n.IsNegative() {
		return decimal.Zero, &Revision{
			Codigo:  CodigoMedidaInvalida,
			Campo:   campo,
			Detalle: campo + " negativa: " + n.String() + ": no se pone a cero",
		}
	}
	return n, nil
}

func parsearEmisiones(s string) (int64, *Revision) {
	s = strings.TrimSpace(s)
	if s == "" {
		// La granularidad de la parrilla es la emision, no la obra. El cero
		// de Go es "no vino", no "cero emisiones". GuardarUsos pone el mismo
		// default; hacerlo aqui deja el Uso canonico listo sin una segunda
		// opinion aguas abajo.
		return 1, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, &Revision{
			Codigo:  CodigoMedidaInvalida,
			Campo:   "emisiones",
			Detalle: "emisiones " + strconv.Quote(s) + ": no es un entero",
		}
	}
	if n < 0 {
		return 0, &Revision{
			Codigo:  CodigoMedidaInvalida,
			Campo:   "emisiones",
			Detalle: "emisiones negativas: " + s + ": no se pone a cero",
		}
	}
	if n == 0 {
		return 1, nil
	}
	return n, nil
}
