package aplicacion

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// LectorProcesos es la mitad de solo lectura de [RepositorioProcesos]: quien consulta no puede escribir.
type LectorProcesos interface {
	ProcesoPorID(ctx context.Context, id string) (ProcesoVista, error)
	ListarProcesos(ctx context.Context) ([]ProcesoVista, error)
}

// ConsultarEstadoCorrida dice en que etapa de RD 13.5 esta una corrida y que le falta, para el staff (#69).
type ConsultarEstadoCorrida struct {
	Procesos LectorProcesos
}

// EstadoCorrida es la etapa de una corrida explicada en lenguaje llano.
type EstadoCorrida struct {
	ProcesoID         string
	Periodo           string
	BolsaID           string
	Reglamento        string
	Circuito          string
	CircuitoExplicado string
	Etapa             string
	EtapaNombre       string
	EtapaExplicada    string
	Paso              int
	TotalPasos        int
	SiguienteEtapa    string
	Compuerta         bool
	Revision          int
	FirmasFaltantes   []string
	Pendiente         string
	UltimoRechazo     string
}

// PorID lee el estado de una corrida; misma matriz que GET /procesos/{id}.
func (c ConsultarEstadoCorrida) PorID(ctx context.Context, actor Usuario, procesoID string) (EstadoCorrida, error) {
	if !esStaff(actor.Rol) {
		return EstadoCorrida{}, ErrNoAutorizado
	}
	v, err := c.Procesos.ProcesoPorID(ctx, strings.TrimSpace(procesoID))
	if err != nil {
		return EstadoCorrida{}, fmt.Errorf("estado de la corrida %q: %w", procesoID, err)
	}
	return estadoDe(v), nil
}

// DePeriodo lista las corridas de un periodo (AAAA cubre sus meses; vacio, todas), la mas reciente primero.
func (c ConsultarEstadoCorrida) DePeriodo(ctx context.Context, actor Usuario, periodo string) ([]EstadoCorrida, error) {
	if !esStaff(actor.Rol) {
		return nil, ErrNoAutorizado
	}
	periodo = strings.TrimSpace(periodo)
	if periodo != "" && !periodoRe.MatchString(periodo) {
		return nil, ErrPeriodoInvalido
	}
	vs, err := c.Procesos.ListarProcesos(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar corridas: %w", err)
	}
	out := []EstadoCorrida{}
	for _, v := range vs {
		if periodo == "" || v.Periodo == periodo || strings.HasPrefix(v.Periodo, periodo+"-") {
			out = append(out, estadoDe(v))
		}
	}
	slices.SortFunc(out, func(a, b EstadoCorrida) int {
		return cmp.Or(strings.Compare(b.Periodo, a.Periodo), strings.Compare(a.ProcesoID, b.ProcesoID))
	})
	return out, nil
}

func estadoDe(v ProcesoVista) EstadoCorrida {
	p := unProceso(v)
	etapas := reparto.EtapasDe(p.Circuito)
	i := slices.Index(etapas, p.Etapa)
	e := EstadoCorrida{
		ProcesoID:         v.ID,
		Periodo:           v.Periodo,
		BolsaID:           v.BolsaID,
		Reglamento:        v.Reglamento,
		Circuito:          string(p.Circuito),
		CircuitoExplicado: circuitosExplicados[p.Circuito],
		Etapa:             string(p.Etapa),
		EtapaNombre:       etapasExplicadas[p.Etapa].nombre,
		EtapaExplicada:    etapasExplicadas[p.Etapa].texto,
		Paso:              i + 1,
		TotalPasos:        len(etapas),
		Compuerta:         p.EnCompuerta(),
		Revision:          v.Revision,
		FirmasFaltantes:   []string{},
		UltimoRechazo:     v.RechazoMotivo,
	}
	if i >= 0 && i < len(etapas)-1 {
		e.SiguienteEtapa = string(etapas[i+1])
	}
	for _, rol := range p.FirmasFaltantes() {
		e.FirmasFaltantes = append(e.FirmasFaltantes, string(rol))
	}
	e.Pendiente = pendienteDe(p, e)
	return e
}

// pendienteDe dice, sin jerga, que tiene que pasar para que la corrida avance.
func pendienteDe(p reparto.ProcesoDeReparto, e EstadoCorrida) string {
	siguiente := etapasExplicadas[reparto.Etapa(e.SiguienteEtapa)].nombre
	switch {
	case e.Paso == 0:
		return "La etapa registrada no pertenece al recorrido de este circuito; hay que revisarla con el administrador."
	case e.SiguienteEtapa == "":
		return "Nada: es la última etapa. La corrida queda cerrada para la auditoría del Revisor Fiscal."
	case e.Compuerta && len(e.FirmasFaltantes) > 0:
		return fmt.Sprintf("Es una compuerta de doble firma (RD 13.5): falta la firma de %s sobre la revisión %d. Con las dos firmas, el administrador puede avanzarla a %s.",
			strings.Join(e.FirmasFaltantes, " y "), e.Revision, siguiente)
	case e.Compuerta:
		return fmt.Sprintf("Ya tiene las firmas de distribución y contabilidad; falta que el administrador la avance a %s.", siguiente)
	case p.Etapa == reparto.EtapaDeducciones || reparto.Etapa(e.SiguienteEtapa) == reparto.EtapaVerificacion:
		return fmt.Sprintf("Falta que el administrador la avance a %s. Antes se revisa que el periodo no tenga anomalías críticas abiertas; si las tiene, no avanza hasta resolverlas.", siguiente)
	case p.Circuito == reparto.Nacional && p.Etapa == reparto.EtapaLiquidacionFinal:
		return fmt.Sprintf("Falta que el administrador la avance a %s. Para salir, la liquidación del periodo tiene que estar emitida, y eso espera a que todas las corridas del periodo lleguen a esta etapa.", siguiente)
	default:
		return fmt.Sprintf("Falta que el administrador la avance a %s.", siguiente)
	}
}

var circuitosExplicados = map[reparto.Circuito]string{
	reparto.Nacional:      "Nacional: dinero recaudado en Colombia. Se valoriza obra por obra y se reparte entre sus autores (RD 13.5).",
	reparto.Internacional: "Internacional: dinero que envían las sociedades hermanas del exterior. Ya llega por obra, así que no se valoriza por puntos (RD 7.4), y tiene la etapa Fees in Error (RD 13.7).",
}

type etapaExplicada struct{ nombre, texto string }

// etapasExplicadas resume RD 13.5 etapa por etapa para quien no conoce el reglamento.
var etapasExplicadas = map[reparto.Etapa]etapaExplicada{
	reparto.EtapaRecaudo:            {"Recaudo", "Se consolida el informe del dinero recaudado que se va a repartir."},
	reparto.EtapaDeducciones:        {"Deducciones", "Se aplican las deducciones de ley sobre lo recaudado."},
	reparto.EtapaImporteObra:        {"Importe de la obra", "Se calculó cuánto le corresponde a cada obra según la ponderación del reglamento."},
	reparto.EtapaImporteTitular:     {"Importe por titular", "Se reparte el importe de cada obra entre sus autores según la declaración de obra."},
	reparto.EtapaLiquidacionParcial: {"Liquidación parcial", "Se generó el listado parcial de liquidaciones por obra y por titular."},
	reparto.EtapaVerificacion:       {"Verificación", "Distribución y contabilidad revisan la liquidación parcial y la autorizan o piden correcciones."},
	reparto.EtapaLiquidacionFinal:   {"Liquidación final", "Se genera la liquidación final con las observaciones de la verificación."},
	reparto.EtapaPagoRegistro:       {"Pago y registro", "Se autoriza el pago y se hace su registro contable."},
	reparto.EtapaFeesInError:        {"Fees in Error", "Se verifica que los montos del exterior correspondan a socios de REDES SGC; los que no, se listan para devolverlos."},
	reparto.EtapaAuditoria:          {"Auditoría", "Corrida terminada; queda para la auditoría periódica del Revisor Fiscal."},
}
