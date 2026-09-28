package aplicacion

import (
	"context"

	"github.com/shopspring/decimal"
)

// FiltroIngresos recorta la lista del titular. Los tres campos son
// opcionales: vacio significa "todas".
//
// El titularID NO esta aqui. Sale de la sesion. Un query string
// `titular_id=` no puede cambiar a quien se consulta.
type FiltroIngresos struct {
	ObraID  string
	Fuente  string
	Periodo string
}

// Ingreso es una fila del panel del titular.
//
// Neto es el unico monto. El bruto vive solo dentro de Explicacion (OE-6:
// "el panel refleja los montos netos y no los brutos").
type Ingreso struct {
	Ref     string
	ObraID  string
	Obra    string
	Fuente  string
	Periodo string
	Neto    decimal.Decimal
}

// ConsultaIngresos es el caso de uso del panel: lo que le toca a ESTE titular.
type ConsultaIngresos struct {
	Repo RepositorioIngresos
}

// MisIngresos lista las cifras netas del actor. El alcance lo fija
// actor.TitularID; el filtro solo recorta.
//
// Un rol que no es titular no tiene "mis" ingresos: el administrador no
// cobra reparto (R-01) y el auditor mira por /explicar, no por este listado.
func (c ConsultaIngresos) MisIngresos(ctx context.Context, actor Usuario, f FiltroIngresos) ([]Ingreso, error) {
	if actor.Rol != RolTitular || actor.TitularID == "" {
		return nil, ErrNoAutorizado
	}
	filas, err := c.Repo.IngresosDe(ctx, actor.TitularID, f)
	if err != nil {
		return nil, err
	}
	if filas == nil {
		filas = []Ingreso{}
	}
	return filas, nil
}
