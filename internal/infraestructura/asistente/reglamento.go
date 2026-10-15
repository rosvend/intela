package asistente

import (
	"context"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/aplicacion/herramientas"
	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/embeddings"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
)

// buscarReglamento solo se ofrece al modelo si hay EMBEDDINGS_PROVEEDOR: una herramienta que no puede responder no se anuncia.
func buscarReglamento(store *postgres.Store) (aplicacion.Herramienta, bool, error) {
	e, err := embeddings.Elegir(context.Background(), config.Cadena("EMBEDDINGS_PROVEEDOR", ""), config.Cadena("EMBEDDINGS_MODELO", ""))
	if err != nil || e.Motor == nil {
		return aplicacion.Herramienta{}, false, err
	}
	return herramientas.BuscarReglamento(aplicacion.ConsultarReglamento{Motor: e.Motor, Almacen: store, Piso: e.Piso}), true, nil
}
