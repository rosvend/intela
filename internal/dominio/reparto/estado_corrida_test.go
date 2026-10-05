package reparto_test

import (
	"testing"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

func TestEstadoDeCorrida(t *testing.T) {
	casos := []struct {
		nombre   string
		circuito reparto.Circuito
		etapa    reparto.Etapa
		quiere   string
	}{
		{"recaudo esta en curso", reparto.Nacional, reparto.EtapaRecaudo, reparto.EstadoCorridaEnCurso},
		{"verificacion espera firmas", reparto.Nacional, reparto.EtapaVerificacion, reparto.EstadoCorridaEnFirma},
		{"pago y registro espera firmas", reparto.Internacional, reparto.EtapaPagoRegistro, reparto.EstadoCorridaEnFirma},
		{"auditoria es terminal", reparto.Nacional, reparto.EtapaAuditoria, reparto.EstadoCorridaCerrada},
		{"fees in error del internacional sigue en curso", reparto.Internacional, reparto.EtapaFeesInError, reparto.EstadoCorridaEnCurso},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := reparto.EstadoDeCorrida(c.circuito, c.etapa); got != c.quiere {
				t.Fatalf("EstadoDeCorrida(%s, %s) = %q, se esperaba %q", c.circuito, c.etapa, got, c.quiere)
			}
		})
	}
}
