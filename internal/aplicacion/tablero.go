package aplicacion

import (
	"context"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// ObraDeclarada es una obra con su declaracion vigente completa, no solo la parte del titular.
type ObraDeclarada struct {
	ID          string
	Titulo      string
	Declaracion repertorio.Declaracion
}

// ObraResumen es una fila de "mis obras": el estado es el mismo vocabulario del catalogo.
type ObraResumen struct {
	ID     string
	Titulo string
	Estado string
}

// ResumenLiquidacion es el ultimo periodo del titular con ordenes de pago emitidas: neto sumado y obras distintas.
type ResumenLiquidacion struct {
	Periodo string
	Neto    decimal.Decimal
	Obras   int
}

// CorridaResumen es la ultima corrida en tres palabras.
type CorridaResumen struct {
	Periodo string
	Etapa   string
	Estado  string
}

// RepositorioTablero es la lectura de las tarjetas del tablero; sin filas es ErrNoEncontrado donde aplica.
type RepositorioTablero interface {
	ObrasDeclaradasDe(ctx context.Context, titularID string) ([]ObraDeclarada, error)
	// DeTitular son las ordenes de pago del titular: la misma lectura que /mis-liquidaciones.
	DeTitular(ctx context.Context, titularID string) ([]liquidacion.OrdenDePago, error)
	ObrasDeTitularEnProcesos(ctx context.Context, titularID string, procesos []string) (int, error)
	CargasPendientes(ctx context.Context) (int, error)
	CasosONIPendientes(ctx context.Context) (int, error)
	UltimaCorrida(ctx context.Context) (ProcesoVista, error)
	// ContarObrasEnReserva cuenta las obras del catalogo cuya declaracion vigente no es Completa().
	ContarObrasEnReserva(ctx context.Context) (int, error)
}

// Tablero sirve los conteos del panel de inicio; el rol de staff lo cierra el adaptador HTTP.
type Tablero struct {
	Repo RepositorioTablero
}

func titularDe(actor Usuario) (string, error) {
	if actor.Rol != RolTitular || actor.TitularID == "" {
		return "", ErrNoAutorizado
	}
	return actor.TitularID, nil
}

// MisObras lista las obras donde el titular de la sesion tiene parte declarada.
func (t Tablero) MisObras(ctx context.Context, actor Usuario) ([]ObraResumen, error) {
	titularID, err := titularDe(actor)
	if err != nil {
		return nil, err
	}
	obras, err := t.Repo.ObrasDeclaradasDe(ctx, titularID)
	if err != nil {
		return nil, err
	}
	out := make([]ObraResumen, 0, len(obras))
	for _, o := range obras {
		out = append(out, ObraResumen{ID: o.ID, Titulo: o.Titulo, Estado: o.Declaracion.Estado()})
	}
	return out, nil
}

// UltimaLiquidacion resume el ultimo periodo del titular con ordenes de pago (ADR 0024).
//
// Un periodo solo tiene ordenes cuando TODAS sus corridas llegaron a liquidacion_final, y la
// orden ya trae las diferidas y los arrastres de R-11: por eso se lee ordenes_pago y no la etapa
// de cada linea. Es la misma fuente que /mis-liquidaciones.
func (t Tablero) UltimaLiquidacion(ctx context.Context, actor Usuario) (ResumenLiquidacion, error) {
	titularID, err := titularDe(actor)
	if err != nil {
		return ResumenLiquidacion{}, err
	}
	ordenes, err := t.Repo.DeTitular(ctx, titularID)
	if err != nil {
		return ResumenLiquidacion{}, err
	}
	if len(ordenes) == 0 {
		return ResumenLiquidacion{}, fmt.Errorf("%w: %s no tiene ordenes de pago", ErrNoEncontrado, titularID)
	}
	r := ResumenLiquidacion{Neto: decimal.Zero}
	for _, o := range ordenes {
		r.Periodo = max(r.Periodo, o.Periodo)
	}
	var procesos []string
	for _, o := range ordenes {
		if o.Periodo == r.Periodo {
			r.Neto = r.Neto.Add(o.Neto)
			procesos = append(procesos, o.Procesos...)
		}
	}
	slices.Sort(procesos)
	procesos = slices.Compact(procesos)
	if r.Obras, err = t.Repo.ObrasDeTitularEnProcesos(ctx, titularID, procesos); err != nil {
		return ResumenLiquidacion{}, err
	}
	return r, nil
}

// CargasPendientes cuenta los reportes con filas que la cascada aun no proceso.
func (t Tablero) CargasPendientes(ctx context.Context) (int, error) {
	return t.Repo.CargasPendientes(ctx)
}

// ONIPendientes cuenta los casos de identificacion pendientes (escalon oni).
func (t Tablero) ONIPendientes(ctx context.Context) (int, error) {
	return t.Repo.CasosONIPendientes(ctx)
}

// ObrasEnReserva cuenta las obras del catalogo sin declaracion completa, tambien las no declaradas (R-04, RD 13.1.3).
func (t Tablero) ObrasEnReserva(ctx context.Context) (int, error) {
	return t.Repo.ContarObrasEnReserva(ctx)
}

// UltimaCorrida devuelve la corrida mas reciente con su estado derivado de la etapa.
func (t Tablero) UltimaCorrida(ctx context.Context) (CorridaResumen, error) {
	p, err := t.Repo.UltimaCorrida(ctx)
	if err != nil {
		return CorridaResumen{}, err
	}
	return CorridaResumen{
		Periodo: p.Periodo,
		Etapa:   string(p.Etapa),
		Estado:  reparto.EstadoDeCorrida(p.Circuito, p.Etapa),
	}, nil
}
