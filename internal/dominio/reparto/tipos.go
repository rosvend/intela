package reparto

import "github.com/shopspring/decimal"

// Modalidad del acto de comunicacion publica (RD 8). Determina con que
// formula se valoriza un uso, no cuanto vale.
type Modalidad string

const (
	TV    Modalidad = "tv"
	Cine  Modalidad = "cine"
	OTT   Modalidad = "ott"
	Hotel Modalidad = "hotel"
)

// Circuito de la corrida. Son dos recorridos distintos, no una variante de
// uno: el internacional no valoriza por puntos (RD 7.4). Ver ADR 0008.
type Circuito string

const (
	Nacional      Circuito = "nacional"
	Internacional Circuito = "internacional"
)

// Etapa del sistema de distribucion (RD 13.5). Cada una tiene dueno y
// verificacion; el dinero no sale con una sola firma.
type Etapa string

const (
	EtapaRecaudo            Etapa = "recaudo"
	EtapaDeducciones        Etapa = "deducciones"
	EtapaImporteObra        Etapa = "importe_obra"
	EtapaImporteTitular     Etapa = "importe_titular"
	EtapaLiquidacionParcial Etapa = "liquidacion_parcial"
	EtapaVerificacion       Etapa = "verificacion"
	EtapaLiquidacionFinal   Etapa = "liquidacion_final"
	EtapaPagoRegistro       Etapa = "pago_registro"
	EtapaFeesInError        Etapa = "fees_in_error"
	EtapaAuditoria          Etapa = "auditoria"
)

// Firma de una compuerta, sobre una revision concreta del proceso. Que sea
// sobre la revision y no sobre el proceso es deliberado: un rechazo sube la
// revision, y las firmas de la anterior dejan de contar.
type Firma struct {
	Rol      string
	ActorID  string
	SobreRev int
}

// Snapshot congelado de los parametros normativos vigentes en la fecha del
// periodo. Se resuelve al ABRIR el proceso y queda referenciado por la
// corrida; recalcular no vuelve a resolverlo (ADR 0004, ADR 0005). Sin esto
// una corrida no se reproduce bit a bit anos despues.
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

// Uso de una obra en un periodo, ya identificado.
//
// No tiene campo de dinero, y es el invariante mas importante del paquete: un
// reporte de uso PONDERA la bolsa, no la aporta. Anadir aqui un importe hace
// compilable la operacion de sumar dinero por fila, que es exactamente lo que
// el reglamento no permite.
type Uso struct {
	ObraID        string
	Modalidad     Modalidad
	TipoObra      string
	DuracionMin   decimal.Decimal
	Emisiones     int64
	Rating        decimal.Decimal
	Taquilla      decimal.Decimal
	Vistas        decimal.Decimal
	MinutosVistos decimal.Decimal
	PB            decimal.Decimal
}

// Bolsa a repartir en un periodo. Es lo unico que Recaudo pasa aguas abajo:
// Reparto no conoce Usuario, Convenio ni Tarifa (ADR 0003).
type Bolsa struct {
	UsuarioID string
	Periodo   string
	Circuito  Circuito
	Bruto     decimal.Decimal
}

// LineaObra es lo que le toca a una obra antes de mirar su declaracion.
//
// Retenida marca que la declaracion no suma 100% o le faltan IPI. El importe
// de una linea retenida va a Resultado.Retenido, no desaparece: retener es
// mover a reserva (R-04, RD 13.1.3).
type LineaObra struct {
	ObraID   string
	Puntos   decimal.Decimal
	Importe  decimal.Decimal
	Retenida bool
	Motivo   string
}

// LineaTitular es una orden de pago en potencia. Solo se emite a escritor
// persona natural (R-01, RD 4.5).
type LineaTitular struct {
	ObraID     string
	TitularID  string
	IPI        string
	Porcentaje decimal.Decimal
	Importe    decimal.Decimal
}

// Resultado de una corrida.
//
// La invariante de cierre que el motor tiene que probar:
//
//	Neto == suma(Titulares.Importe) + Retenido + Residuo
//
// Retenido y Residuo existen como campos propios precisamente para que esa
// igualdad se pueda comprobar. El ADR 0005 pide que el residuo de redondeo
// sea explicito y reproducible, no un sobrante que absorbe la ultima linea.
//
// Snapshot y Reglamento guardan la procedencia: sin ellos no se puede
// defender una cifra ante una auditoria de RD 16.
type Resultado struct {
	Neto       decimal.Decimal
	Admin      decimal.Decimal
	Social     decimal.Decimal
	Reserva    decimal.Decimal
	Retenido   decimal.Decimal
	Residuo    decimal.Decimal
	ValorPunto decimal.Decimal
	Obras      []LineaObra
	Titulares  []LineaTitular

	SnapshotID string
	Reglamento string
}
