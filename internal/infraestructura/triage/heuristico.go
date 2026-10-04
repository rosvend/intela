// Package triage es el rankeador de la cola manual (#53).
//
// Propone un orden y una accion. No resuelve casos: la decision sigue siendo
// de una persona (ADR 0007). Hoy la propuesta es la heuristica del nucleo;
// un modelo entra sustituyendo este adaptador, detras del mismo puerto.
package triage

import (
	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// Heuristico aplica [identificacion.Rankear]. Es el adaptador del puerto
// [aplicacion.PuertoRankeadorDeResoluciones].
type Heuristico struct{}

var _ aplicacion.PuertoRankeadorDeResoluciones = Heuristico{}

// Rankear devuelve la sugerencia. No escribe.
func (Heuristico) Rankear(p identificacion.PedidoTriage) identificacion.Sugerencia {
	return identificacion.Rankear(p)
}
