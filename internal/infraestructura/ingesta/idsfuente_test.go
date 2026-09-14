package ingesta

import (
	"context"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Los tres mapas tienen que persistir ids_fuente en el formato del ADR 0018.
// Sin el par clave=valor, entradaDesdeUso tira la linea y el escalon 1 no
// casa nunca: un reporte ingerido no resolveria por alias.
func TestLosMapasEscribenIDsFuenteDelContrato(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		mapa   Mapa
		tabla  Tabla
		quiere string
	}{
		{
			nombre: "caracol: id_ficha e imdb",
			mapa:   MapaCaracol(),
			tabla: Tabla{
				Columnas: []string{"Titulo", "ID_Ficha", "Programa ID_IMDB", "Duracion_total", "Fecha", "Hora"},
				Filas:    [][]string{{"Rebelde", "55174", "tt0100001", "45", "20241231", "0:00"}},
			},
			quiere: "id_ficha=55174\nimdb=tt0100001",
		},
		{
			nombre: "netflix: show_id, series_id y netflix_id",
			mapa:   MapaNetflix(),
			tabla: Tabla{
				Columnas: []string{"show_name", "show_id", "series_id", "netflix_id", "stream_starts"},
				Filas:    [][]string{{"My Horrible Boss", "80141259", "81004793", "81003997", "2172"}},
			},
			quiere: "netflix_id=81003997\nseries_id=81004793\nshow_id=80141259",
		},
		{
			nombre: "cine: id_pelicula",
			mapa:   MapaCine(),
			tabla: Tabla{
				Columnas: []string{"titulo", "id", "taquilla"},
				Filas:    [][]string{{"Pelicula X", "PX-1", "10000"}},
			},
			quiere: "id_pelicula=PX-1",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			usos, err := c.mapa.Aplicar(c.tabla)
			if err != nil {
				t.Fatalf("Aplicar: %v", err)
			}
			if len(usos) != 1 || usos[0].RechazoMotivo != "" {
				t.Fatalf("usos = %+v", usos)
			}
			if usos[0].IDsFuente != c.quiere {
				t.Errorf("ids_fuente = %q, se esperaba %q", usos[0].IDsFuente, c.quiere)
			}
			if aplicacion.LeerIDsFuente(usos[0].IDsFuente) == nil {
				t.Fatal("LeerIDsFuente no deberia devolver nil")
			}
		})
	}
}

// La prueba que ata las dos mitades: lo que persiste la ingesta es lo que
// sondea la cascada. Sin show_id en el texto persistido, Alias se llamaria
// con otra clave (o no se llamaria) y el verde no detectaria el desacuerdo.
func TestMapaNetflixProduceUnParQueLaCascadaSondeaPorShowID(t *testing.T) {
	t.Parallel()

	usos, err := MapaNetflix().Aplicar(Tabla{
		Columnas: []string{"show_name", "show_id", "series_id", "netflix_id", "stream_starts"},
		Filas:    [][]string{{"My Horrible Boss", "80141259", "81004793", "81003997", "2172"}},
	})
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if got := aplicacion.LeerIDsFuente(usos[0].IDsFuente)[aplicacion.ClaveShowID]; got != "80141259" {
		t.Fatalf("show_id persistido = %q", got)
	}

	uso := usos[0]
	uso.ID = "u-1"
	uso.Fuente = FuenteNetflix
	uso.Escalon = identificacion.EscalonPendiente
	uso.Modalidad = reparto.OTT

	ids := &identificacionMemoria{
		alias: map[string]string{"netflix|show_id|80141259": "obra-12"},
	}
	n, err := (aplicacion.ResolverUsos{
		Usos:           usosDelPeriodo{usos: []aplicacion.UsoPersistido{uso}},
		Identificacion: ids,
	}).ResolverUsos(t.Context(), "2018")
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("resueltas = %d, se esperaba 1", n)
	}
	if len(ids.sondeos) != 1 || ids.sondeos[0] != "netflix|show_id|80141259" {
		t.Fatalf("sondeos de alias = %v, se esperaba [netflix|show_id|80141259]", ids.sondeos)
	}
}

func TestAplicarRechazaLaFilaConCamposDeMas(t *testing.T) {
	t.Parallel()

	tabla, err := TablaCSV([]byte("titulo,duracion\nbuena,10\ncorrida,20,valor que no iba\n"))
	if err != nil {
		t.Fatalf("TablaCSV: %v", err)
	}
	if len(tabla.Filas) != 2 || len(tabla.Filas[1]) <= len(tabla.Columnas) {
		t.Fatalf("la fila ancha se recorto: %q", tabla.Filas)
	}

	usos, err := mapaMinimo().Aplicar(tabla)
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if usos[0].RechazoMotivo != "" {
		t.Fatalf("la fila justa no deberia rechazarse: %s", usos[0].RechazoMotivo)
	}
	if !strings.Contains(usos[1].RechazoMotivo, "campos") || !strings.Contains(usos[1].RechazoMotivo, "fila 3") {
		t.Errorf("el motivo no nombra el desajuste ni la linea: %s", usos[1].RechazoMotivo)
	}
	if usos[1].Titulo != "corrida" {
		t.Errorf("la fila rechazada perdio el titulo: %+v", usos[1])
	}
}

type usosDelPeriodo struct {
	usos []aplicacion.UsoPersistido
}

func (s usosDelPeriodo) GuardarReporte(context.Context, string, string, string, string, string, int) error {
	return nil
}
func (s usosDelPeriodo) GuardarUsos(context.Context, []aplicacion.UsoPersistido) error { return nil }
func (s usosDelPeriodo) GuardarEntrega(context.Context, aplicacion.Reporte, []aplicacion.UsoPersistido) error {
	return nil
}
func (s usosDelPeriodo) UsosSinResolver(context.Context) ([]aplicacion.UsoPersistido, error) {
	return nil, nil
}
func (s usosDelPeriodo) UsoPorID(context.Context, string) (aplicacion.UsoPersistido, error) {
	return aplicacion.UsoPersistido{}, nil
}
func (s usosDelPeriodo) UsosDePeriodo(context.Context, string) ([]aplicacion.UsoPersistido, error) {
	return s.usos, nil
}
func (s usosDelPeriodo) ListarCargas(context.Context, string) ([]aplicacion.CargaReporte, error) {
	return nil, nil
}

type identificacionMemoria struct {
	alias   map[string]string
	sondeos []string
}

func (i *identificacionMemoria) Alias(_ context.Context, fuente, tipo, valor string) (string, error) {
	clave := fuente + "|" + tipo + "|" + valor
	i.sondeos = append(i.sondeos, clave)
	if obraID, ok := i.alias[clave]; ok {
		return obraID, nil
	}
	return "", aplicacion.ErrNoEncontrado
}
func (i *identificacionMemoria) GuardarAlias(context.Context, string, string, string, string, string) error {
	return nil
}
func (i *identificacionMemoria) ObraPorIDGlobal(context.Context, string, string, string) (string, error) {
	return "", aplicacion.ErrNoEncontrado
}
func (i *identificacionMemoria) GuardarMatch(context.Context, string, string, identificacion.Resultado) error {
	return nil
}
