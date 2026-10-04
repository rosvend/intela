package reparto

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Origen de un factor del desglose.
const (
	OrigenParametro = "parametro" // del snapshot congelado de la corrida (ADR 0004)
	OrigenUso       = "uso"       // de la fila canonica del uso, ya normalizada (RD 9.1.1(c) ya aplicado)
)

// Nombres de factor. Son el vocabulario del contrato: no renombrar.
const (
	FactorPonderacion   = "ponderacion"
	FactorDuracion      = "duracion_min"
	FactorRating        = "rating"
	FactorEmisiones     = "emisiones"
	FactorEspectadores  = "espectadores"
	FactorTaquilla      = "taquilla"
	FactorExhibiciones  = "exhibiciones"
	FactorPB            = "pb"
	FactorWa            = "wa"
	FactorMinutosVistos = "minutos_vistos"
	FactorWb            = "wb"
	FactorVistas        = "vistas"
	FactorWc            = "wc"
)

// Factor es un multiplicando de la formula, con de donde salio.
type Factor struct {
	Nombre string
	Valor  decimal.Decimal
	Origen string
}

// Termino es un producto de factores. Producto es exacto, sin redondear.
type Termino struct {
	Factores []Factor
	Producto decimal.Decimal
}

// Desglose es la aritmetica de los puntos de un uso: Puntos = suma de los
// Producto de Terminos (#187). Es lo que el recibo itemiza; el motor suma
// exactamente estos mismos terminos, salvo los usos que suscripcion y hotel
// descartan por R-27 (ver doc.go).
type Desglose struct {
	Formula  string
	Terminos []Termino
	Puntos   decimal.Decimal
}

// DesglosarUso devuelve los factores que producen los puntos de un uso.
// Funcion pura (ADR 0005). Mismos errores que el motor para el mismo uso.
func DesglosarUso(u Uso, snap Snapshot) (Desglose, error) {
	var (
		formula string
		ts      []Termino
		err     error
	)
	switch u.Modalidad {
	case TV:
		formula = "RD 9.1.1"
		ts, err = terminosTV(u, snap)
	case Suscripcion:
		formula = "RD 9.5"
		ts, err = terminosTV(u, snap)
	case Hotel:
		formula = "RD 9.6"
		ts, err = terminosTV(u, snap)
	case Cine:
		formula = "RD 9.2"
		ts, err = terminosCineTeatro(u, snap)
	case Teatro:
		formula = "RD 9.3"
		ts, err = terminosCineTeatro(u, snap)
	case Transporte:
		formula = "RD 9.4"
		ts, err = terminosTransporte(u)
	case OTT:
		formula = "RD 9.7"
		ts, err = terminosOTT(u, snap)
	default:
		return Desglose{}, fmt.Errorf("%w: %q", ErrModalidadDesconocida, u.Modalidad)
	}
	if err != nil {
		return Desglose{}, err
	}
	return Desglose{Formula: formula, Terminos: ts, Puntos: sumar(ts)}, nil
}

// nuevoTermino multiplica los factores en el orden dado, sin redondear.
func nuevoTermino(fs ...Factor) Termino {
	p := decimal.NewFromInt(1)
	for _, f := range fs {
		p = p.Mul(f.Valor)
	}
	return Termino{Factores: fs, Producto: p}
}

// sumar es la suma exacta de los productos de los terminos.
func sumar(ts []Termino) decimal.Decimal {
	s := decimal.Zero
	for _, t := range ts {
		s = s.Add(t.Producto)
	}
	return s
}

// terminosTV es la formula de RD 9.1.1: Pond(tipo) * Duracion * Rating * Emisiones.
func terminosTV(u Uso, snap Snapshot) ([]Termino, error) {
	pond, err := ponderacionTipo(u.TipoObra, u.ObraID, snap)
	if err != nil {
		return nil, err
	}
	if u.DuracionMin.IsNegative() || u.Rating.IsNegative() {
		return nil, fmt.Errorf("%w: medida negativa en obra %q", ErrRepartoInvalido, u.ObraID)
	}
	emisiones := decimal.NewFromInt(u.Emisiones)
	if emisiones.IsNegative() {
		return nil, fmt.Errorf("%w: medida negativa en obra %q", ErrRepartoInvalido, u.ObraID)
	}
	return []Termino{nuevoTermino(
		Factor{FactorPonderacion, pond, OrigenParametro},
		Factor{FactorDuracion, u.DuracionMin, OrigenUso},
		Factor{FactorRating, u.Rating, OrigenUso},
		Factor{FactorEmisiones, emisiones, OrigenUso},
	)}, nil
}

// terminosCineTeatro pondera por espectadores o taquilla segun el snapshot (P-18).
func terminosCineTeatro(u Uso, snap Snapshot) ([]Termino, error) {
	var (
		w      decimal.Decimal
		nombre string
	)
	switch snap.BaseCineTeatro {
	case BaseEspectadores:
		w, nombre = u.Espectadores, FactorEspectadores
	case BaseTaquilla:
		w, nombre = u.Taquilla, FactorTaquilla
	default:
		// El mensaje nombra tambien la clave de `parametros` que la llena:
		// es la que el operador reconoce y carga (ADR 0004), no el campo.
		return nil, fmt.Errorf("%w: base_cine_teatro (clave %s, P-18)", ErrParametroAusente, ClaveBaseCineTeatro)
	}
	if w.IsNegative() {
		return nil, fmt.Errorf("%w: medida negativa en obra %q", ErrRepartoInvalido, u.ObraID)
	}
	return []Termino{nuevoTermino(Factor{nombre, w, OrigenUso})}, nil
}

// terminosTransporte pondera por exhibiciones. Cero exhibiciones = cero peso.
func terminosTransporte(u Uso) ([]Termino, error) {
	if u.Exhibiciones < 0 {
		return nil, fmt.Errorf("%w: medida negativa en obra %q", ErrRepartoInvalido, u.ObraID)
	}
	return []Termino{nuevoTermino(Factor{FactorExhibiciones, decimal.NewFromInt(u.Exhibiciones), OrigenUso})}, nil
}

// terminosOTT es Pi = PB*Wa + DU*Wb + V*Wc, un termino por sumando.
func terminosOTT(u Uso, snap Snapshot) ([]Termino, error) {
	if err := exigirPositivo("ott.wa", snap.Wa); err != nil {
		return nil, err
	}
	if err := exigirPositivo("ott.wb", snap.Wb); err != nil {
		return nil, err
	}
	if err := exigirPositivo("ott.wc", snap.Wc); err != nil {
		return nil, err
	}
	return []Termino{
		nuevoTermino(Factor{FactorPB, u.PB, OrigenUso}, Factor{FactorWa, snap.Wa, OrigenParametro}),
		nuevoTermino(Factor{FactorMinutosVistos, u.MinutosVistos, OrigenUso}, Factor{FactorWb, snap.Wb, OrigenParametro}),
		nuevoTermino(Factor{FactorVistas, u.Vistas, OrigenUso}, Factor{FactorWc, snap.Wc, OrigenParametro}),
	}, nil
}
