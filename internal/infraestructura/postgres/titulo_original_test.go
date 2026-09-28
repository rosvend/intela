package postgres

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// Review de PR #146, B1: el titulo original tiene que llegar al escalon 3. Estas
// pruebas cubren el camino por la base: la columna `usos.titulo_original`, su
// lectura por la proyeccion canonica y la bandeja que dice contra que titulo se
// puntuo cada candidato.

// El titulo original sobrevive el viaje de ida y vuelta por GuardarUsos y las
// dos lecturas que usa la cascada, y una fila sin original lo devuelve vacio.
func TestGuardarUsosConservaElTituloOriginalDeIdaYVuelta(t *testing.T) {
	s, _ := sembrarReportes(t)
	ctx := t.Context()

	conOriginal := usoPendiente("uso-con-original", reporteEnero, "Sin Tetas No Hay Paraiso")
	conOriginal.TituloOrig = "Without Breasts There Is No Paradise"
	sinOriginal := usoPendiente("uso-sin-original", reporteEnero, "Rebelde")

	if err := s.GuardarUsos(ctx, []aplicacion.UsoPersistido{conOriginal, sinOriginal}); err != nil {
		t.Fatalf("GuardarUsos: %v", err)
	}

	porID, err := s.UsoPorID(ctx, "uso-con-original")
	if err != nil {
		t.Fatalf("UsoPorID: %v", err)
	}
	if porID.Titulo != conOriginal.Titulo || porID.TituloOrig != conOriginal.TituloOrig {
		t.Fatalf("UsoPorID perdio un titulo: titulo=%q original=%q", porID.Titulo, porID.TituloOrig)
	}

	delPeriodo, err := s.UsosDePeriodo(ctx, "2026-01")
	if err != nil {
		t.Fatalf("UsosDePeriodo: %v", err)
	}
	originales := map[string]string{}
	for _, u := range delPeriodo {
		originales[u.ID] = u.TituloOrig
	}
	if originales["uso-con-original"] != "Without Breasts There Is No Paradise" {
		t.Errorf("UsosDePeriodo perdio el original: %q", originales["uso-con-original"])
	}
	if got, hay := originales["uso-sin-original"]; !hay || got != "" {
		t.Errorf("sin original tenia que volver vacio, volvio %q (esta=%v)", got, hay)
	}
}

// La bandeja guarda cuanto y contra que titulo se puntuo, y se lee en el orden
// en que se escribio (RD 16).
func TestGuardarCandidatosConservaElTituloConsultadoYElOrden(t *testing.T) {
	s, _ := sembrarIdentificacion(t)
	ctx := t.Context()

	quiero := []identificacion.Candidato{
		{ObraID: obraIda, Puntaje: decimal.RequireFromString("0.52"), TituloConsultado: "Without Breasts"},
		{ObraID: obraImdb, Puntaje: decimal.RequireFromString("0.47"), TituloConsultado: "Sin Tetas"},
	}
	if err := s.GuardarCandidatos(ctx, "u-3", quiero); err != nil {
		t.Fatalf("GuardarCandidatos: %v", err)
	}

	tengo, err := s.CandidatosDeUso(ctx, "u-3")
	if err != nil {
		t.Fatalf("CandidatosDeUso: %v", err)
	}
	if len(tengo) != len(quiero) {
		t.Fatalf("bandeja = %+v, se esperaba %+v", tengo, quiero)
	}
	for i := range quiero {
		if tengo[i].ObraID != quiero[i].ObraID || !tengo[i].Puntaje.Equal(quiero[i].Puntaje) ||
			tengo[i].TituloConsultado != quiero[i].TituloConsultado {
			t.Fatalf("candidato %d = %+v, se esperaba %+v", i, tengo[i], quiero[i])
		}
	}
}

// D9: la bandeja se REEMPLAZA. Una lista vacia la deja vacia: una fila que en
// una corrida anterior quedo en banda y ahora cae por debajo del piso no puede
// conservar candidatos que ya no son suyos.
func TestGuardarCandidatosReemplazaLaBandejaYUnaListaVaciaLaVacia(t *testing.T) {
	s, _ := sembrarIdentificacion(t)
	ctx := t.Context()

	primera := []identificacion.Candidato{
		{ObraID: obraIda, Puntaje: decimal.RequireFromString("0.52"), TituloConsultado: "A"},
		{ObraID: obraImdb, Puntaje: decimal.RequireFromString("0.47"), TituloConsultado: "A"},
	}
	if err := s.GuardarCandidatos(ctx, "u-3", primera); err != nil {
		t.Fatalf("primera bandeja: %v", err)
	}

	segunda := []identificacion.Candidato{
		{ObraID: obraVacia, Puntaje: decimal.RequireFromString("0.49"), TituloConsultado: "B"},
	}
	if err := s.GuardarCandidatos(ctx, "u-3", segunda); err != nil {
		t.Fatalf("segunda bandeja: %v", err)
	}
	tengo, err := s.CandidatosDeUso(ctx, "u-3")
	if err != nil {
		t.Fatalf("CandidatosDeUso: %v", err)
	}
	if len(tengo) != 1 || tengo[0].ObraID != obraVacia || tengo[0].TituloConsultado != "B" {
		t.Fatalf("la segunda bandeja no reemplazo a la primera: %+v", tengo)
	}

	if err := s.GuardarCandidatos(ctx, "u-3", nil); err != nil {
		t.Fatalf("bandeja vacia: %v", err)
	}
	tengo, err = s.CandidatosDeUso(ctx, "u-3")
	if err != nil {
		t.Fatalf("CandidatosDeUso: %v", err)
	}
	if len(tengo) != 0 {
		t.Fatalf("una lista vacia tenia que vaciar la bandeja, quedo %+v", tengo)
	}
}

// La cascada COMPLETA, contra Postgres de verdad: el titulo emitido no se parece
// a nada del catalogo y el original si. Sin consultar los dos, esta fila iba a
// ONI. Es el caso de Caracol (16 de 59 filas), en pequeno.
func TestResolverUsosIntegracionIdentificaPorElTituloOriginal(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()
	r := cascadaDePrueba(t, s, pool)

	obraConTitulo(t, pool, "obra-paraiso", "Without Breasts There Is No Paradise")

	conOriginal := aplicacion.UsoPersistido{
		ID: "u-con-original", ReporteID: reporteUno, Fuente: "caracol",
		Titulo: "Sin Tetas No Hay Paraiso", TituloOrig: "Without Breasts There Is No Paradise",
		IDsFuente: "id_ficha=555",
	}
	soloEmitido := aplicacion.UsoPersistido{
		ID: "u-solo-emitido", ReporteID: reporteUno, Fuente: "caracol",
		Titulo: "Sin Tetas No Hay Paraiso", IDsFuente: "id_ficha=556",
	}
	insertarUsoSQL(t, pool, conOriginal)
	insertarUsoSQL(t, pool, soloEmitido)

	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}

	f := leerUso(t, pool, "u-con-original")
	if f.escalon != identificacion.EscalonDifuso || f.obraID != "obra-paraiso" || f.oni {
		t.Fatalf("la fila con original tenia que identificarse por difuso: %+v", f)
	}
	// La evidencia dice de donde salio el parecido: del original, no del emitido.
	if !strings.Contains(f.evidencia, "Without Breasts There Is No Paradise") || strings.Contains(f.evidencia, "Sin Tetas") {
		t.Fatalf("la evidencia no nombra el titulo original: %q", f.evidencia)
	}
	// El difuso aprende (D4): la siguiente emision del programa entra por alias.
	if contarAlias(t, pool, "caracol", "id_ficha", "555") != 1 {
		t.Fatal("el match difuso tenia que aprender el alias")
	}

	// Control: la misma fila sin original NO se identifica. Es lo que demuestra
	// que el original es lo que decidio, y no el emitido.
	g := leerUso(t, pool, "u-solo-emitido")
	if g.obraID != "" || !g.oni {
		t.Fatalf("sin el original la fila tenia que quedar ONI: %+v", g)
	}
}
