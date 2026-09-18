package reparto

import (
	"errors"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/recaudo"
)

// Modalidad del acto de comunicacion publica (RD 8). Determina con que
// formula se valoriza un uso, no cuanto vale.
type Modalidad string

const (
	TV          Modalidad = "tv"
	Cine        Modalidad = "cine"
	OTT         Modalidad = "ott"
	Hotel       Modalidad = "hotel" // RD 9.6: remite a 9.5 (misma estrategia que Suscripcion)
	Teatro      Modalidad = "teatro"
	Transporte  Modalidad = "transporte"
	Suscripcion Modalidad = "suscripcion"
)

// Modalidades devuelve los valores validos, en el orden del CHECK de usos.
func Modalidades() []Modalidad {
	return []Modalidad{TV, Cine, OTT, Hotel, Teatro, Transporte, Suscripcion}
}

// ParseModalidad valida una modalidad. Una desconocida es error tipado, no
// cero puntos silenciosos (#119 / #33).
func ParseModalidad(s string) (Modalidad, error) {
	m := Modalidad(s)
	if !slices.Contains(Modalidades(), m) {
		return "", fmt.Errorf("%w: %q", ErrModalidadDesconocida, s)
	}
	return m, nil
}

// GrupoCanal es la clasificacion efectiva de un canal para RD 9.5.
// La resuelve la capa de aplicacion contra canales_clasificacion del ano
// anterior; el motor la recibe ya fijada (ADR 0005).
type GrupoCanal string

const (
	GrupoPrivadosNacionales GrupoCanal = "privado_nacional"
	GrupoRegionalesPublicos GrupoCanal = "regional_publico"
	GrupoPremium            GrupoCanal = "premium"
	GrupoLideresRating      GrupoCanal = "lideres_rating"
	GrupoEstandar           GrupoCanal = "estandar"
)

// GruposCanal devuelve los cinco grupos de RD 9.5.1–9.5.5.
func GruposCanal() []GrupoCanal {
	return []GrupoCanal{
		GrupoPrivadosNacionales, GrupoRegionalesPublicos, GrupoPremium,
		GrupoLideresRating, GrupoEstandar,
	}
}

// BaseCineTeatro elige la medida de ponderacion de cine/teatro (P-01 / T-02).
const (
	BaseEspectadores = "espectadores"
	BaseTaquilla     = "taquilla"
)

// Circuito de la corrida. Son dos recorridos distintos, no una variante de
// uno: el internacional no valoriza por puntos (RD 7.4). Ver ADR 0008.
//
// Alias de [recaudo.Circuito], que es donde se declara. Ver [Bolsa].
type Circuito = recaudo.Circuito

const (
	Nacional      = recaudo.Nacional
	Internacional = recaudo.Internacional
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
//
// Ningun porcentaje de grupo ni la asignacion de plataformas de terceros es
// literal en el motor: viven aqui (ADR 0004, RD 9.5, RD 9.7).
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

	GrupoPrivadosPct      decimal.Decimal
	GrupoRegionalesPct    decimal.Decimal
	GrupoPremiumPct       decimal.Decimal
	GrupoLideresPct       decimal.Decimal
	GrupoEstandarPct      decimal.Decimal
	AsignacionTercerosPct decimal.Decimal

	// BaseCineTeatro es "espectadores" o "taquilla" (P-01). Vacio es error.
	BaseCineTeatro string

	Reglamento string

	// Coeficientes de RD 9.1.1(c). Viven en el snapshot y no en el codigo
	// (ADR 0004): el 80% artistico y los 48 minutos de la hora televisiva
	// los aplica normalizacion al canonizar la fila, y el motor consume
	// ya la duracion transformada.
	DuracionArtisticaPct decimal.Decimal
	MinutosHoraTV        decimal.Decimal

	// Moneda base del periodo y tasas a esa base (clave ISO → factor).
	// No hay lista de monedas en codigo: solo se convierten las que traen
	// tasa. EUR sin entrada propia no hereda la del dolar.
	MonedaBase string
	Tasas      map[string]decimal.Decimal
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
	CanalID       string
	Grupo         GrupoCanal // clasificacion efectiva; solo suscripcion/hotel
	DuracionMin   decimal.Decimal
	Emisiones     int64
	Rating        decimal.Decimal
	Taquilla      decimal.Decimal
	Espectadores  decimal.Decimal
	Exhibiciones  int64
	Vistas        decimal.Decimal
	MinutosVistos decimal.Decimal
	PB            decimal.Decimal

	// FueraDeRepertorio marca un canal sin contenido del catalogo (R-27).
	// Cero-valor = dentro del repertorio. Solo la estrategia de suscripcion
	// lo aplica, y lo hace ANTES del split por grupos.
	FueraDeRepertorio bool
}

// Bolsa a repartir en un periodo. Es lo unico que Recaudo pasa aguas abajo:
// Reparto no conoce Usuario, Convenio ni Tarifa (ADR 0003).
//
// Alias de [recaudo.Bolsa]. Se declaraba aqui, y estaba al reves: la regla del
// ADR 0003 dice que la bolsa es lo que Recaudo ENTREGA, asi que el consumidor
// no puede ser el dueno del tipo. Vive ahora en internal/dominio/recaudo, con
// el constructor que la valida ([recaudo.NuevaBolsa]); el alias deja intacto
// todo lo que ya la nombraba por este paquete.
//
// Que este paquete importe `recaudo` es la direccion permitida: depguard
// deniega la vuelta (regla modulos-recaudo). Y que las reglas modulos-* de
// otros modulos denieguen `recaudo` no entra en conflicto con esto: lo que
// esas reglas protegen es que nadie mas vea Usuario ni la categoria del
// pagador, no la bolsa, que es justo lo que el reglamento hace bajar.
type Bolsa = recaudo.Bolsa

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

// Sentinel errors del paquete. Un solo centinela por clase de fallo.
var (
	ErrModalidadDesconocida = errors.New("modalidad desconocida")
	ErrParametroAusente     = errors.New("parametro normativo ausente")
	ErrRepartoInvalido      = errors.New("reparto invalido")
)
