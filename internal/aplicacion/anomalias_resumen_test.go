package aplicacion

import (
	"errors"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/dominio/anomalias"
	"github.com/rosvend/intela/internal/dominio/recaudo"
)

func resumenDe(alertas []Alerta, asientos []Asiento) Anomalias {
	return Anomalias{
		Alertas:  &alertasFalsas{filas: alertas},
		Bitacora: &bitacoraFalsa{asientos: asientos},
	}
}

func alertaAbierta(tipo, periodo string) Alerta {
	return Alerta{Tipo: tipo, Periodo: periodo, RefTipo: "uso", RefID: tipo + periodo}
}

func TestResumenTraeLosSeisTiposConCerosExplicitos(t *testing.T) {
	r, err := resumenDe(nil, nil).Resumen(t.Context(), periodoDePrueba)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.PorTipo) != 6 {
		t.Fatalf("PorTipo trae %d claves, se esperaban 6", len(r.PorTipo))
	}
	for _, tipo := range anomalias.Tipos() {
		c, ok := r.PorTipo[tipo]
		if !ok || c.Abiertas != 0 || c.Critica != anomalias.EsCritica(tipo) {
			t.Errorf("%s = %+v (presente %v)", tipo, c, ok)
		}
	}
}

func TestResumenCuentaAbiertasYSoloCriticasLasQueBloquean(t *testing.T) {
	var filas []Alerta
	for i := range 3 {
		a := alertaAbierta(anomalias.TipoONI, periodoDePrueba)
		a.RefID += string(rune('a' + i))
		filas = append(filas, a)
	}
	for i := range 2 {
		a := alertaAbierta(anomalias.TipoDuplicadoRegistro, periodoDePrueba)
		a.RefID += string(rune('a' + i))
		filas = append(filas, a)
	}
	resuelta := alertaAbierta(anomalias.TipoDuplicadoRegistro, periodoDePrueba)
	resuelta.Resuelta = true
	otroPeriodo := alertaAbierta(anomalias.TipoDuplicadoRegistro, "2025-02")
	filas = append(filas, resuelta, otroPeriodo)

	r, err := resumenDe(filas, nil).Resumen(t.Context(), periodoDePrueba)
	if err != nil {
		t.Fatal(err)
	}
	if r.Abiertas != 5 || r.CriticasAbiertas != 2 {
		t.Fatalf("Abiertas = %d, CriticasAbiertas = %d; se esperaba 5 y 2", r.Abiertas, r.CriticasAbiertas)
	}
	if r.PorTipo[anomalias.TipoONI].Abiertas != 3 || r.PorTipo[anomalias.TipoDuplicadoRegistro].Abiertas != 2 {
		t.Fatalf("PorTipo = %+v", r.PorTipo)
	}
}

func TestResumenCuentaLasAceptadasTalCual(t *testing.T) {
	a := alertaAbierta(anomalias.TipoDuplicadoRegistro, periodoDePrueba)
	a.Resuelta, a.Accion = true, anomalias.AccionAceptarTalCual
	r, err := resumenDe([]Alerta{a}, nil).Resumen(t.Context(), periodoDePrueba)
	if err != nil {
		t.Fatal(err)
	}
	if r.CriticasAceptadas != 1 || r.Abiertas != 0 {
		t.Fatalf("CriticasAceptadas = %d, Abiertas = %d; se esperaba 1 y 0", r.CriticasAceptadas, r.Abiertas)
	}
}

func TestResumenSinEvaluacionDevuelveNil(t *testing.T) {
	otro := asientoDePrueba("otro.hecho", RefPeriodo, periodoDePrueba)
	r, err := resumenDe(nil, []Asiento{otro}).Resumen(t.Context(), periodoDePrueba)
	if err != nil {
		t.Fatal(err)
	}
	if r.UltimaEvaluacion != nil {
		t.Fatalf("UltimaEvaluacion = %v, se esperaba nil", r.UltimaEvaluacion)
	}
}

// La bitacora real entrega los asientos en orden ascendente (bitacora.go,
// ORDER BY cuando): "la primera" seria la mas antigua. Se prueba en los dos
// ordenes para que ni "gana la primera" ni "gana la ultima" pasen.
func TestResumenTomaLaEvaluacionMasReciente(t *testing.T) {
	viejo := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	nuevo := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	otro := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	casos := map[string][3]time.Time{
		"mas nueva primero":  {nuevo, viejo, otro},
		"mas nueva al final": {viejo, nuevo, otro},
	}
	for nombre, cuandos := range casos {
		t.Run(nombre, func(t *testing.T) {
			asientos := []Asiento{
				asientoDePrueba(HechoAnomaliasEvaluadas, RefPeriodo, periodoDePrueba),
				asientoDePrueba(HechoAnomaliasEvaluadas, RefPeriodo, periodoDePrueba),
				asientoDePrueba("otro.hecho", RefPeriodo, periodoDePrueba),
			}
			asientos[0].Cuando, asientos[1].Cuando, asientos[2].Cuando = cuandos[0], cuandos[1], cuandos[2]

			r, err := resumenDe(nil, asientos).Resumen(t.Context(), periodoDePrueba)
			if err != nil {
				t.Fatal(err)
			}
			if r.UltimaEvaluacion == nil || !r.UltimaEvaluacion.Equal(nuevo) {
				t.Fatalf("UltimaEvaluacion = %v, se esperaba %v", r.UltimaEvaluacion, nuevo)
			}
		})
	}
}

func TestResumenRechazaPeriodoInvalido(t *testing.T) {
	_, err := resumenDe(nil, nil).Resumen(t.Context(), "2026-13")
	if !errors.Is(err, recaudo.ErrBolsaInvalida) {
		t.Fatalf("err = %v, se esperaba ErrBolsaInvalida", err)
	}
}
