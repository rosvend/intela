package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/dominio/oni"
)

type repoPublicacionMem struct {
	pendientes    []oni.DatosIdentificatorios
	errPendientes error
	guardadas     []PublicacionONI
	errGuardar    error
	anclados      map[string]time.Time
	errAnclar     error
	siguienteID   string
}

func (r *repoPublicacionMem) PendientesDePeriodo(_ context.Context, _ string) ([]oni.DatosIdentificatorios, error) {
	return r.pendientes, r.errPendientes
}

func (r *repoPublicacionMem) GuardarPublicacion(_ context.Context, p PublicacionONI) (PublicacionONI, error) {
	if r.errGuardar != nil {
		return PublicacionONI{}, r.errGuardar
	}
	for _, g := range r.guardadas {
		if g.Periodo == p.Periodo {
			return PublicacionONI{}, ErrYaPublicado
		}
	}
	if p.ID == "" {
		p.ID = r.siguienteID
		if p.ID == "" {
			p.ID = "pub-1"
		}
	}
	if p.Obras == nil {
		p.Obras = []oni.ProyeccionPublica{}
	}
	r.guardadas = append(r.guardadas, p)
	return p, nil
}

func (r *repoPublicacionMem) AnclarPrescripcion(_ context.Context, usoIDs []string, cuando time.Time) error {
	if r.errAnclar != nil {
		return r.errAnclar
	}
	if r.anclados == nil {
		r.anclados = map[string]time.Time{}
	}
	for _, id := range usoIDs {
		if _, hay := r.anclados[id]; !hay {
			r.anclados[id] = cuando
		}
	}
	return nil
}

func (r *repoPublicacionMem) PublicacionVigente(_ context.Context) (PublicacionONI, error) {
	if len(r.guardadas) == 0 {
		return PublicacionONI{}, ErrNoEncontrado
	}
	return r.guardadas[len(r.guardadas)-1], nil
}

func (r *repoPublicacionMem) PublicacionDePeriodo(_ context.Context, periodo string) (PublicacionONI, error) {
	for _, g := range r.guardadas {
		if g.Periodo == periodo {
			return g, nil
		}
	}
	return PublicacionONI{}, ErrNoEncontrado
}

type bitacoraMem struct {
	asientos []Asiento
	err      error
}

func (b *bitacoraMem) Asentar(_ context.Context, a Asiento) error {
	if b.err != nil {
		return b.err
	}
	b.asientos = append(b.asientos, a)
	return nil
}

func (b *bitacoraMem) De(_ context.Context, _, _ string) ([]Asiento, error) {
	return b.asientos, nil
}

func (b *bitacoraMem) PorID(_ context.Context, _ string) (Asiento, error) {
	return Asiento{}, ErrNoEncontrado
}

type txPassthrough struct{}

func (txPassthrough) Ejecutar(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func publicarPrueba(repo *repoPublicacionMem, bit *bitacoraMem) PublicarListadoONI {
	return PublicarListadoONI{
		ONI:         repo,
		Bitacora:    bit,
		Reloj:       relojFijo{instante: momento},
		Tx:          txPassthrough{},
		Fisica:      "Calle 74 #7-35, Bogota",
		Electronica: "oni@redescritores.com",
	}
}

func TestPublicarListadoONICongelaTitulosSinMontos(t *testing.T) {
	repo := &repoPublicacionMem{
		pendientes: []oni.DatosIdentificatorios{
			{ID: "uso-1", Titulo: "Serie X", Fuente: "caracol", IDsFuente: "ID-1", Modalidad: "tv", Periodo: "2026-01"},
			{ID: "uso-2", Titulo: "Unitario Y", Fuente: "netflix", IDsFuente: "show-9", Modalidad: "ott", Periodo: "2026-01"},
		},
	}
	bit := &bitacoraMem{}

	pub, err := publicarPrueba(repo, bit).Ejecutar(context.Background(), "2026-01", "usr-admin")
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if pub.Periodo != "2026-01" {
		t.Fatalf("Periodo = %q", pub.Periodo)
	}
	if !pub.FechaProceso.Equal(momento) {
		t.Fatalf("FechaProceso = %v, se esperaba el del reloj", pub.FechaProceso)
	}
	if pub.DireccionFisica == "" || pub.DireccionElectronica == "" {
		t.Fatal("R-18 exige las dos direcciones")
	}
	if len(pub.Obras) != 2 {
		t.Fatalf("Obras = %d, se esperaban 2", len(pub.Obras))
	}
	if pub.Obras[0].Titulo != "Serie X" {
		t.Fatalf("Titulo = %q", pub.Obras[0].Titulo)
	}
}

func TestPublicarListadoONIEmiteAsientoYAnclaPrescripcion(t *testing.T) {
	repo := &repoPublicacionMem{
		pendientes: []oni.DatosIdentificatorios{
			{ID: "uso-1", Titulo: "Serie X", Fuente: "caracol", Modalidad: "tv", Periodo: "2026-01"},
		},
		siguienteID: "pub-42",
	}
	bit := &bitacoraMem{}

	if _, err := publicarPrueba(repo, bit).Ejecutar(context.Background(), "2026-01", "usr-admin"); err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}

	if len(bit.asientos) != 1 {
		t.Fatalf("asientos = %d, se esperaba 1", len(bit.asientos))
	}
	a := bit.asientos[0]
	if a.Hecho != HechoListadoONIPublicado {
		t.Fatalf("Hecho = %q", a.Hecho)
	}
	if a.RefTipo != RefTipoPublicacionONI || a.RefID != "pub-42" {
		t.Fatalf("ref = %s/%s", a.RefTipo, a.RefID)
	}
	if a.ActorID != "usr-admin" {
		t.Fatalf("ActorID = %q", a.ActorID)
	}
	if !a.Cuando.Equal(momento) {
		t.Fatalf("Cuando = %v", a.Cuando)
	}

	var payload map[string]any
	if err := json.Unmarshal(a.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	for _, prohibido := range []string{"monto", "importe", "bruto", "neto", "taquilla"} {
		if _, hay := payload[prohibido]; hay {
			t.Fatalf("el asiento publico no puede llevar %q: %v", prohibido, payload)
		}
		if strings.Contains(strings.ToLower(string(a.Payload)), prohibido) {
			t.Fatalf("el payload menciona %q: %s", prohibido, a.Payload)
		}
	}

	if _, hay := repo.anclados["uso-1"]; !hay {
		t.Fatal("no se anclo la prescripcion del uso publicado")
	}
	if !repo.anclados["uso-1"].Equal(momento) {
		t.Fatalf("ancla = %v, se esperaba %v", repo.anclados["uso-1"], momento)
	}
}

func TestPublicarListadoONINoRepublicaElMismoPeriodo(t *testing.T) {
	repo := &repoPublicacionMem{
		pendientes: []oni.DatosIdentificatorios{
			{ID: "uso-1", Titulo: "Serie X", Fuente: "caracol", Modalidad: "tv", Periodo: "2026-01"},
		},
	}
	bit := &bitacoraMem{}
	uc := publicarPrueba(repo, bit)

	if _, err := uc.Ejecutar(context.Background(), "2026-01", "usr-admin"); err != nil {
		t.Fatalf("primera publicacion: %v", err)
	}
	_, err := uc.Ejecutar(context.Background(), "2026-01", "usr-admin")
	if !errors.Is(err, ErrYaPublicado) {
		t.Fatalf("se esperaba ErrYaPublicado, se obtuvo %v", err)
	}
	if len(bit.asientos) != 1 {
		t.Fatalf("republicar no debe dejar un segundo asiento: %d", len(bit.asientos))
	}
}

func TestPublicarListadoONIRechazaPeriodoInvalido(t *testing.T) {
	uc := publicarPrueba(&repoPublicacionMem{}, &bitacoraMem{})
	for _, periodo := range []string{"", "enero", "2026/01", "26-01"} {
		t.Run(periodo, func(t *testing.T) {
			_, err := uc.Ejecutar(context.Background(), periodo, "usr-admin")
			if !errors.Is(err, ErrPeriodoInvalido) {
				t.Fatalf("periodo %q: se esperaba ErrPeriodoInvalido, se obtuvo %v", periodo, err)
			}
		})
	}
}

func TestPublicarListadoONIExigeDirecciones(t *testing.T) {
	uc := publicarPrueba(&repoPublicacionMem{}, &bitacoraMem{})
	uc.Fisica = ""
	_, err := uc.Ejecutar(context.Background(), "2026-01", "usr-admin")
	if !errors.Is(err, ErrDireccionPublicacionAusente) {
		t.Fatalf("se esperaba ErrDireccionPublicacionAusente, se obtuvo %v", err)
	}
}

func TestPublicarListadoONINoDescartaElFalloDelAsiento(t *testing.T) {
	repo := &repoPublicacionMem{
		pendientes: []oni.DatosIdentificatorios{
			{ID: "uso-1", Titulo: "Serie X", Fuente: "caracol", Modalidad: "tv", Periodo: "2026-01"},
		},
	}
	bit := &bitacoraMem{err: errors.New("bitacora caida")}

	_, err := publicarPrueba(repo, bit).Ejecutar(context.Background(), "2026-01", "usr-admin")
	if err == nil {
		t.Fatal("un asiento que falla no deja el caso de uso hecho")
	}
	if !errors.Is(err, bit.err) {
		t.Fatalf("se perdio la causa: %v", err)
	}
}

func TestConsultarListadoONI(t *testing.T) {
	repo := &repoPublicacionMem{
		guardadas: []PublicacionONI{
			{ID: "pub-1", Periodo: "2025-06", FechaProceso: momento.Add(-time.Hour)},
			{ID: "pub-2", Periodo: "2026-01", FechaProceso: momento},
		},
	}
	c := ConsultarListadoONI{ONI: repo}

	t.Run("sin periodo devuelve la vigente", func(t *testing.T) {
		p, err := c.Ejecutar(context.Background(), "")
		if err != nil {
			t.Fatalf("Ejecutar: %v", err)
		}
		if p.ID != "pub-2" {
			t.Fatalf("ID = %q, se esperaba la ultima", p.ID)
		}
	})

	t.Run("con periodo filtra", func(t *testing.T) {
		p, err := c.Ejecutar(context.Background(), "2025-06")
		if err != nil {
			t.Fatalf("Ejecutar: %v", err)
		}
		if p.ID != "pub-1" {
			t.Fatalf("ID = %q", p.ID)
		}
	})

	t.Run("periodo desconocido es no encontrado", func(t *testing.T) {
		_, err := c.Ejecutar(context.Background(), "2024")
		if !errors.Is(err, ErrNoEncontrado) {
			t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
		}
	})

	t.Run("periodo invalido", func(t *testing.T) {
		_, err := c.Ejecutar(context.Background(), "enero")
		if !errors.Is(err, ErrPeriodoInvalido) {
			t.Fatalf("se esperaba ErrPeriodoInvalido, se obtuvo %v", err)
		}
	})
}

func TestAnclaDePrescripcionNoSeReescribe(t *testing.T) {
	repo := &repoPublicacionMem{anclados: map[string]time.Time{
		"uso-1": momento,
	}}
	otra := momento.Add(3 * 365 * 24 * time.Hour)
	if err := repo.AnclarPrescripcion(context.Background(), []string{"uso-1"}, otra); err != nil {
		t.Fatalf("AnclarPrescripcion: %v", err)
	}
	if !repo.anclados["uso-1"].Equal(momento) {
		t.Fatal("reescribir el ancla resetearia R-19")
	}
}
