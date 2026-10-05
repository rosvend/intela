// Package asistente compone el catalogo de herramientas del agente; cmd/api y cmd/lambda lo comparten.
package asistente

import (
	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/aplicacion/herramientas"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
)

// Herramientas es el UNICO sitio donde se registra una herramienta: una linea en su hueco, sin tocar los main.
func Herramientas(store *postgres.Store) (aplicacion.CatalogoHerramientas, error) {
	var hs []aplicacion.Herramienta

	// #67: buscar_reglamento.
	if h, ok, err := buscarReglamento(store); err != nil {
		return aplicacion.CatalogoHerramientas{}, err
	} else if ok {
		hs = append(hs, h)
	}

	// #69: estado_corrida y listar_oni.
	hs = append(hs, herramientas.EstadoCorrida(aplicacion.ConsultarEstadoCorrida{Procesos: store}))
	hs = append(hs, herramientas.ListarONI(aplicacion.ConsultarONI{Casos: store}))

	// #49: explicar_cifra.

	// #68: buscar_obra y estado_declaracion.

	return aplicacion.NuevoCatalogoHerramientas(hs...)
}
