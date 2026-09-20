package aplicacion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

func usoDeCanal(canal string, mod reparto.Modalidad, grupo string) UsoDeReparto {
	return UsoDeReparto{
		Uso: UsoPersistido{
			ID:            "u-1",
			ObraID:        "obra-1",
			Modalidad:     mod,
			TipoObra:      "serie",
			CanalID:       canal,
			DuracionMin:   decimal.RequireFromString("48"),
			Emisiones:     10,
			Rating:        decimal.RequireFromString("9.0"),
			Taquilla:      decimal.RequireFromString("1000"),
			Espectadores:  decimal.RequireFromString("250"),
			Exhibiciones:  7,
			Vistas:        decimal.RequireFromString("1000"),
			MinutosVistos: decimal.RequireFromString("40000"),
			PB:            decimal.RequireFromString("1.3"),
		},
		GrupoEfectivo: grupo,
	}
}

func TestUsoDeRepartoConservaLaAtribucionDeCanalYLasMedidas(t *testing.T) {
	t.Parallel()

	u, err := aUsoDeReparto(usoDeCanal("rcn", reparto.TV, ""))
	if err != nil {
		t.Fatalf("mapear un uso valido: %v", err)
	}

	// El canal es lo que #119 anade: sin el no hay forma de saber contra que
	// bolsa pondera la fila.
	if u.CanalID != "rcn" {
		t.Errorf("CanalID = %q, se esperaba %q", u.CanalID, "rcn")
	}
	if u.ObraID != "obra-1" || u.TipoObra != "serie" || u.Emisiones != 10 {
		t.Errorf("identidad de la obra perdida: %+v", u)
	}
	if u.Exhibiciones != 7 {
		t.Errorf("Exhibiciones = %d, se esperaba 7 (RD 9.4)", u.Exhibiciones)
	}
	if !u.Espectadores.Equal(decimal.RequireFromString("250")) {
		t.Errorf("Espectadores = %s, se esperaba 250 (RD 9.2)", u.Espectadores)
	}
	for _, m := range []struct {
		nombre string
		got    decimal.Decimal
		quiero string
	}{
		{"DuracionMin", u.DuracionMin, "48"},
		{"Rating", u.Rating, "9.0"},
		{"Taquilla", u.Taquilla, "1000"},
		{"Vistas", u.Vistas, "1000"},
		{"MinutosVistos", u.MinutosVistos, "40000"},
		{"PB", u.PB, "1.3"},
	} {
		if !m.got.Equal(decimal.RequireFromString(m.quiero)) {
			t.Errorf("%s = %s, se esperaba %s", m.nombre, m.got, m.quiero)
		}
	}
}

// TestUsoDeRepartoSinObraEsErrorTipado comprueba la segunda linea de defensa:
// una fila pendiente, ONI o excluida tiene `obra_id` NULL, y
// COALESCE(obra_id, ”) la convierte en una obra fantasma de id "" si nada la
// detiene antes del motor. El filtro real vive en el SQL de UsosDeCanal; esto
// atrapa a un adaptador futuro que lo olvide.
func TestUsoDeRepartoSinObraEsErrorTipado(t *testing.T) {
	t.Parallel()

	f := usoDeCanal("rcn", reparto.TV, "")
	f.Uso.ObraID = ""
	f.Uso.ID = "u-sin-obra"

	_, err := aUsoDeReparto(f)
	if !errors.Is(err, ErrUsoSinObra) {
		t.Fatalf("una fila sin obra_id tiene que ser ErrUsoSinObra, no un Uso con ObraID vacio: %v", err)
	}
	if !strings.Contains(err.Error(), "u-sin-obra") {
		t.Errorf("el error no dice que fila fue: %v", err)
	}
}

func TestUsoDeRepartoConModalidadDesconocidaEsErrorTipado(t *testing.T) {
	t.Parallel()

	_, err := aUsoDeReparto(usoDeCanal("rcn", reparto.Modalidad("radio"), ""))
	if !errors.Is(err, reparto.ErrModalidadDesconocida) {
		t.Fatalf("una modalidad fuera del reglamento tiene que ser error tipado y no cero puntos: %v", err)
	}
}

func TestUsoDeRepartoSoloFijaGrupoEnSuscripcionYHotel(t *testing.T) {
	t.Parallel()

	casos := []struct {
		modalidad   reparto.Modalidad
		grupoQuiero reparto.GrupoCanal
	}{
		{reparto.TV, ""},
		{reparto.Cine, ""},
		{reparto.OTT, ""},
		{reparto.Teatro, ""},
		{reparto.Transporte, ""},
		{reparto.Suscripcion, reparto.GrupoPrivadosNacionales},
		{reparto.Hotel, reparto.GrupoPrivadosNacionales},
	}

	for _, c := range casos {
		t.Run(string(c.modalidad), func(t *testing.T) {
			t.Parallel()

			u, err := aUsoDeReparto(usoDeCanal("rcn", c.modalidad, "privado_nacional"))
			if err != nil {
				t.Fatalf("mapear %s: %v", c.modalidad, err)
			}
			// validarGrupos del motor rechaza un grupo no vacio fuera de
			// suscripcion, asi que arrastrarlo siempre romperia el reparto.
			if u.Grupo != c.grupoQuiero {
				t.Errorf("Grupo = %q, se esperaba %q", u.Grupo, c.grupoQuiero)
			}
		})
	}
}

func TestUsoDeSuscripcionSinClasificarEsErrorTipado(t *testing.T) {
	t.Parallel()

	_, err := aUsoDeReparto(usoDeCanal("rcn", reparto.Suscripcion, ""))
	if !errors.Is(err, reparto.ErrGrupoDesconocido) {
		t.Fatalf("un canal de suscripcion sin clasificacion anual (RD 9.5.4) "+
			"tiene que fallar ruidosamente y no repartir: %v", err)
	}
}

func TestAnioDeClasificacionEsElInmediatamenteAnterior(t *testing.T) {
	t.Parallel()

	casos := []struct {
		periodo string
		quiero  int
		falla   bool
	}{
		{"2025-01", 2024, false},
		{"2025", 2024, false},
		{"2025-13", 0, true},
		{"", 0, true},
		{"25-01", 0, true},
	}

	for _, c := range casos {
		anio, err := anioDeClasificacion(c.periodo)
		if c.falla {
			if err == nil {
				t.Errorf("periodo %q tendria que ser invalido", c.periodo)
			}
			continue
		}
		if err != nil {
			t.Errorf("periodo %q: %v", c.periodo, err)
			continue
		}
		if anio != c.quiero {
			t.Errorf("periodo %q -> %d, se esperaba %d", c.periodo, anio, c.quiero)
		}
	}
}

type usosDeCanalFalsos struct {
	porCanal    map[string][]UsoDeReparto
	resumen     ResumenUsosDeCanal
	sinCanal    int
	err         error
	errSinCanal error
	pedido      []string
}

func (u *usosDeCanalFalsos) UsosDeCanal(_ context.Context, periodo, canalID string, anio int) ([]UsoDeReparto, ResumenUsosDeCanal, error) {
	u.pedido = append(u.pedido, fmt.Sprintf("%s/%s/%d", periodo, canalID, anio))
	return u.porCanal[canalID], u.resumen, u.err
}

func (u *usosDeCanalFalsos) UsosSinCanal(_ context.Context, _ string) (int, error) {
	return u.sinCanal, u.errSinCanal
}

func TestUsosDeCanalPasaLosArgumentosYPropagaElResultado(t *testing.T) {
	t.Parallel()

	repo := &usosDeCanalFalsos{
		porCanal: map[string][]UsoDeReparto{
			"rcn": {usoDeCanal("rcn", reparto.TV, ""), usoDeCanal("rcn", reparto.TV, "")},
		},
		resumen: ResumenUsosDeCanal{Pendientes: 1, ONI: 2},
	}
	r := Reparto{Usos: repo}

	usos, resumen, err := r.UsosDeCanal(t.Context(), "2025-01", "rcn")
	if err != nil {
		t.Fatalf("reunir los usos de un canal: %v", err)
	}
	if len(usos) != 2 {
		t.Fatalf("usos = %d, se esperaban 2", len(usos))
	}
	if resumen != (ResumenUsosDeCanal{Pendientes: 1, ONI: 2}) {
		t.Errorf("el resumen de exclusiones no se propago: %+v", resumen)
	}
	// Una sola consulta, y la clasificacion se pide contra 2024: RD 9.5.4
	// mira el ano inmediatamente anterior al que se reparte.
	if len(repo.pedido) != 1 || repo.pedido[0] != "2025-01/rcn/2024" {
		t.Errorf("consulta = %v, se esperaba una sola por (periodo, canal, ano anterior)", repo.pedido)
	}
}

// TestUsosDeCanalRechazaUnCanalVacio: un canal vacio no identifica ninguna
// bolsa, y dejarlo pasar como filtro devolveria las filas sin atribuir de
// TODOS los pagadores mezcladas en una sola corrida.
func TestUsosDeCanalRechazaUnCanalVacio(t *testing.T) {
	t.Parallel()

	for _, canal := range []string{"", "   "} {
		repo := &usosDeCanalFalsos{}
		r := Reparto{Usos: repo}

		_, _, err := r.UsosDeCanal(t.Context(), "2025-01", canal)
		if !errors.Is(err, ErrCanalVacio) {
			t.Fatalf("canal %q: se esperaba ErrCanalVacio, se obtuvo %v", canal, err)
		}
		if len(repo.pedido) != 0 {
			t.Errorf("canal %q: se consulto el repositorio con un canal vacio", canal)
		}
	}
}

func TestUsosDeCanalPropagaElErrorDeUnaFilaConSuIdentificador(t *testing.T) {
	t.Parallel()

	malo := usoDeCanal("rcn", reparto.Modalidad("radio"), "")
	malo.Uso.ID = "u-rota"
	r := Reparto{Usos: &usosDeCanalFalsos{porCanal: map[string][]UsoDeReparto{"rcn": {malo}}}}

	_, _, err := r.UsosDeCanal(t.Context(), "2025-01", "rcn")
	if !errors.Is(err, reparto.ErrModalidadDesconocida) {
		t.Fatalf("el error de la fila tiene que llegar tipado arriba: %v", err)
	}
	if !strings.Contains(err.Error(), "u-rota") {
		t.Errorf("el error no dice que fila fue: %v", err)
	}
}

func TestUsosDeCanalRechazaUnPeriodoInvalidoSinTocarElRepositorio(t *testing.T) {
	t.Parallel()

	repo := &usosDeCanalFalsos{}
	r := Reparto{Usos: repo}

	if _, _, err := r.UsosDeCanal(t.Context(), "2025-13", "rcn"); err == nil {
		t.Fatal("un periodo fuera de rango tiene que fallar antes de consultar")
	}
	if len(repo.pedido) != 0 {
		t.Errorf("se consulto el repositorio con un periodo invalido: %v", repo.pedido)
	}
}

func TestUsosSinCanalPropagaElConteo(t *testing.T) {
	t.Parallel()

	r := Reparto{Usos: &usosDeCanalFalsos{sinCanal: 59}}
	n, err := r.UsosSinCanal(t.Context(), "2025-01")
	if err != nil {
		t.Fatalf("UsosSinCanal: %v", err)
	}
	if n != 59 {
		t.Fatalf("n = %d, se esperaban 59", n)
	}
}
