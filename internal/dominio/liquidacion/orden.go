package liquidacion

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// PlazoAceptacionDias es la ventana de R-10 / RD 13.2: quince dias
// calendario desde el envio. Calendario, no habiles; R-22 (reclamaciones)
// usa habiles y no se unifican.
const PlazoAceptacionDias = 15

// Conceptos de deduccion que RD 13.2 obliga a mostrar itemizados. No son
// la lista cerrada: un anticipo (R-33) entra con su propio concepto sin
// tocar estos.
const (
	ConceptoAdministracion = "gastos_administrativos"
	ConceptoSocial         = "bienestar_social"
	ConceptoReserva        = "reserva_errores_tecnicos"
)

// Estado de una orden. La transicion enviada -> aceptada_por_silencio /
// diferida la decide [OrdenDePago.EvaluarPlazo], no un UPDATE suelto.
type Estado string

const (
	EstadoEnviada             Estado = "enviada"
	EstadoAceptada            Estado = "aceptada"
	EstadoAceptadaPorSilencio Estado = "aceptada_por_silencio"
	EstadoDiferida            Estado = "diferida"
	EstadoAcumulada           Estado = "acumulada"
	EstadoObjetada            Estado = "objetada"
)

var (
	// ErrBrutoNegativo: no hay orden de pago con un bruto bajo cero.
	ErrBrutoNegativo = errors.New("bruto negativo")

	// ErrDeduccionNegativa: una deduccion que suma no es una deduccion.
	ErrDeduccionNegativa = errors.New("deduccion negativa")

	// ErrNetoNegativo: las deducciones no pueden superar el bruto.
	ErrNetoNegativo = errors.New("neto negativo")
)

// Deduccion es un renglon del desglose. El concepto viaja con el monto
// para que OE-4 y OE-6 no tengan que reconstruirlo.
type Deduccion struct {
	Concepto string
	Monto    decimal.Decimal
}

// Documentos que R-12 / RD 13.1.6 exige para cobrar. Su ausencia bloquea
// el pago, no la liquidacion: se puede liquidar a quien no puede cobrar
// todavia.
type Documentos struct {
	RUT                   bool
	CertificacionBancaria bool
}

// Completos es la puerta de R-12: hacen falta los dos. La autorizacion a
// tercero es otro camino y no se finge aqui.
func (d Documentos) Completos() bool {
	return d.RUT && d.CertificacionBancaria
}

// OrdenDePago es lo que ve el titular: bruto, cada deduccion, neto.
//
// Neto se calcula aqui como bruto menos la suma de las deducciones, y no
// se recibe del motor. Lo que el motor aporta es el neto por titular; la
// capa de aplicacion reconstruye bruto y el desglose a partir de esa cifra
// y de las deducciones de la corrida, para que si algo no cuadra, la
// corrida mande.
type OrdenDePago struct {
	ID          string
	ProcesoID   string
	TitularID   string
	Periodo     string
	Bruto       decimal.Decimal
	Deducciones []Deduccion
	Neto        decimal.Decimal
	Estado      Estado

	// EnviadaDia es YYYY-MM-DD. El instante que la produjo entra por
	// PuertoReloj en aplicacion; este paquete no importa time.
	EnviadaDia string

	// Arrastres son las ordenes diferidas (R-11) cuyo neto se incorporo
	// a esta. Vacio si no hay saldo arrastrado.
	Arrastres []string
}

// NuevaOrden construye una orden en estado enviada. Las deducciones no
// pueden ser negativas; el neto es bruto menos su suma, y si eso queda
// bajo cero la orden no se emite.
func NuevaOrden(id, procesoID, titularID, periodo, enviadaDia string, bruto decimal.Decimal, deducciones []Deduccion) (OrdenDePago, error) {
	if bruto.IsNegative() {
		return OrdenDePago{}, ErrBrutoNegativo
	}
	if deducciones == nil {
		deducciones = []Deduccion{}
	}
	suma := decimal.Zero
	for _, d := range deducciones {
		if d.Monto.IsNegative() {
			return OrdenDePago{}, fmt.Errorf("%w: %s", ErrDeduccionNegativa, d.Concepto)
		}
		suma = suma.Add(d.Monto)
	}
	neto := bruto.Sub(suma)
	if neto.IsNegative() {
		return OrdenDePago{}, ErrNetoNegativo
	}
	return OrdenDePago{
		ID:          id,
		ProcesoID:   procesoID,
		TitularID:   titularID,
		Periodo:     periodo,
		Bruto:       bruto,
		Deducciones: deducciones,
		Neto:        neto,
		Estado:      EstadoEnviada,
		EnviadaDia:  enviadaDia,
		Arrastres:   []string{},
	}, nil
}

// UmbralMenorCuantia es el 2% de un SMMLV (R-11, RD 13.3). El SMMLV llega
// como dato, no como constante: cambia por ano y lo aprueba el Gobierno
// (ADR 0004).
func UmbralMenorCuantia(smmlv decimal.Decimal) decimal.Decimal {
	return smmlv.Mul(decimal.NewFromInt(2)).Div(decimal.NewFromInt(100))
}

// EsPagable: aceptada (por respuesta o por silencio) y con RUT mas
// certificacion bancaria en expediente. Una diferida no se paga: se
// arrastra. Una enviada tampoco: el titular todavia puede objetar.
func (o OrdenDePago) EsPagable(docs Documentos) bool {
	if !docs.Completos() {
		return false
	}
	switch o.Estado {
	case EstadoAceptada, EstadoAceptadaPorSilencio:
		return !o.Neto.IsNegative() && !o.Neto.IsZero()
	default:
		return false
	}
}

// RegistrarRespuesta aplica la voluntad del titular dentro de la ventana.
//
// quierePago cubre el caso de R-11 en el que el titular pide el giro aunque
// el neto no llegue al umbral. Una objecion no paga ni arrastra: queda
// objetada para que una reclamacion (otro modulo) la resuelva.
func (o OrdenDePago) RegistrarRespuesta(quierePago bool) OrdenDePago {
	if o.Estado != EstadoEnviada {
		return o
	}
	if quierePago {
		o.Estado = EstadoAceptada
		return o
	}
	o.Estado = EstadoObjetada
	return o
}

// IncorporarArrastre suma el neto de una orden diferida (R-11) a esta.
//
// Se suma al bruto y al neto por igual, sin volver a deducir: esas
// deducciones ya se aplicaron en el periodo de origen. Deducir otra vez
// alteraria la cifra que la corrida de entonces ya cerro.
func (o OrdenDePago) IncorporarArrastre(anterior OrdenDePago) OrdenDePago {
	if anterior.Estado != EstadoDiferida {
		return o
	}
	o.Bruto = o.Bruto.Add(anterior.Neto)
	o.Neto = o.Neto.Add(anterior.Neto)
	o.Arrastres = append(append([]string{}, o.Arrastres...), anterior.ID)
	return o
}

// MarcarAcumulada cierra una diferida que ya se incorporo al periodo
// siguiente, para no arrastrarla dos veces.
func (o OrdenDePago) MarcarAcumulada() OrdenDePago {
	if o.Estado == EstadoDiferida {
		o.Estado = EstadoAcumulada
	}
	return o
}
