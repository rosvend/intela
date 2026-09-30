package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// Corre una corrida Nacional de verdad y explica la linea del titular solo desde la bitacora.
func TestExplicarCifraDeUnaCorridaReal(t *testing.T) {
	s, _ := sembrarProcesoNacionalListoParaValorizar(t)
	ctx := t.Context()

	if err := s.Asentar(ctx, aplicacion.Asiento{
		Hecho: aplicacion.HechoRecaudoRegistrado, RefTipo: aplicacion.RefBolsa, RefID: "bolsa-1",
		Payload: []byte(`{"usuario_id":"caracol","periodo":"2026-01","circuito":"nacional","bruto":"1000000.00","convenio":"conv-1","tarifa":"T-01","factura":"F-1"}`),
		Cuando:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("asentar recaudo: %v", err)
	}
	if err := s.Asentar(ctx, aplicacion.Asiento{
		Hecho: aplicacion.HechoObraRegistrada, RefTipo: aplicacion.RefObra, RefID: "obra-y",
		Payload: []byte(`{"despues":{"titulo":"Obra Y","genero":"Drama","anio":2020}}`),
		Cuando:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("asentar alta de obra: %v", err)
	}

	uc := aplicacion.Procesos{
		Repo: s, Parametros: s, Bolsas: s, Declaraciones: s, Usos: s,
		Resultados: s, Unidad: s, Bitacora: s, Reloj: reloj.Sistema{}, Origen: s,
		Anomalias: servicioDeAnomalias(s, time.Now()),
	}
	if _, err := uc.IniciarProceso(ctx, "proc-y", "2026-01", reparto.Nacional, "bolsa-1", "actor-dist"); err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	for range 5 {
		if _, err := uc.AvanzarEtapa(ctx, "proc-y", "actor-dist"); err != nil {
			t.Fatalf("avanzar: %v", err)
		}
	}
	if _, err := uc.Firmar(ctx, "proc-y", reparto.RolDistribucion, "actor-dist"); err != nil {
		t.Fatalf("firmar distribucion: %v", err)
	}
	if _, err := uc.Firmar(ctx, "proc-y", reparto.RolContabilidad, "actor-conta"); err != nil {
		t.Fatalf("firmar contabilidad: %v", err)
	}

	auditor := aplicacion.Usuario{ID: "usr-aud", Rol: aplicacion.RolAuditor}
	x, err := aplicacion.ExplicarCifra{Bitacora: s}.Explicar(ctx, auditor, "proc-y:obra-y:titular-y")
	if err != nil {
		t.Fatalf("explicar: %v", err)
	}

	resultado, err := s.ResultadoPorProceso(ctx, "proc-y")
	if err != nil {
		t.Fatalf("resultado: %v", err)
	}
	if !x.Neto.Equal(resultado.Titulares[0].Importe) {
		t.Fatalf("neto explicado %s != importe persistido %s", x.Neto, resultado.Titulares[0].Importe)
	}
	if x.Bolsa.ID != "bolsa-1" || x.Bolsa.Recaudo == nil || x.Bolsa.Recaudo.Factura != "F-1" {
		t.Fatalf("1. bolsa = %+v", x.Bolsa)
	}
	if len(x.Reportes) != 1 || x.Reportes[0].SHA256 != shaParrilla || x.Reportes[0].ClaveObjeto != "reportes/"+shaParrilla {
		t.Fatalf("2. reportes = %+v", x.Reportes)
	}
	if x.Obra.Escalon != "alias" || len(x.Identificacion) != 1 || x.Identificacion[0].UsoID != "uso-y" {
		t.Fatalf("3. obra = %+v / %+v", x.Obra, x.Identificacion)
	}
	if x.Regla.SnapshotID == "" || x.Regla.SnapshotID != resultado.SnapshotID {
		t.Fatalf("4. regla = %+v, snapshot del resultado %q", x.Regla, resultado.SnapshotID)
	}
	if len(resultado.Titulares) != 1 || resultado.Titulares[0].DeclaracionVersion == nil || *resultado.Titulares[0].DeclaracionVersion != 1 {
		t.Fatalf("la linea persistida no guardo la version usada: %+v", resultado.Titulares)
	}
	if x.Split == nil || x.Split.Version == nil || *x.Split.Version != *resultado.Titulares[0].DeclaracionVersion || x.Split.IPI != "IPI-Y" {
		t.Fatalf("5. split = %+v, se esperaba la version persistida en la linea", x.Split)
	}
	suma := x.Neto
	for _, d := range x.Deducciones {
		if d.Porcentaje.IsZero() {
			t.Fatalf("6. deduccion sin porcentaje: %+v", d)
		}
		suma = suma.Add(d.Monto)
	}
	if len(x.Deducciones) != 3 || !suma.Equal(x.Bruto) {
		t.Fatalf("6. deducciones = %+v, bruto %s", x.Deducciones, x.Bruto)
	}
	if len(x.Firmas) != 2 || x.Firmas[0].ActorID != "actor-dist" || x.Firmas[1].ActorID != "actor-conta" {
		t.Fatalf("7. firmas = %+v", x.Firmas)
	}
	if x.Obra.Titulo != "Obra Y" || len(x.Faltantes) != 0 {
		t.Fatalf("obra.titulo = %q, faltantes = %v: la cadena esta completa", x.Obra.Titulo, x.Faltantes)
	}

	if _, err := (aplicacion.ExplicarCifra{Bitacora: s}).Explicar(ctx, auditor, "proc-y:obra-y:titular-otro"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("titular sin linea: %v", err)
	}
}
