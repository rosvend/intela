package aplicacion

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

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

// ResumenLiquidacion es el ultimo periodo del titular con corridas firmadas: neto sumado y obras distintas.
type ResumenLiquidacion struct {
	Periodo string
	Neto    decimal.Decimal
	Obras   int
}

// LineaDeTitular es una linea neta del titular con la etapa de su corrida, para filtrar las firmadas.
type LineaDeTitular struct {
	Periodo  string
	Circuito reparto.Circuito
	Etapa    reparto.Etapa
	ObraID   string
	Neto     decimal.Decimal
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
	LineasDeTitular(ctx context.Context, titularID string) ([]LineaDeTitular, error)
	CargasPendientes(ctx context.Context) (int, error)
	CasosONIPendientes(ctx context.Context) (int, error)
	UltimaCorrida(ctx context.Context) (ProcesoVista, error)
	Declaraciones(ctx context.Context) (map[string]repertorio.Declaracion, error)
	ListarObras(ctx context.Context, p Paginacion) ([]Obra, error)
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

// UltimaLiquidacion resume el ultimo periodo del titular con corridas que ya cerraron la verificacion (ADR 0024).
func (t Tablero) UltimaLiquidacion(ctx context.Context, actor Usuario) (ResumenLiquidacion, error) {
	titularID, err := titularDe(actor)
	if err != nil {
		return ResumenLiquidacion{}, err
	}
	lineas, err := t.Repo.LineasDeTitular(ctx, titularID)
	if err != nil {
		return ResumenLiquidacion{}, err
	}
	firmadas := make([]LineaDeTitular, 0, len(lineas))
	for _, l := range lineas {
		if reparto.AlcanzoEtapa(l.Circuito, l.Etapa, reparto.EtapaLiquidacionFinal) {
			firmadas = append(firmadas, l)
		}
	}
	if len(firmadas) == 0 {
		return ResumenLiquidacion{}, fmt.Errorf("%w: %s no tiene lineas firmadas", ErrNoEncontrado, titularID)
	}
	r := ResumenLiquidacion{Neto: decimal.Zero}
	for _, l := range firmadas {
		r.Periodo = max(r.Periodo, l.Periodo)
	}
	obras := map[string]struct{}{}
	for _, l := range firmadas {
		if l.Periodo == r.Periodo {
			r.Neto = r.Neto.Add(l.Neto)
			obras[l.ObraID] = struct{}{}
		}
	}
	r.Obras = len(obras)
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
	obras, err := t.Repo.ListarObras(ctx, Paginacion{Limite: LimiteSinTope})
	if err != nil {
		return 0, err
	}
	decls, err := t.Repo.Declaraciones(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, o := range obras {
		if !decls[o.ID].Completa() {
			n++
		}
	}
	return n, nil
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
