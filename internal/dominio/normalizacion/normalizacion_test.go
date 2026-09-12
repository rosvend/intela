package normalizacion

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func params() Parametros {
	return Parametros{
		DuracionArtisticaPct: decimal.RequireFromString("0.80"),
		MinutosHoraTV:        decimal.NewFromInt(48),
		MonedaBase:           "COP",
		MonedasReconocidas:   []string{"COP", "USD", "EUR"},
		TRM:                  decimal.RequireFromString("4000"),
	}
}

func d(s string) decimal.Decimal {
	return decimal.RequireFromString(s)
}

func filaTV() Fila {
	return Fila{
		ID:        "u-1",
		Fuente:    "caracol",
		Modalidad: ModalidadTV,
		Titulo:    "Serie Y",
		IDsFuente: "ID_Ficha=29",
		TipoObra:  "serie",
		Fecha:     "20241231",
		Hora:      "20:00:00",
		Duracion:  "60",
		Emisiones: "10",
		Rating:    "9.0",
	}
}

// ---------------------------------------------------------------------------
// Duracion: 80% y hora de 48 minutos, cada una y el borde.
// ---------------------------------------------------------------------------

func TestDuracionArtisticaYHoraTelevisiva(t *testing.T) {
	casos := []struct {
		nombre   string
		entrada  decimal.Decimal
		pct      decimal.Decimal
		horas    bool
		minHora  decimal.Decimal
		esperada decimal.Decimal
	}{
		{"80% de 60 minutos es la hora televisiva del ejemplo", d("60"), d("0.80"), false, d("48"), d("48")},
		{"80% de 87.5 es la pelicula X del ejemplo", d("87.5"), d("0.80"), false, d("48"), d("70")},
		{"cero se queda en cero", d("0"), d("0.80"), false, d("48"), d("0")},
		{"un minuto artistico", d("1"), d("0.80"), false, d("48"), d("0.8")},
		{"una hora reportada son 48 minutos", d("1"), d("0.80"), true, d("48"), d("48")},
		{"hora y media", d("1.5"), d("0.80"), true, d("48"), d("72")},
		{"coeficiente distinto del snapshot", d("100"), d("0.75"), false, d("48"), d("75")},
		{"hora televisiva distinta del snapshot", d("1"), d("0.80"), true, d("50"), d("50")},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var got decimal.Decimal
			if c.horas {
				got = MinutosDeHoraTelevisiva(c.entrada, c.minHora)
			} else {
				got = DuracionArtistica(c.entrada, c.pct)
			}
			if !got.Equal(c.esperada) {
				t.Fatalf("got %s, se esperaba %s", got, c.esperada)
			}
		})
	}
}

func TestNormalizarNoEncadena80Y48(t *testing.T) {
	// Si se aplicaran las dos, 60 min -> 48 -> 38.4, y el ejemplo de
	// formulas.md 9.1 (Serie Y, duracion 48.0) dejaria de cuadrar.
	f := filaTV()
	f.Duracion = "60"
	f.UnidadDuracion = "minutos"

	u, rev := Normalizar(f, params())
	if rev != nil {
		t.Fatalf("revision inesperada: %s", rev.Motivo())
	}
	if !u.DuracionMin.Equal(d("48")) {
		t.Fatalf("DuracionMin = %s, se esperaba 48 (solo el 80%%)", u.DuracionMin)
	}

	f.Duracion = "1"
	f.UnidadDuracion = "horas"
	u, rev = Normalizar(f, params())
	if rev != nil {
		t.Fatalf("revision inesperada: %s", rev.Motivo())
	}
	if !u.DuracionMin.Equal(d("48")) {
		t.Fatalf("DuracionMin = %s, se esperaba 48 (solo la hora televisiva)", u.DuracionMin)
	}
}

func TestAutopromoNoComputaYNoSeDescarta(t *testing.T) {
	f := filaTV()
	f.Autopromo = true
	f.Duracion = "60"

	u, rev := Normalizar(f, params())
	if rev != nil {
		t.Fatalf("un avance de programacion propia no va a revision: %s", rev.Motivo())
	}
	if !u.DuracionMin.IsZero() {
		t.Fatalf("DuracionMin = %s, los avances no computan", u.DuracionMin)
	}
	if u.Titulo != "Serie Y" {
		t.Fatal("la fila se perdio: un avance se aparta de la ponderacion, no del archivo")
	}
	if !u.Autopromo {
		t.Fatal("Autopromo tiene que sobrevivir: es la evidencia de por que no puntua")
	}
}

func TestDuracionTVSinParametrosVaARevision(t *testing.T) {
	f := filaTV()
	_, rev := Normalizar(f, Parametros{})
	if rev == nil || rev.Codigo != CodigoParametroAusente {
		t.Fatalf("sin coeficientes en el snapshot tiene que ir a revision, no inventar 80%%: %+v", rev)
	}
	if !strings.Contains(rev.Motivo(), "duracion") {
		t.Fatalf("el motivo tiene que nombrar el campo: %s", rev.Motivo())
	}
}

// ---------------------------------------------------------------------------
// Fechas: YYYYMMDD, ISO, serial de Excel, inparseable.
// ---------------------------------------------------------------------------

func TestParsearFecha(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   Fecha
		falla  bool
	}{
		{"vacia no es un fallo", "", Fecha{}, false},
		{"entero YYYYMMDD de la parrilla Caracol", "20241231", Fecha{2024, 12, 31}, false},
		{"YYYYMMDD con cola de Excel float", "20241231.0", Fecha{2024, 12, 31}, false},
		{"ISO con guion", "2024-12-31", Fecha{2024, 12, 31}, false},
		{"ISO con barra", "2024/12/31", Fecha{2024, 12, 31}, false},
		{"ISO con objeto de tiempo", "2024-12-31T20:00:00", Fecha{2024, 12, 31}, false},
		{"serial Excel 2024-12-31", "45657", Fecha{2024, 12, 31}, false},
		{"serial Excel con fraccion de hora", "45657.833333", Fecha{2024, 12, 31}, false},
		{"epoch Unix en serial Excel", "25569", Fecha{1970, 1, 1}, false},
		{"29 de febrero bisiesto", "20240229", Fecha{2024, 2, 29}, false},
		{"29 de febrero que no existe", "20250229", Fecha{}, true},
		{"mes 13", "20241301", Fecha{}, true},
		{"prosa", "ayer", Fecha{}, true},
		{"serial 60, el 29 de febrero inventado de Excel", "60", Fecha{}, true},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := ParsearFecha(c.in)
			if c.falla {
				if err == nil {
					t.Fatalf("se esperaba error, se obtuvo %s", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsearFecha(%q): %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("got %+v, se esperaba %+v", got, c.want)
			}
		})
	}
}

func TestParsearHora(t *testing.T) {
	casos := []struct {
		in    string
		want  string
		falla bool
	}{
		{"", "", false},
		{"20:00:00", "20:00:00", false},
		{"20:00", "20:00:00", false},
		{"0.5", "12:00:00", false},
		{"0.833333333", "20:00:00", false},
		{"25:00", "", true},
		{"mediodia", "", true},
	}

	for _, c := range casos {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParsearHora(c.in)
			if c.falla {
				if err == nil {
					t.Fatalf("se esperaba error, se obtuvo %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsearHora(%q): %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("got %q, se esperaba %q", got, c.want)
			}
		})
	}
}

func TestFechaInparseableMarcaRevisionYNoSePoneACero(t *testing.T) {
	f := filaTV()
	f.Fecha = "no-es-una-fecha"

	u, rev := Normalizar(f, params())
	if rev == nil || rev.Codigo != CodigoFechaInparseable {
		t.Fatalf("se esperaba fecha_inparseable, se obtuvo %+v", rev)
	}
	if !u.Fecha.EsCero() {
		t.Fatalf("la fecha canonica no se inventa: %+v", u.Fecha)
	}
	if u.Titulo != "Serie Y" {
		t.Fatal("la fila no se descarta: identidad tiene que sobrevivir")
	}
}

func TestSerialExcelConFraccionRellenaLaHora(t *testing.T) {
	f := filaTV()
	f.Fecha = "45657.5"
	f.Hora = ""

	u, rev := Normalizar(f, params())
	if rev != nil {
		t.Fatalf("revision inesperada: %s", rev.Motivo())
	}
	if u.Fecha.String() != "2024-12-31" {
		t.Fatalf("Fecha = %s", u.Fecha)
	}
	if u.Hora != "12:00:00" {
		t.Fatalf("Hora = %q, se esperaba 12:00:00 de la fraccion 0.5", u.Hora)
	}
}

// ---------------------------------------------------------------------------
// Moneda: desconocida a revision, no a cero.
// ---------------------------------------------------------------------------

func TestMonedaDesconocidaNoSePoneACero(t *testing.T) {
	casos := []struct {
		nombre string
		fila   Fila
		codigo string
	}{
		{
			nombre: "ISO que no esta en el snapshot",
			fila: Fila{
				Modalidad: ModalidadCine, Titulo: "Pelicula X",
				Taquilla: "1000000", Moneda: "JPY",
			},
			codigo: CodigoMonedaDesconocida,
		},
		{
			nombre: "taquilla sin moneda",
			fila: Fila{
				Modalidad: ModalidadCine, Titulo: "Pelicula X",
				Taquilla: "1000000",
			},
			codigo: CodigoMonedaDesconocida,
		},
		{
			nombre: "moneda en una fila de TV",
			fila: Fila{
				Modalidad: ModalidadTV, Titulo: "Serie Y",
				Duracion: "60", Moneda: "USD",
			},
			codigo: CodigoMonedaDesconocida,
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			u, rev := Normalizar(c.fila, params())
			if rev == nil || rev.Codigo != c.codigo {
				t.Fatalf("se esperaba %s, se obtuvo %+v", c.codigo, rev)
			}
			if u.Taquilla.GreaterThan(decimal.Zero) {
				t.Fatalf("no puede colarse una taquilla convertida: %s", u.Taquilla)
			}
			if !strings.Contains(rev.Motivo(), "no se pone a cero") &&
				!strings.Contains(rev.Motivo(), "no trae importes") &&
				!strings.Contains(rev.Motivo(), "sin moneda") {
				t.Fatalf("el motivo tiene que dejar claro que no se silencia: %s", rev.Motivo())
			}
			if u.Titulo == "" {
				t.Fatal("la fila no se descarta")
			}
		})
	}
}

func TestTaquillaEnMonedaBaseYConversionPorTRM(t *testing.T) {
	t.Run("ya en COP", func(t *testing.T) {
		u, rev := Normalizar(Fila{
			Modalidad: ModalidadCine, Titulo: "Pelicula X",
			Taquilla: "1575.50", Moneda: "cop",
		}, params())
		if rev != nil {
			t.Fatalf("revision: %s", rev.Motivo())
		}
		if !u.Taquilla.Equal(d("1575.50")) {
			t.Fatalf("Taquilla = %s", u.Taquilla)
		}
	})
	t.Run("USD por TRM", func(t *testing.T) {
		u, rev := Normalizar(Fila{
			Modalidad: ModalidadCine, Titulo: "Pelicula X",
			Taquilla: "10", Moneda: "USD",
		}, params())
		if rev != nil {
			t.Fatalf("revision: %s", rev.Motivo())
		}
		if !u.Taquilla.Equal(d("40000")) {
			t.Fatalf("Taquilla = %s, se esperaba 10 * 4000", u.Taquilla)
		}
	})
	t.Run("USD sin TRM", func(t *testing.T) {
		p := params()
		p.TRM = decimal.Zero
		_, rev := Normalizar(Fila{
			Modalidad: ModalidadCine, Titulo: "Pelicula X",
			Taquilla: "10", Moneda: "USD",
		}, p)
		if rev == nil || rev.Codigo != CodigoParametroAusente {
			t.Fatalf("sin TRM no se inventa un tipo de cambio: %+v", rev)
		}
	})
}

// ---------------------------------------------------------------------------
// Esquema canonico compartido entre fuentes.
// ---------------------------------------------------------------------------

func TestFuentesDistintasCompartenElEsquemaCanonico(t *testing.T) {
	tv, rev := Normalizar(filaTV(), params())
	if rev != nil {
		t.Fatalf("tv: %s", rev.Motivo())
	}
	cine, rev := Normalizar(Fila{
		ID: "c-1", Fuente: "procinal", Modalidad: ModalidadCine,
		Titulo: "Pelicula X", Taquilla: "100", Moneda: "COP", Fecha: "2024-06-01",
	}, params())
	if rev != nil {
		t.Fatalf("cine: %s", rev.Motivo())
	}
	ott, rev := Normalizar(Fila{
		ID: "o-1", Fuente: "netflix", Modalidad: ModalidadOTT,
		Titulo: "Show Z", Vistas: "1200", MinutosVistos: "40",
		Fecha: "2018-12-31",
	}, params())
	if rev != nil {
		t.Fatalf("ott: %s", rev.Motivo())
	}

	// El tipo es uno. Las metricas ajenas a la modalidad quedan en cero, no
	// ausentes: es lo que hace que el motor no tenga que conocer la fuente.
	if tv.Vistas.GreaterThan(decimal.Zero) || tv.Taquilla.GreaterThan(decimal.Zero) {
		t.Fatalf("TV no pondera con vistas ni taquilla: %+v", tv)
	}
	if cine.DuracionMin.GreaterThan(decimal.Zero) || cine.Vistas.GreaterThan(decimal.Zero) {
		t.Fatalf("cine no pondera con duracion ni vistas: %+v", cine)
	}
	if ott.Taquilla.GreaterThan(decimal.Zero) {
		t.Fatalf("OTT no pondera con taquilla: %+v", ott)
	}

	for _, u := range []Uso{tv, cine, ott} {
		if u.Modalidad == "" || u.Titulo == "" {
			t.Fatalf("identidad incompleta: %+v", u)
		}
		if u.Fecha.EsCero() {
			t.Fatalf("fecha canonica ausente: %+v", u)
		}
		if u.Emisiones < 1 {
			t.Fatalf("emisiones = %d, el default es 1", u.Emisiones)
		}
	}
	if tv.Fecha.String() != "2024-12-31" {
		t.Fatalf("YYYYMMDD no canonizo: %s", tv.Fecha)
	}
	if cine.Fecha.String() != "2024-06-01" {
		t.Fatalf("ISO no canonizo: %s", cine.Fecha)
	}
}

func TestMedidaNegativaVaARevision(t *testing.T) {
	f := filaTV()
	f.Duracion = "-10"
	_, rev := Normalizar(f, params())
	if rev == nil || rev.Codigo != CodigoMedidaInvalida {
		t.Fatalf("una duracion negativa no se pone a cero: %+v", rev)
	}
}

func TestModalidadFueraDeVocabulario(t *testing.T) {
	_, rev := Normalizar(Fila{Modalidad: "radio", Titulo: "Noticiero"}, params())
	if rev == nil || rev.Codigo != CodigoModalidadInvalida {
		t.Fatalf("se esperaba modalidad_invalida: %+v", rev)
	}
}

func TestNormalizarEsIdempotente(t *testing.T) {
	f := filaTV()
	a, ra := Normalizar(f, params())
	b, rb := Normalizar(f, params())
	if ra != nil || rb != nil {
		t.Fatalf("revision inesperada: %v %v", ra, rb)
	}
	if !a.DuracionMin.Equal(b.DuracionMin) || a.Fecha != b.Fecha || a.Hora != b.Hora {
		t.Fatalf("la misma fila dos veces tiene que dar el mismo Uso: %+v vs %+v", a, b)
	}
}
