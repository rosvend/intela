package aplicacion

import (
	"context"
	"fmt"

	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

// Titulares es el caso de uso del padron: quien figura ante la sociedad y, de
// esos, quien puede recibir reparto (`R-01`, `RD 4.5`).
//
// Existe aunque hoy sea una operacion de una linea, y no es ceremonia: es lo
// que pide la cabecera de httpapi -cada lectura tiene que tener su caso de
// uso, y no un handler consultando la base-, porque es aqui donde viviran el
// asiento y la autorizacion el dia que el padron se edite. Ademas es donde
// vive el defecto de la paginacion, que es lo que hace que el tope no dependa
// de cada adaptador.
//
// No lleva [BitacoraAuditoria]: leer el padron no es un hecho que haya que
// poder explicar dentro de diez anos. El ADR 0006 pide asiento para lo que
// mueve dinero o cambia una cifra, no para cada consulta.
type Titulares struct {
	Padron PadronTitulares
}

// BuscarTitulares resuelve una consulta del padron.
//
// Un filtro vacio devuelve la primera pagina del padron: "sin recorte" de
// nombre/IPI/persona natural sigue siendo un recorte mas, y la paginacion es
// el tope que evita servir el padron entero de REDES SGC de un golpe. El
// defecto vive aqui -no en cada adaptador- para que cualquier
// [PadronTitulares] lo herede y se pueda comprobar sin levantar Postgres; es
// la misma forma que [Catalogo.BuscarObras].
//
// Devuelve el padron entero, personas juridicas incluidas, por lo que explica
// [PadronTitulares]: la regla de quien cobra se aplica al elegir las partes de
// una declaracion, no al leer el padron.
func (t Titulares) BuscarTitulares(ctx context.Context, f FiltroTitulares) ([]afiliacion.Titular, error) {
	f.Paginacion = f.ConDefecto()
	titulares, err := t.Padron.BuscarTitulares(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("buscar titulares: %w", err)
	}
	return titulares, nil
}
