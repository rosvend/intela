package postgres

import (
	"testing"
	"time"
)

func TestOrigenDeUsosDaElArchivoExactoYLaIdentificacion(t *testing.T) {
	s, pool := sembrarProcesoNacionalListoParaValorizar(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO usos (id, reporte_id, fuente, titulo, escalon, oni, modalidad)
		 VALUES ('uso-m', $1, 'caracol', 'Obra Y bis', 'oni', TRUE, 'tv')`, reporteEnero); err != nil {
		t.Fatalf("sembrar uso manual: %v", err)
	}
	resuelto := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	if _, err := pool.Exec(ctx,
		`UPDATE usos SET escalon = 'manual', obra_id = 'obra-y', oni = FALSE, puntaje = 0.72,
		        resuelto_por = 'actor-dist', resuelto_en = $1,
		        nota_resolucion = 'coincide la ficha' WHERE id = 'uso-m'`, resuelto); err != nil {
		t.Fatalf("resolver uso manual: %v", err)
	}

	origen, err := s.OrigenDeUsos(ctx, []string{"uso-y", "uso-m", "uso-inexistente"})
	if err != nil {
		t.Fatalf("OrigenDeUsos: %v", err)
	}
	if len(origen) != 2 {
		t.Fatalf("se esperaban 2 origenes, llegaron %d: %+v", len(origen), origen)
	}

	y := origen["uso-y"]
	if y.ReporteID != reporteEnero || y.Fuente != "caracol" || y.SHA256 != shaParrilla || y.ClaveObjeto != "reportes/"+shaParrilla {
		t.Fatalf("uso-y sin su archivo exacto: %+v", y)
	}
	if y.Escalon != "alias" || (y.Puntaje != nil && !y.Puntaje.IsZero()) || y.Evidencia == "" || y.ResueltoPor != "" || y.ResueltoEn != nil {
		t.Fatalf("uso-y identificacion = %+v", y)
	}

	m := origen["uso-m"]
	if m.Escalon != "manual" || m.Puntaje == nil || m.Puntaje.String() != "0.72" || m.ResueltoPor != "actor-dist" || m.ResueltoEn == nil || !m.ResueltoEn.Equal(resuelto) {
		t.Fatalf("uso-m identificacion = %+v", m)
	}
}

func TestOrigenDeUsosSinIDsNoConsulta(t *testing.T) {
	s, _ := sembrarReportes(t)
	origen, err := s.OrigenDeUsos(t.Context(), nil)
	if err != nil || len(origen) != 0 {
		t.Fatalf("OrigenDeUsos(nil) = %v, %v", origen, err)
	}
}
