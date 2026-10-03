package postgres

import (
	"fmt"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/anomalias"
)

// sembrarAlertas escribe n alertas abiertas de un tipo, sin pasar por la evaluacion.
func sembrarAlertas(t *testing.T, s *Store, tipo, prefijo string, n int, detectada time.Time) {
	t.Helper()

	alertas := make([]aplicacion.Alerta, 0, n)
	for i := range n {
		alertas = append(alertas, aplicacion.Alerta{
			Periodo:   periodoAlertas,
			Tipo:      tipo,
			RefTipo:   anomalias.RefUso,
			RefID:     fmt.Sprintf("%s-%04d", prefijo, i),
			Detalle:   "sembrada para el resumen",
			Critica:   anomalias.EsCritica(tipo),
			Detectada: detectada,
		})
	}
	if nuevas, _, err := s.GuardarAlertas(t.Context(), alertas); err != nil || nuevas != n {
		t.Fatalf("sembrar %d alertas de %s: nuevas=%d err=%v", n, tipo, nuevas, err)
	}
}

// El resumen cuenta en la base: mas alertas que una pagina no lo recortan.
func TestResumenCuentaMasQueUnaPagina(t *testing.T) {
	s, _ := sembrar(t)
	svc := servicioDeAnomalias(s, instanteAlertas)
	sembrarAlertas(t, s, anomalias.TipoDuplicadoRegistro, "dup", aplicacion.LimiteObrasPorDefecto+50, instanteAlertas)
	sembrarAlertas(t, s, anomalias.TipoONI, "oni", aplicacion.LimiteObrasPorDefecto, instanteAlertas)

	r, err := svc.Resumen(t.Context(), periodoAlertas)
	if err != nil {
		t.Fatalf("Resumen: %v", err)
	}
	if r.Abiertas != 250 || r.CriticasAbiertas != 150 {
		t.Fatalf("Abiertas = %d, CriticasAbiertas = %d; se esperaba 250 y 150", r.Abiertas, r.CriticasAbiertas)
	}
	if r.PorTipo[anomalias.TipoONI].Abiertas != 100 || r.PorTipo[anomalias.TipoDuplicadoRegistro].Abiertas != 150 {
		t.Fatalf("PorTipo = %+v", r.PorTipo)
	}
}

// Cien cerradas recientes no pueden tapar las abiertas antiguas: el modo de
// fallo de contar sobre una pagina mezclada.
func TestResumenNoSeDejaTaparPorLasCerradasRecientes(t *testing.T) {
	s, _ := sembrar(t)
	svc := servicioDeAnomalias(s, instanteAlertas)
	sembrarAlertas(t, s, anomalias.TipoONI, "vieja", 5, instanteAlertas.Add(-48*time.Hour))
	sembrarAlertas(t, s, anomalias.TipoONI, "nueva", 100, instanteAlertas)

	recientes, err := s.ListarAlertas(t.Context(), aplicacion.FiltroAlertas{
		Periodo:    periodoAlertas,
		Paginacion: aplicacion.Paginacion{Limite: aplicacion.LimiteSinTope},
	})
	if err != nil {
		t.Fatalf("ListarAlertas: %v", err)
	}
	cerradas := 0
	for _, a := range recientes {
		if !a.Detectada.Equal(instanteAlertas) {
			continue
		}
		_, err := s.ResolverAlerta(t.Context(), a.ID, aplicacion.CierreDeAlerta{
			ActorID: usuarioAdmin, ActorRol: string(aplicacion.RolDistribucion),
			Nota: "cerrada", Cuando: instanteAlertas,
		})
		if err != nil {
			t.Fatalf("ResolverAlerta: %v", err)
		}
		cerradas++
	}
	if cerradas != 100 {
		t.Fatalf("se cerraron %d, se esperaban 100", cerradas)
	}

	r, err := svc.Resumen(t.Context(), periodoAlertas)
	if err != nil {
		t.Fatalf("Resumen: %v", err)
	}
	if r.Abiertas != 5 {
		t.Fatalf("Abiertas = %d, se esperaban las 5 antiguas", r.Abiertas)
	}
}

func TestResumenSinEvaluarYDespuesDeEvaluar(t *testing.T) {
	s, _ := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	antes, err := svc.Resumen(t.Context(), periodoAlertas)
	if err != nil {
		t.Fatalf("Resumen antes: %v", err)
	}
	if antes.UltimaEvaluacion != nil {
		t.Fatalf("UltimaEvaluacion = %v antes de evaluar, se esperaba nil", antes.UltimaEvaluacion)
	}

	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	despues, err := svc.Resumen(t.Context(), periodoAlertas)
	if err != nil {
		t.Fatalf("Resumen despues: %v", err)
	}
	if despues.UltimaEvaluacion == nil || !despues.UltimaEvaluacion.Equal(instanteAlertas) {
		t.Fatalf("UltimaEvaluacion = %v, se esperaba %v", despues.UltimaEvaluacion, instanteAlertas)
	}
	if despues.Abiertas == 0 {
		t.Fatal("el periodo sembrado con anomalias salio sin abiertas")
	}
}
