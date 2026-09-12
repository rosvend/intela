package aplicacion

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

const claveSMMLV = "smmlv"

// OrdenVista es la orden mas lo que solo se sabe en esta capa: si se
// puede pagar (R-12) y el dia civil con el que se evaluo el plazo.
type OrdenVista struct {
	Orden   liquidacion.OrdenDePago
	Pagable bool
}

// Liquidaciones genera ordenes de pago a partir de las lineas de titular
// de una corrida, y las sirve al admin y al titular.
//
// El reloj entra aqui, no en dominio: EvaluarPlazo recibe un YYYY-MM-DD.
type Liquidaciones struct {
	Ordenes RepositorioLiquidacion
	Reloj   Reloj
}

// GenerarLiquidacion convierte las lineas persistidas de un proceso en
// ordenes de pago, una por titular. Idempotente: si el proceso ya tiene
// ordenes, las devuelve (con el plazo reevaluado) en vez de emitir otras.
func (l Liquidaciones) GenerarLiquidacion(ctx context.Context, procesoID string) ([]OrdenVista, error) {
	existentes, err := l.Ordenes.DeProceso(ctx, procesoID)
	if err != nil {
		return nil, fmt.Errorf("ordenes del proceso %s: %w", procesoID, err)
	}
	if len(existentes) > 0 {
		return l.conPlazoYDocumentos(ctx, existentes)
	}

	insumo, err := l.Ordenes.InsumoDeProceso(ctx, procesoID)
	if err != nil {
		return nil, fmt.Errorf("insumo del proceso %s: %w", procesoID, err)
	}

	ahora := l.Reloj.Ahora().UTC()
	smmlv, err := l.Ordenes.SMMLVVigente(ctx, ahora)
	if err != nil {
		return nil, fmt.Errorf("smmlv: %w", err)
	}
	if smmlv.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("%w: %s", ErrParametroAusente, claveSMMLV)
	}

	porTitular := agruparPorTitular(insumo.Titulares)
	distribuido := decimal.Zero
	for _, neto := range porTitular {
		distribuido = distribuido.Add(neto)
	}

	ids := make([]string, 0, len(porTitular))
	for id := range porTitular {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	enviadaDia := diaCivil(ahora)
	ordenes := make([]liquidacion.OrdenDePago, 0, len(ids))
	acumuladas := make([]liquidacion.OrdenDePago, 0)
	for _, titularID := range ids {
		neto := porTitular[titularID]
		deducciones := prorratearDeducciones(neto, distribuido, insumo)
		bruto := neto
		for _, d := range deducciones {
			bruto = bruto.Add(d.Monto)
		}
		o, err := liquidacion.NuevaOrden(
			idOrden(procesoID, titularID),
			procesoID,
			titularID,
			insumo.Periodo,
			enviadaDia,
			bruto,
			deducciones,
		)
		if err != nil {
			return nil, fmt.Errorf("armar orden de %s: %w", titularID, err)
		}

		previas, err := l.Ordenes.DeTitular(ctx, titularID)
		if err != nil {
			return nil, fmt.Errorf("ordenes previas de %s: %w", titularID, err)
		}
		for _, prev := range previas {
			if prev.Estado != liquidacion.EstadoDiferida {
				continue
			}
			o = o.IncorporarArrastre(prev)
			acumuladas = append(acumuladas, prev.MarcarAcumulada())
		}
		ordenes = append(ordenes, o)
	}

	aGuardar := append([]liquidacion.OrdenDePago{}, ordenes...)
	aGuardar = append(aGuardar, acumuladas...)
	if err := l.Ordenes.Guardar(ctx, aGuardar); err != nil {
		return nil, fmt.Errorf("guardar liquidacion del proceso %s: %w", procesoID, err)
	}
	return l.conDocumentos(ctx, ordenes)
}

// Listar es el listado de administracion. Un titular no lo ve: el suyo
// sale por DeTitular.
func (l Liquidaciones) Listar(ctx context.Context, actor Usuario) ([]OrdenVista, error) {
	if !esStaff(actor.Rol) {
		return nil, ErrNoAutorizado
	}
	ordenes, err := l.Ordenes.Listar(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar liquidaciones: %w", err)
	}
	return l.conPlazoYDocumentos(ctx, ordenes)
}

// DeTitular es GET /mis-liquidaciones. El titular solo ve las suyas; el
// id sale de la sesion, no de la URL, para que no se consulten las de otro.
func (l Liquidaciones) DeTitular(ctx context.Context, actor Usuario) ([]OrdenVista, error) {
	if actor.Rol != RolTitular || actor.TitularID == "" {
		return nil, ErrNoAutorizado
	}
	ordenes, err := l.Ordenes.DeTitular(ctx, actor.TitularID)
	if err != nil {
		return nil, fmt.Errorf("liquidaciones de %s: %w", actor.TitularID, err)
	}
	return l.conPlazoYDocumentos(ctx, ordenes)
}

func (l Liquidaciones) conPlazoYDocumentos(ctx context.Context, ordenes []liquidacion.OrdenDePago) ([]OrdenVista, error) {
	if len(ordenes) == 0 {
		return []OrdenVista{}, nil
	}
	ahora := l.Reloj.Ahora().UTC()
	smmlv, err := l.Ordenes.SMMLVVigente(ctx, ahora)
	if err != nil {
		return nil, fmt.Errorf("smmlv: %w", err)
	}
	if smmlv.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("%w: %s", ErrParametroAusente, claveSMMLV)
	}
	umbral := liquidacion.UmbralMenorCuantia(smmlv)
	hoy := diaCivil(ahora)

	cambiadas := make([]liquidacion.OrdenDePago, 0)
	for i, o := range ordenes {
		nueva := o.EvaluarPlazo(hoy, umbral)
		if nueva.Estado != o.Estado {
			cambiadas = append(cambiadas, nueva)
			ordenes[i] = nueva
		}
	}
	if len(cambiadas) > 0 {
		if err := l.Ordenes.Guardar(ctx, cambiadas); err != nil {
			return nil, fmt.Errorf("persistir transicion de silencio: %w", err)
		}
	}
	return l.conDocumentos(ctx, ordenes)
}

func (l Liquidaciones) conDocumentos(ctx context.Context, ordenes []liquidacion.OrdenDePago) ([]OrdenVista, error) {
	docs, err := l.Ordenes.Documentos(ctx)
	if err != nil {
		return nil, fmt.Errorf("documentos de titulares: %w", err)
	}
	if docs == nil {
		docs = map[string]liquidacion.Documentos{}
	}
	vistas := make([]OrdenVista, 0, len(ordenes))
	for _, o := range ordenes {
		vistas = append(vistas, OrdenVista{
			Orden:   o,
			Pagable: o.EsPagable(docs[o.TitularID]),
		})
	}
	return vistas, nil
}

func esStaff(r Rol) bool {
	switch r {
	case RolAdministrador, RolDistribucion, RolContabilidad, RolAuditor:
		return true
	default:
		return false
	}
}

func idOrden(procesoID, titularID string) string {
	return "liq-" + procesoID + "-" + titularID
}

func agruparPorTitular(lineas []reparto.LineaTitular) map[string]decimal.Decimal {
	porTitular := make(map[string]decimal.Decimal, len(lineas))
	for _, linea := range lineas {
		porTitular[linea.TitularID] = porTitular[linea.TitularID].Add(linea.Importe)
	}
	return porTitular
}

// prorratearDeducciones reparte admin, social y reserva de la bolsa en
// proporcion al neto de cada titular. El neto no se toca: viene de la
// corrida. Bruto = neto + suma(deducciones).
func prorratearDeducciones(netoTitular, distribuido decimal.Decimal, insumo InsumoLiquidacion) []liquidacion.Deduccion {
	if distribuido.IsZero() || netoTitular.IsZero() {
		return []liquidacion.Deduccion{}
	}
	prop := netoTitular.Div(distribuido)
	return []liquidacion.Deduccion{
		{Concepto: liquidacion.ConceptoAdministracion, Monto: insumo.Admin.Mul(prop).Round(2)},
		{Concepto: liquidacion.ConceptoSocial, Monto: insumo.Social.Mul(prop).Round(2)},
		{Concepto: liquidacion.ConceptoReserva, Monto: insumo.Reserva.Mul(prop).Round(2)},
	}
}

// diaCivil deja el helper de formato en un solo sitio: las pruebas del
// plazo mueven el Reloj y comparan contra este mismo recorte.
func diaCivil(instante time.Time) string {
	return instante.UTC().Format("2006-01-02")
}
