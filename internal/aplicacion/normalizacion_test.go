package aplicacion

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/normalizacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

func paramsNormativos() normalizacion.Parametros {
	return normalizacion.Parametros{
		DuracionArtisticaPct: decimal.RequireFromString("0.80"),
		MinutosHoraTV:        decimal.NewFromInt(48),
		MonedaBase:           "COP",
		MonedasReconocidas:   []string{"COP", "USD", "EUR"},
		TRM:                  decimal.RequireFromString("4000"),
	}
}

func loteMixto() []normalizacion.Fila {
	return []normalizacion.Fila{
		{
			ID: "tv-ok", Fuente: "caracol", Modalidad: "tv",
			Titulo: "Serie Y", Fecha: "20241231", Duracion: "60", Emisiones: "10",
		},
		{
			ID: "cine-ok", Fuente: "procinal", Modalidad: "cine",
			Titulo: "Pelicula X", Fecha: "2024-06-01", Taquilla: "100", Moneda: "COP",
		},
		{
			ID: "ott-ok", Fuente: "netflix", Modalidad: "ott",
			Titulo: "Show Z", Fecha: "2018-12-31", Vistas: "1200",
		},
		{
			ID: "tv-fecha", Fuente: "caracol", Modalidad: "tv",
			Titulo: "Inparseable", Fecha: "ayer", Duracion: "60",
		},
		{
			ID: "cine-moneda", Fuente: "procinal", Modalidad: "cine",
			Titulo: "Yen", Taquilla: "500", Moneda: "JPY",
		},
	}
}

func TestProcesarSeparaCanonicasYRevisionSinPerderFilas(t *testing.T) {
	n := Normalizacion{}
	filas := loteMixto()

	r := n.Procesar(filas, paramsNormativos())

	if len(r.Normalizados) != 3 {
		t.Fatalf("normalizados = %d, se esperaban 3", len(r.Normalizados))
	}
	if len(r.Revision) != 2 {
		t.Fatalf("revision = %d, se esperaban 2", len(r.Revision))
	}
	if len(r.Normalizados)+len(r.Revision) != len(filas) {
		t.Fatal("una fila se descarto en silencio")
	}

	porID := map[string]UsoPersistido{}
	for _, u := range r.Normalizados {
		porID[u.ID] = u
		if u.RechazoMotivo != "" {
			t.Fatalf("%s canonica con motivo %q", u.ID, u.RechazoMotivo)
		}
	}
	if !porID["tv-ok"].DuracionMin.Equal(decimal.RequireFromString("48")) {
		t.Fatalf("Serie Y: duracion %s, se esperaba 48 (80%% de 60)", porID["tv-ok"].DuracionMin)
	}

	codigos := map[string]string{}
	for _, u := range r.Revision {
		if u.RechazoMotivo == "" {
			t.Fatalf("%s en revision sin motivo", u.ID)
		}
		codigo, _ := cortarMotivo(u.RechazoMotivo)
		codigos[u.ID] = codigo
	}
	if codigos["tv-fecha"] != normalizacion.CodigoFechaInparseable {
		t.Fatalf("tv-fecha: %q", codigos["tv-fecha"])
	}
	if codigos["cine-moneda"] != normalizacion.CodigoMonedaDesconocida {
		t.Fatalf("cine-moneda: %q", codigos["cine-moneda"])
	}
}

func TestProcesarEsIdempotente(t *testing.T) {
	n := Normalizacion{}
	filas := loteMixto()
	p := paramsNormativos()

	a := n.Procesar(filas, p)
	b := n.Procesar(filas, p)

	if len(a.Normalizados) != len(b.Normalizados) || len(a.Revision) != len(b.Revision) {
		t.Fatalf("re-ejecutar cambio el recuento: %d/%d vs %d/%d",
			len(a.Normalizados), len(a.Revision), len(b.Normalizados), len(b.Revision))
	}
	if !a.Normalizados[0].DuracionMin.Equal(b.Normalizados[0].DuracionMin) {
		t.Fatal("re-ejecutar cambio una cifra")
	}
}

func TestProcesarYGuardarEsIdempotenteContraElRepositorio(t *testing.T) {
	repo := nuevoRepoIngesta()
	n := Normalizacion{Reportes: repo}
	rep := repDePrueba()
	filas := loteMixto()
	p := paramsNormativos()
	ctx := t.Context()

	primero, err := n.ProcesarYGuardar(ctx, rep, filas, p)
	if err != nil {
		t.Fatalf("primera pasada: %v", err)
	}
	if len(primero.Normalizados) != 3 || len(primero.Revision) != 2 {
		t.Fatalf("primera pasada: %d normalizados, %d revision",
			len(primero.Normalizados), len(primero.Revision))
	}

	escrituras := len(repo.usos)
	segundo, err := n.ProcesarYGuardar(ctx, rep, filas, p)
	if err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if len(segundo.Normalizados) != 3 || len(segundo.Revision) != 2 {
		t.Fatalf("segunda pasada cambio el recuento logico")
	}
	if len(repo.usos) != escrituras {
		t.Fatalf("la segunda pasada escribio de nuevo: %d filas, habia %d", len(repo.usos), escrituras)
	}
}

func TestListarRevisionLeeElLogDeRechazos(t *testing.T) {
	repo := nuevoRepoIngesta()
	n := Normalizacion{Reportes: repo}
	ctx := t.Context()

	if _, err := n.ProcesarYGuardar(ctx, repDePrueba(), loteMixto(), paramsNormativos()); err != nil {
		t.Fatalf("guardar: %v", err)
	}

	items, err := n.ListarRevision(ctx)
	if err != nil {
		t.Fatalf("ListarRevision: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("cola = %d, se esperaban 2", len(items))
	}
	for _, it := range items {
		if it.Tipo != TipoRevisionNormalizacion {
			t.Fatalf("tipo %q, se esperaba %s", it.Tipo, TipoRevisionNormalizacion)
		}
		if it.Codigo == "" || it.Motivo == "" || it.Titulo == "" {
			t.Fatalf("item incompleto: %+v", it)
		}
	}
}

func TestParametrosDesdeExigeLosCoeficientesDeTV(t *testing.T) {
	_, err := ParametrosDesde(reparto.Snapshot{})
	if !errors.Is(err, ErrParametroAusente) {
		t.Fatalf("snapshot vacio: %v", err)
	}

	p, err := ParametrosDesde(reparto.Snapshot{
		DuracionArtisticaPct: decimal.RequireFromString("0.80"),
		MinutosHoraTV:        decimal.NewFromInt(48),
		MonedaBase:           "COP",
	})
	if err != nil {
		t.Fatalf("snapshot completo: %v", err)
	}
	if !p.DuracionArtisticaPct.Equal(decimal.RequireFromString("0.80")) {
		t.Fatalf("pct = %s", p.DuracionArtisticaPct)
	}
}
