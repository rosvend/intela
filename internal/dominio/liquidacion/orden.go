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

	// ErrCircuitoAusente: una orden pertenece a UN circuito y hay que decir a
	// cual.
	//
	// El circuito no es decorativo: desde el ADR 0019 es parte de la identidad
	// de la orden -- hay una por (titular, periodo, circuito) -- porque el
	// nacional y el internacional son dos recorridos distintos del mismo
	// periodo y no se suman. Una orden sin circuito no se puede colocar en esa
	// clave, y el UNIQUE del esquema la dejaria pasar con la cadena vacia como
	// si fuera un tercer circuito.
	//
	// Cuales son los dos valores legales NO se comprueba aqui: los declara
	// recaudo.Circuito y los hace cumplir el CHECK de `ordenes_pago.circuito`.
	// Este paquete no importa recaudo (ver doc.go), y repetir la lista seria
	// una segunda definicion que se puede desincronizar de la primera.
	ErrCircuitoAusente = errors.New("circuito ausente")
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
// Es UNA por (TitularID, Periodo, Circuito), no una por corrida (ADR 0019):
// un periodo puede cerrarse con varias corridas del mismo circuito -- una por
// bolsa de pagador -- y el umbral de menor cuantia de R-11 se mide sobre lo
// que el titular cobra por ese periodo, no sobre cada trozo por separado.
// Partirlo en una orden por corrida difiere saldos que juntos si pasan el 2%.
//
// # De donde salen las tres cifras
//
// El motor aporta el neto POR TITULAR; bruto y el desglose no se reciben de
// el, se reconstruyen aqui. Neto es bruto menos la suma de las deducciones,
// y las deducciones son las de la corrida prorrateadas contra el neto de la
// corrida ([Prorratear]). Es esa cuenta -- y no una preferencia entre dos
// cifras que discrepen -- la que ata la orden al cierre de la corrida: la
// tasa que la orden muestra es la misma que la corrida aplico.
type OrdenDePago struct {
	ID string

	// ProcesoID es la corrida de REFERENCIA, no la unica que aporto. Cuando
	// varias corridas del mismo periodo y circuito contribuyen, es la primera
	// por orden lexicografico; Procesos lleva la lista completa.
	ProcesoID string

	// Procesos son todas las corridas cuyas lineas entraron en esta orden,
	// ordenadas. Existe porque sin ella una orden agregada no dice de donde
	// salio su bruto, y el ADR 0006 pregunta justamente eso.
	Procesos []string

	TitularID string
	Periodo   string

	// Circuito es 'nacional' o 'internacional'. Texto y no un tipo de
	// recaudo: este paquete no importa recaudo (ver doc.go).
	Circuito string

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

// DatosOrden es lo que hace falta para emitir una orden.
//
// Es un struct y no siete parametros posicionales porque seis de ellos son
// cadenas seguidas -- id, proceso, titular, periodo, circuito, dia -- y ese es
// exactamente el sitio donde dos argumentos intercambiados compilan sin
// quejarse y producen una orden del circuito equivocado.
type DatosOrden struct {
	ID          string
	ProcesoID   string
	Procesos    []string
	TitularID   string
	Periodo     string
	Circuito    string
	EnviadaDia  string
	Bruto       decimal.Decimal
	Deducciones []Deduccion
}

// NuevaOrden construye una orden en estado enviada. Las deducciones no
// pueden ser negativas; el neto es bruto menos su suma, y si eso queda
// bajo cero la orden no se emite.
func NuevaOrden(d DatosOrden) (OrdenDePago, error) {
	if d.Circuito == "" {
		return OrdenDePago{}, ErrCircuitoAusente
	}
	if d.Bruto.IsNegative() {
		return OrdenDePago{}, ErrBrutoNegativo
	}
	deducciones := d.Deducciones
	if deducciones == nil {
		deducciones = []Deduccion{}
	}
	suma := decimal.Zero
	for _, ded := range deducciones {
		if ded.Monto.IsNegative() {
			return OrdenDePago{}, fmt.Errorf("%w: %s", ErrDeduccionNegativa, ded.Concepto)
		}
		suma = suma.Add(ded.Monto)
	}
	neto := d.Bruto.Sub(suma)
	if neto.IsNegative() {
		return OrdenDePago{}, ErrNetoNegativo
	}
	procesos := d.Procesos
	if len(procesos) == 0 {
		// Una sola corrida contribuyente no es un caso especial: es la lista
		// de uno. Dejarla vacia obligaria a cada lector a saber que entonces
		// hay que mirar ProcesoID.
		procesos = []string{d.ProcesoID}
	}
	return OrdenDePago{
		ID:          d.ID,
		ProcesoID:   d.ProcesoID,
		Procesos:    append([]string{}, procesos...),
		TitularID:   d.TitularID,
		Periodo:     d.Periodo,
		Circuito:    d.Circuito,
		Bruto:       d.Bruto,
		Deducciones: deducciones,
		Neto:        neto,
		Estado:      EstadoEnviada,
		EnviadaDia:  d.EnviadaDia,
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
