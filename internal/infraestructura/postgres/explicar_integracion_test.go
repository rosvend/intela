package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

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

	// #187: el desglose sale de la bitacora y reproduce los puntos persistidos.
	if len(x.Valorizacion) != 1 || x.Valorizacion[0].UsoID != "uso-y" || x.Valorizacion[0].Formula != "RD 9.1.1" {
		t.Fatalf("valorizacion = %+v, se esperaba un uso uso-y por RD 9.1.1", x.Valorizacion)
	}
	val := x.Valorizacion[0]
	if len(val.Terminos) != 1 {
		t.Fatalf("terminos = %+v", val.Terminos)
	}
	nombres := []string{"ponderacion", "duracion_min", "rating", "emisiones"}
	valores := []string{"1.3", "48", "9", "10"}
	if len(val.Terminos[0].Factores) != len(nombres) {
		t.Fatalf("factores = %+v", val.Terminos[0].Factores)
	}
	producto := decimal.NewFromInt(1)
	for i, f := range val.Terminos[0].Factores {
		got, err := decimal.NewFromString(f.Valor)
		if err != nil {
			t.Fatalf("factor %q: %v", f.Nombre, err)
		}
		if f.Nombre != nombres[i] || !got.Equal(decimal.RequireFromString(valores[i])) {
			t.Errorf("factor %d = %s %s, se esperaba %s %s", i, f.Nombre, f.Valor, nombres[i], valores[i])
		}
		producto = producto.Mul(got)
	}
	puntosUso, err := decimal.NewFromString(val.Puntos)
	if err != nil {
		t.Fatalf("puntos del uso: %v", err)
	}
	productoAsentado, err := decimal.NewFromString(val.Terminos[0].Producto)
	if err != nil {
		t.Fatalf("producto: %v", err)
	}
	puntosObra, err := decimal.NewFromString(x.Obra.Puntos)
	if err != nil {
		t.Fatalf("obra.puntos: %v", err)
	}
	if !producto.Equal(productoAsentado) || !producto.Equal(puntosUso) {
		t.Errorf("producto de factores %s != producto asentado %s / puntos %s", producto, productoAsentado, puntosUso)
	}
	if len(resultado.Obras) != 1 || !puntosUso.Round(8).Equal(resultado.Obras[0].Puntos) || !puntosUso.Round(8).Equal(puntosObra) {
		t.Errorf("los factores dan %s, el resultado persistido %+v, obra.puntos %s", puntosUso.Round(8), resultado.Obras, puntosObra)
	}

	if _, err := (aplicacion.ExplicarCifra{Bitacora: s}).Explicar(ctx, auditor, "proc-y:obra-y:titular-otro"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("titular sin linea: %v", err)
	}
}
