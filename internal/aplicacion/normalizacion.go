package aplicacion

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/normalizacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Origenes de la cola de revision. El listado es uno; el tipo dice de cual
// detector salio la fila. "anomalia" lo rellena el #37.
const (
	TipoRevisionNormalizacion = "normalizacion"
	TipoRevisionAnomalia      = "anomalia"
)

// Normalizacion orquesta el paso de esquema: filas crudas -> forma canonica
// o cola de revision.
//
// El dominio decide QUE fila no es canonica y por que. Este caso de uso
// decide que hacer con cada una: persistirla como uso o moverla al log de
// rechazos, que es la cola de revision de OE-1. No hay un tercer destino.
//
// "Usos sin resolver" aqui son las filas crudas que el adaptador de formato
// acaba de mapear, no las pendientes de identificacion. UsosSinResolver es
// el puerto de la cascada (ADR 0007); mezclarlo aqui reaplicaria el 80%
// sobre una duracion ya transformada.
type Normalizacion struct {
	Reportes RepositorioIngesta
}

// ResultadoLote es el recuento que pide la aceptacion: N canonicas, M en
// revision. Ninguna fila se pierde en el camino.
type ResultadoLote struct {
	Normalizados []UsoPersistido
	Revision     []UsoPersistido
}

// ParametrosDesde extrae del snapshot congelado lo que [normalizacion]
// necesita. Los coeficientes de duracion ausentes no se inventan (ADR 0004):
// el error es ErrParametroAusente. Una fila de cine no los usa, y el dominio
// ya manda a revision la de TV que los necesite y no los tenga; esta funcion
// es la guarda de quien arma el snapshot, no un segundo criterio.
func ParametrosDesde(s reparto.Snapshot) (normalizacion.Parametros, error) {
	p := normalizacion.Parametros{
		DuracionArtisticaPct: s.DuracionArtisticaPct,
		MinutosHoraTV:        s.MinutosHoraTV,
		MonedaBase:           s.MonedaBase,
		TRM:                  s.TRM,
	}
	if s.MonedaBase != "" {
		p.MonedasReconocidas = []string{s.MonedaBase, "USD", "EUR"}
	}
	faltanTV := !p.DuracionArtisticaPct.GreaterThan(decimal.Zero) ||
		!p.MinutosHoraTV.GreaterThan(decimal.Zero)
	if faltanTV {
		return p, fmt.Errorf("%w: duracion.artistica_pct o duracion.minutos_hora_tv", ErrParametroAusente)
	}
	return p, nil
}

// Procesar normaliza un lote en memoria. No toca el repositorio: es lo que
// hace idempotente re-ejecutarlo, y lo que permite afirmar N + M = len(filas)
// sin una transaccion.
func (n Normalizacion) Procesar(filas []normalizacion.Fila, p normalizacion.Parametros) ResultadoLote {
	out := ResultadoLote{
		Normalizados: make([]UsoPersistido, 0, len(filas)),
		Revision:     make([]UsoPersistido, 0),
	}
	for _, f := range filas {
		uso, rev := normalizacion.Normalizar(f, p)
		persistido := aPersistido(uso, rev)
		if rev != nil {
			out.Revision = append(out.Revision, persistido)
			continue
		}
		out.Normalizados = append(out.Normalizados, persistido)
	}
	return out
}

// ProcesarYGuardar normaliza y persiste. Reejecutarlo con las mismas filas
// (mismos ids) no duplica: las que ya estan en usos o en el log de rechazos
// se saltan. El recuento que devuelve es el del lote logico, no el de la
// escritura de esta pasada.
func (n Normalizacion) ProcesarYGuardar(
	ctx context.Context,
	rep Reporte,
	filas []normalizacion.Fila,
	p normalizacion.Parametros,
) (ResultadoLote, error) {
	resultado := n.Procesar(filas, p)
	if n.Reportes == nil {
		return resultado, nil
	}

	conocidos, err := n.idsEnRevision(ctx)
	if err != nil {
		return ResultadoLote{}, err
	}

	lote := make([]UsoPersistido, 0, len(filas))
	for _, u := range append(append([]UsoPersistido{}, resultado.Normalizados...), resultado.Revision...) {
		if u.ID != "" && conocidos[u.ID] {
			continue
		}
		if u.ID != "" {
			if _, err := n.Reportes.UsoPorID(ctx, u.ID); err == nil {
				continue
			}
		}
		lote = append(lote, u)
	}
	if len(lote) == 0 {
		return resultado, nil
	}

	ingesta := Ingesta{Reportes: n.Reportes}
	if _, err := ingesta.GuardarUsos(ctx, rep, lote); err != nil {
		return ResultadoLote{}, fmt.Errorf("persistir el lote normalizado del reporte %q: %w", rep.ID, err)
	}
	return resultado, nil
}

// ListarRevision es la cola compartida con las anomalias del #37. Hoy solo
// hay filas de normalizacion: salen del log de rechazos, cada una con su
// motivo. Un listado vacio es una lista vacia, no nil.
func (n Normalizacion) ListarRevision(ctx context.Context) ([]ItemRevision, error) {
	if n.Reportes == nil {
		return []ItemRevision{}, nil
	}
	rechazos, err := n.Reportes.ListarRechazos(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar la cola de revision: %w", err)
	}
	items := make([]ItemRevision, 0, len(rechazos))
	for _, u := range rechazos {
		codigo, _ := cortarMotivo(u.RechazoMotivo)
		items = append(items, ItemRevision{
			ID:        u.ID,
			Tipo:      TipoRevisionNormalizacion,
			Codigo:    codigo,
			Motivo:    u.RechazoMotivo,
			Fuente:    u.Fuente,
			Titulo:    u.Titulo,
			ReporteID: u.ReporteID,
		})
	}
	return items, nil
}

func (n Normalizacion) idsEnRevision(ctx context.Context) (map[string]bool, error) {
	conocidos := map[string]bool{}
	rechazos, err := n.Reportes.ListarRechazos(ctx)
	if err != nil {
		return nil, fmt.Errorf("leer rechazos para idempotencia: %w", err)
	}
	for _, u := range rechazos {
		if u.ID != "" {
			conocidos[u.ID] = true
		}
	}
	return conocidos, nil
}

func aPersistido(u normalizacion.Uso, rev *normalizacion.Revision) UsoPersistido {
	p := UsoPersistido{
		ID:            u.ID,
		Fuente:        u.Fuente,
		Titulo:        u.Titulo,
		IDsFuente:     u.IDsFuente,
		Modalidad:     reparto.Modalidad(u.Modalidad),
		TipoObra:      u.TipoObra,
		DuracionMin:   u.DuracionMin,
		Emisiones:     u.Emisiones,
		Rating:        u.Rating,
		Taquilla:      u.Taquilla,
		Vistas:        u.Vistas,
		MinutosVistos: u.MinutosVistos,
		PB:            u.PB,
	}
	if rev != nil {
		p.RechazoMotivo = rev.Motivo()
	}
	return p
}

func cortarMotivo(motivo string) (codigo, detalle string) {
	codigo, detalle, ok := strings.Cut(motivo, ": ")
	if !ok {
		return "", motivo
	}
	return codigo, detalle
}
