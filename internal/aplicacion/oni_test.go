package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/dominio/oni"
)

type repoPublicacionMem struct {
	pendientes      []oni.DatosIdentificatorios
	errPendientes   error
	guardadas       []PublicacionONI
	errGuardar      error
	anclados        map[string]time.Time
	errAnclar       error
	siguienteID     string
	bloqueosPeriodo []string
	errBloquear     error
}

func (r *repoPublicacionMem) BloquearPeriodoONI(_ context.Context, periodo string) error {
	if r.errBloquear != nil {
		return r.errBloquear
	}
	r.bloqueosPeriodo = append(r.bloqueosPeriodo, periodo)
	return nil
}

func (r *repoPublicacionMem) PendientesDePeriodo(_ context.Context, periodo string) ([]oni.DatosIdentificatorios, error) {
	if r.errPendientes != nil {
		return nil, r.errPendientes
	}
	var out []oni.DatosIdentificatorios
	for _, d := range r.pendientes {
		if d.Periodo != "" && d.Periodo != periodo {
			continue
		}
		if _, anclado := r.anclados[d.ID]; anclado {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func (r *repoPublicacionMem) GuardarPublicacion(_ context.Context, p PublicacionONI) (PublicacionONI, error) {
	if r.errGuardar != nil {
		return PublicacionONI{}, r.errGuardar
	}
	secuencia := 1
	for _, g := range r.guardadas {
		if g.Periodo == p.Periodo {
			if len(p.Obras) == 0 {
				return PublicacionONI{}, ErrYaPublicado
			}
			secuencia++
		}
	}
	p.Secuencia = secuencia
	if p.ID == "" {
		p.ID = r.siguienteID
		if p.ID == "" {
			p.ID = fmt.Sprintf("pub-%d", len(r.guardadas)+1)
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

func (r *repoPublicacionMem) PublicacionVigente(ctx context.Context) (PublicacionONI, error) {
	if len(r.guardadas) == 0 {
		return PublicacionONI{}, ErrNoEncontrado
	}
	ultima := r.guardadas[len(r.guardadas)-1]
	return r.PublicacionDePeriodo(ctx, ultima.Periodo)
}

func (r *repoPublicacionMem) PublicacionDePeriodo(_ context.Context, periodo string) (PublicacionONI, error) {
	var encontrada *PublicacionONI
	var todasObras []oni.ProyeccionPublica
	for i := range r.guardadas {
		g := &r.guardadas[i]
		if g.Periodo == periodo {
			encontrada = g
			todasObras = append(todasObras, g.Obras...)
		}
	}
	if encontrada == nil {
		return PublicacionONI{}, ErrNoEncontrado
	}
	res := *encontrada
	res.Obras = todasObras
	return res, nil
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

func (b *bitacoraMem) AsientoPorID(_ context.Context, _ string) (Asiento, error) {
	return Asiento{}, ErrNoEncontrado
}

func (b *bitacoraMem) ListarAsientos(_ context.Context, _ Paginacion) ([]Asiento, error) {
	return b.asientos, b.err
}

type txPassthrough struct{}

func (txPassthrough) EnUnidad(ctx context.Context, fn func(context.Context) error) error {
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

func TestPublicacionComplementariaONITardio(t *testing.T) {
	repo := &repoPublicacionMem{
		pendientes: []oni.DatosIdentificatorios{
			{ID: "uso-1", Titulo: "Serie X", Fuente: "caracol", Modalidad: "tv", Periodo: "2026-01"},
		},
		siguienteID: "pub-1",
	}
	bit := &bitacoraMem{}
	t1 := momento
	uc1 := PublicarListadoONI{
		ONI:         repo,
		Bitacora:    bit,
		Reloj:       relojFijo{instante: t1},
		Tx:          txPassthrough{},
		Fisica:      "Calle 74 #7-35, Bogota",
		Electronica: "oni@redescritores.com",
	}

	pub1, err := uc1.Ejecutar(context.Background(), "2026-01", "usr-admin")
	if err != nil {
		t.Fatalf("primera publicacion: %v", err)
	}
	if pub1.Secuencia != 1 {
		t.Fatalf("Secuencia primera = %d, se esperaba 1", pub1.Secuencia)
	}
	if len(pub1.Obras) != 1 || pub1.Obras[0].ID != "uso-1" {
		t.Fatalf("obras primera = %v", pub1.Obras)
	}
	if !repo.anclados["uso-1"].Equal(t1) {
		t.Fatalf("ancla uso-1 = %v, se esperaba %v", repo.anclados["uso-1"], t1)
	}

	// Republicar sin nuevos pendientes falla con ErrYaPublicado.
	_, err = uc1.Ejecutar(context.Background(), "2026-01", "usr-admin")
	if !errors.Is(err, ErrYaPublicado) {
		t.Fatalf("se esperaba ErrYaPublicado, se obtuvo %v", err)
	}

	// Llega un reporte tardio con un nuevo uso ONI.
	t2 := momento.Add(24 * time.Hour)
	repo.pendientes = append(repo.pendientes, oni.DatosIdentificatorios{
		ID:        "uso-tardio",
		Titulo:    "Capitulo Olvidado",
		Fuente:    "caracol",
		Modalidad: "tv",
		Periodo:   "2026-01",
	})
	repo.siguienteID = "pub-2"

	uc2 := PublicarListadoONI{
		ONI:         repo,
		Bitacora:    bit,
		Reloj:       relojFijo{instante: t2},
		Tx:          txPassthrough{},
		Fisica:      "Calle 74 #7-35, Bogota",
		Electronica: "oni@redescritores.com",
	}

	pub2, err := uc2.Ejecutar(context.Background(), "2026-01", "usr-admin")
	if err != nil {
		t.Fatalf("publicacion complementaria: %v", err)
	}
	if pub2.Secuencia != 2 {
		t.Fatalf("Secuencia complementaria = %d, se esperaba 2", pub2.Secuencia)
	}
	if len(pub2.Obras) != 1 || pub2.Obras[0].ID != "uso-tardio" {
		t.Fatalf("obras complementaria = %v, se esperaba solo el tardio", pub2.Obras)
	}
	if !repo.anclados["uso-tardio"].Equal(t2) {
		t.Fatalf("ancla uso-tardio = %v, se esperaba %v", repo.anclados["uso-tardio"], t2)
	}
	// El ancla del uso anterior no se reseteo.
	if !repo.anclados["uso-1"].Equal(t1) {
		t.Fatalf("ancla uso-1 cambio a %v", repo.anclados["uso-1"])
	}

	// Bitacora tiene 2 asientos.
	if len(bit.asientos) != 2 {
		t.Fatalf("asientos = %d, se esperaban 2", len(bit.asientos))
	}
	if bit.asientos[1].RefID != "pub-2" {
		t.Fatalf("asiento complementario ref = %s", bit.asientos[1].RefID)
	}

	// Consultar el listado del periodo consolida ambos.
	consultar := ConsultarListadoONI{ONI: repo}
	consolidada, err := consultar.Ejecutar(context.Background(), "2026-01")
	if err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if len(consolidada.Obras) != 2 {
		t.Fatalf("obras consolidadas = %d, se esperaban 2", len(consolidada.Obras))
	}

	// Intentar publicar de nuevo sin mas pendientes falla.
	_, err = uc2.Ejecutar(context.Background(), "2026-01", "usr-admin")
	if !errors.Is(err, ErrYaPublicado) {
		t.Fatalf("republicar sin pendientes: se esperaba ErrYaPublicado, se obtuvo %v", err)
	}
}

func TestPublicarTomaCerrojoDePeriodo(t *testing.T) {
	repo := &repoPublicacionMem{
		pendientes: []oni.DatosIdentificatorios{
			{ID: "uso-1", Titulo: "Serie X", Fuente: "caracol", Modalidad: "tv", Periodo: "2026-01"},
		},
	}
	uc := PublicarListadoONI{
		ONI:         repo,
		Bitacora:    &bitacoraMem{},
		Reloj:       relojFijo{instante: momento},
		Tx:          txPassthrough{},
		Fisica:      "Calle 74 #7-35, Bogota",
		Electronica: "oni@redescritores.com",
	}

	_, err := uc.Ejecutar(context.Background(), "2026-01", "usr-admin")
	if err != nil {
		t.Fatalf("publicar: %v", err)
	}
	if len(repo.bloqueosPeriodo) != 1 || repo.bloqueosPeriodo[0] != "2026-01" {
		t.Fatalf("se esperaba bloqueo de '2026-01', se obtuvo %v", repo.bloqueosPeriodo)
	}
}
