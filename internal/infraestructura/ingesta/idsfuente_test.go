package ingesta

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

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
		Similitud:      sinSimilitud{},
		Parametros:     umbralesFijos{},
		Unidad:         unidadDirecta{},
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

// Gemelo de Caracol: lo que persiste la ingesta es lo que sondea la cascada.
// Cierra el circulo entre MapaCaracol, la grafia id_ficha del contrato y el
// par canonico de la fuente en parCanonicoPorFuente. Sin id_ficha en el texto
// persistido, Alias se llamaria con otra clave y el verde no detectaria el
// desacuerdo.
func TestMapaCaracolProduceUnParQueLaCascadaSondeaPorIDFicha(t *testing.T) {
	t.Parallel()

	usos, err := MapaCaracol().Aplicar(Tabla{
		Columnas: []string{"Titulo", "ID_Ficha", "Programa ID_IMDB", "Duracion_total", "Fecha", "Hora"},
		Filas:    [][]string{{"Rebelde", "55174", "tt0100001", "45", "20241231", "0:00"}},
	})
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if got := aplicacion.LeerIDsFuente(usos[0].IDsFuente)[aplicacion.ClaveIDFicha]; got != "55174" {
		t.Fatalf("id_ficha persistido = %q", got)
	}

	uso := usos[0]
	uso.ID = "u-1"
	uso.Fuente = FuenteCaracol
	uso.Escalon = identificacion.EscalonPendiente
	uso.Modalidad = reparto.TV

	ids := &identificacionMemoria{
		alias: map[string]string{"caracol|id_ficha|55174": "obra-7"},
	}
	n, err := (aplicacion.ResolverUsos{
		Usos:           usosDelPeriodo{usos: []aplicacion.UsoPersistido{uso}},
		Identificacion: ids,
		Similitud:      sinSimilitud{},
		Parametros:     umbralesFijos{},
		Unidad:         unidadDirecta{},
	}).ResolverUsos(t.Context(), "2026-01")
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("resueltas = %d, se esperaba 1", n)
	}
	if len(ids.sondeos) != 1 || ids.sondeos[0] != "caracol|id_ficha|55174" {
		t.Fatalf("sondeos de alias = %v, se esperaba [caracol|id_ficha|55174]", ids.sondeos)
	}
}

// Gemelo de cine: lo que persiste la ingesta es lo que sondea la cascada.
// Cierra el circulo entre MapaCine, la grafia id_pelicula del contrato, la
// fuente "cine" del adaptador y el par canonico en parCanonicoPorFuente. Es
// ademas la prueba de cine a traves de la cascada que pide la entrada de
// "cine" en el mapa: sin ella, el fallback alfabetico pasaria por canonico.
func TestMapaCineProduceUnParQueLaCascadaSondeaPorIDPelicula(t *testing.T) {
	t.Parallel()

	usos, err := MapaCine().Aplicar(Tabla{
		Columnas: []string{"titulo", "id", "taquilla"},
		Filas:    [][]string{{"Pelicula X", "PX-1", "10000"}},
	})
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if got := aplicacion.LeerIDsFuente(usos[0].IDsFuente)[aplicacion.ClaveIDPelicula]; got != "PX-1" {
		t.Fatalf("id_pelicula persistido = %q", got)
	}

	uso := usos[0]
	uso.ID = "u-1"
	uso.Fuente = FuenteCine
	uso.Escalon = identificacion.EscalonPendiente
	uso.Modalidad = reparto.Cine

	ids := &identificacionMemoria{
		alias: map[string]string{"cine|id_pelicula|PX-1": "obra-9"},
	}
	n, err := (aplicacion.ResolverUsos{
		Usos:           usosDelPeriodo{usos: []aplicacion.UsoPersistido{uso}},
		Identificacion: ids,
		Similitud:      sinSimilitud{},
		Parametros:     umbralesFijos{},
		Unidad:         unidadDirecta{},
	}).ResolverUsos(t.Context(), "2026-01")
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("resueltas = %d, se esperaba 1", n)
	}
	if len(ids.sondeos) != 1 || ids.sondeos[0] != "cine|id_pelicula|PX-1" {
		t.Fatalf("sondeos de alias = %v, se esperaba [cine|id_pelicula|PX-1]", ids.sondeos)
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
	// Y dice que SOBRA, no que falta: con la fila corta ya rechazada, un
	// motivo de "coma perdida" en una fila ancha mandaria al cliente a buscar
	// una coma que no falta.
	if !strings.Contains(usos[1].RechazoMotivo, "campo de mas") ||
		strings.Contains(usos[1].RechazoMotivo, "fila corta") {
		t.Errorf("el motivo de la fila ancha no dice que sobra un campo: %s", usos[1].RechazoMotivo)
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
func (s usosDelPeriodo) ListarRechazos(context.Context) ([]aplicacion.UsoPersistido, error) {
	return nil, nil
}
func (s usosDelPeriodo) RechazosDeReporte(context.Context, string, aplicacion.Paginacion) ([]aplicacion.UsoPersistido, error) {
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
func (i *identificacionMemoria) GuardarCandidatos(context.Context, string, []identificacion.Candidato) error {
	return nil
}

// El escalon 3 no pinta nada en estas gemelas: un motor mudo y unos umbrales
// cualesquiera bastan para que la cascada corra entera.
type sinSimilitud struct{}

func (sinSimilitud) Candidatos(context.Context, string, decimal.Decimal) ([]identificacion.Candidato, error) {
	return nil, nil
}

// unidadDirecta corre fn sin transaccion: estas pruebas miran que se sondea, no
// como se confirma.
type unidadDirecta struct{}

func (unidadDirecta) EnUnidad(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type umbralesFijos struct{}

func (umbralesFijos) ParametroVigente(_ context.Context, clave string, _ time.Time) (decimal.Decimal, error) {
	switch clave {
	case aplicacion.ClaveUmbralMatch:
		return decimal.RequireFromString("0.60"), nil
	default:
		return decimal.RequireFromString("0.45"), nil
	}
}

// La simetrica de la de arriba (issue #113, punto 5). Una coma PERDIDA corre
// los valores a la izquierda igual que una de mas los corre a la derecha, y
// rellenar la fila corta en silencio escondia el primer caso: "Corrida,100"
// entraba con id=100 y la taquilla vacia.
//
// El motivo tiene que ser el del ancho, no el de la celda: con el corrimiento
// la taquilla viene vacia, y un "taquilla requerida vacia" mandaria al cliente
// a rellenar una celda cuando lo que falta es una coma.
func TestAplicarRechazaLaFilaCSVConCamposDeMenos(t *testing.T) {
	t.Parallel()

	datos := "titulo,id,taquilla\nBuena,PX-1,1\nCorrida,100\n"
	usos, err := lector(t, MapaCine(), aplicacion.FormatoCSV).Leer([]byte(datos))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if len(usos) != 2 {
		t.Fatalf("usos = %d, la fila corta no se descarta", len(usos))
	}
	if usos[0].RechazoMotivo != "" {
		t.Fatalf("la fila justa no deberia rechazarse: %s", usos[0].RechazoMotivo)
	}
	m := usos[1].RechazoMotivo
	for _, quiere := range []string{"fila 3", "trae 2 campos", "filas de este archivo traen 3"} {
		if !strings.Contains(m, quiere) {
			t.Errorf("el motivo no dice %q: %s", quiere, m)
		}
	}
	if usos[1].Titulo != "Corrida" {
		t.Errorf("la fila rechazada perdio el titulo: %+v", usos[1])
	}
}

// En .xlsx la fila corta NO es sospechosa: excelize recorta las celdas vacias
// del final, asi que es la forma normal de una fila con opcionales vacias.
func TestAplicarNoRechazaLaFilaXLSXQueExcelizeRecorta(t *testing.T) {
	t.Parallel()

	datos := xlsxDeCeldas(t, map[string]string{
		"A1": "titulo", "B1": "id", "C1": "taquilla", "D1": "espectadores",
		"A2": "Pelicula X", "B2": "PX-1", "C2": "100",
	})
	usos, err := lector(t, MapaCine(), aplicacion.FormatoXLSX).Leer(datos)
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if len(usos) != 1 || usos[0].RechazoMotivo != "" {
		t.Fatalf("la fila recortada por excelize no se rechaza: %+v", motivos(usos))
	}
}

// Una cabecera con coma final -- `titulo,id,taquilla,` -- trae una columna
// SIN NOMBRE al final. El xlsx real de Caracol trae lo mismo: una columna 49
// con cabecera vacia. Lo que sigue fija como se lee, y la regla es una sola:
// NINGUNA variante acepta una fila corrida; lo que no se puede decidir con
// seguridad se rechaza con motivo.
//
//   - El ancho esperado se decide POR ARCHIVO. Si alguna fila de datos llega
//     al ancho original de la cabecera, el archivo escribe la coma final, y
//     una fila por debajo de ese ancho es corta. Si ninguna llega, el ancho es
//     el de la ultima columna con nombre.
//   - Mas alla del ancho ORIGINAL de la cabecera, todo campo -- vacio o no --
//     hace la fila ancha.
//   - Un dato bajo una columna sin nombre rechaza la fila: no hay nombre al que
//     mandarlo.
func TestAplicarIgnoraLasColumnasFinalesSinNombreDeLaCabecera(t *testing.T) {
	t.Parallel()

	for nombre, datos := range map[string]string{
		"ninguna fila escribe la coma final":  "titulo,id,taquilla,\nA,PX-1,1\nB,PX-2,2\n",
		"todas escriben la coma final":        "titulo,id,taquilla,\nA,PX-1,1,\nB,PX-2,2, \n",
		"varias sin nombre, ninguna coma":     "titulo,id,taquilla,, \nA,PX-1,1\nB,PX-2,2\n",
		"varias sin nombre, todas las comas":  "titulo,id,taquilla,, \nA,PX-1,1,,\nB,PX-2,2,,\n",
		"comas parciales sin llegar al ancho": "titulo,id,taquilla,, \nA,PX-1,1,\nB,PX-2,2\n",
	} {
		t.Run(nombre, func(t *testing.T) {
			t.Parallel()
			usos, err := lector(t, MapaCine(), aplicacion.FormatoCSV).Leer([]byte(datos))
			if err != nil {
				t.Fatalf("Leer: %v", err)
			}
			for i, u := range usos {
				if u.RechazoMotivo != "" {
					t.Errorf("fila %d rechazada por la coma final de la cabecera: %s", i, u.RechazoMotivo)
				}
			}
		})
	}
}

// Las variantes que NO se aceptan, cada una con el motivo que la explica.
func TestAplicarNoAceptaCorridoAlrededorDeColumnasSinNombre(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		datos  string
		// motivos esperados por fila, "" = aceptada
		quiere []string
	}{
		{
			// H1 de la segunda auditoria: la cabecera NO tiene coma final, y la
			// coma de mas de "Rapido, furioso" deja una celda vacia al final.
			// Recortarla hacia entrar la fila corrida (id=furioso, taquilla=55).
			nombre: "coma de mas con la ultima celda vacia",
			datos:  "titulo,id,taquilla,moneda\nRapido, furioso,55,100,\nB,PX-2,2,COP\n",
			quiere: []string{"campo de mas", ""},
		},
		{
			nombre: "vacios mas alla de una cabecera sin coma final",
			datos:  "titulo,id,taquilla\nA,PX-1,1\nB,PX-2,2,,,\n",
			quiere: []string{"", "campo de mas"},
		},
		{
			// H2: con la coma final en todas las filas, la coma PERDIDA de la
			// fila 3 la deja justo en el ancho de las columnas con nombre.
			nombre: "coma perdida en un archivo que escribe la coma final",
			datos:  "titulo,id,taquilla,espectadores,\nA,55,100,7,\nA55,100,7,\n",
			quiere: []string{"", "fila corta"},
		},
		{
			// Mezcla: la primera fila escribe las dos comas finales, la segunda
			// ninguna. No se puede saber cual de las dos perdio algo, asi que
			// la que no llega al ancho del archivo se rechaza con motivo.
			nombre: "anchos mezclados con varias columnas sin nombre",
			datos:  "titulo,id,taquilla,, \nA,PX-1,1,,\nB,PX-2,2\n",
			quiere: []string{"", "fila corta"},
		},
		{
			nombre: "dato bajo la columna final sin nombre",
			datos:  "titulo,id,taquilla,\nA,PX-1,1,valor huerfano\nB,PX-2,2,\n",
			quiere: []string{"columna 4, que no tiene nombre", ""},
		},
		{
			// H3: una columna sin nombre EN MEDIO se conserva en su posicion, y
			// lo que traiga no se descarta en silencio.
			nombre: "dato bajo una columna sin nombre en medio",
			datos:  "titulo,,id,taquilla\nA,huerfano,PX-1,1\nB,,PX-2,2\n",
			quiere: []string{"columna 2, que no tiene nombre", ""},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			usos, err := lector(t, MapaCine(), aplicacion.FormatoCSV).Leer([]byte(c.datos))
			if err != nil {
				t.Fatalf("Leer: %v", err)
			}
			if len(usos) != len(c.quiere) {
				t.Fatalf("usos = %d, se esperaban %d", len(usos), len(c.quiere))
			}
			for i, q := range c.quiere {
				m := usos[i].RechazoMotivo
				if q == "" && m != "" {
					t.Errorf("fila %d rechazada: %s", i, m)
				}
				if q != "" && !strings.Contains(m, q) {
					t.Errorf("fila %d: el motivo no dice %q: %q", i, q, m)
				}
			}
		})
	}
}

// H3: la columna sin nombre en medio conserva las posiciones de las de detras.
// Borrar todas las cabeceras vacias correria `id` y `taquilla` una posicion.
func TestTablaCSVConservaLaColumnaSinNombreEnMedio(t *testing.T) {
	t.Parallel()

	tabla, err := TablaCSV([]byte("a,,b\n1,,2\n"))
	if err != nil {
		t.Fatalf("TablaCSV: %v", err)
	}
	if want := []string{"a", "", "b"}; !slices.Equal(tabla.Columnas, want) {
		t.Fatalf("columnas = %q, se esperaban %q", tabla.Columnas, want)
	}
	if tabla.Filas[0][2] != "2" {
		t.Errorf("la celda de `b` se corrio: %q", tabla.Filas[0])
	}

	usos, err := MapaCine().Aplicar(Tabla{
		Columnas: []string{"titulo", "", "id", "taquilla"},
		Filas:    [][]string{{"A", "", "PX-1", "7"}},
	})
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if usos[0].RechazoMotivo != "" || usos[0].Taquilla.String() != "7" ||
		aplicacion.LeerIDsFuente(usos[0].IDsFuente)[aplicacion.ClaveIDPelicula] != "PX-1" {
		t.Errorf("las columnas de detras se corrieron: %+v", usos[0])
	}
}

// En .xlsx, igual: la cabecera de Caracol trae una columna final vacia.
func TestAplicarIgnoraLaColumnaFinalSinNombreEnXLSX(t *testing.T) {
	t.Parallel()

	datos := xlsxDeCeldas(t, map[string]string{
		"A1": "titulo", "B1": "id", "C1": "taquilla", "D1": " ",
		"A2": "Pelicula X", "B2": "PX-1", "C2": "100",
		"A3": "Otra", "B3": "PX-2", "C3": "5", "D3": "huerfano",
	})
	usos, err := lector(t, MapaCine(), aplicacion.FormatoXLSX).Leer(datos)
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if usos[0].RechazoMotivo != "" {
		t.Errorf("fila 2 rechazada: %s", usos[0].RechazoMotivo)
	}
	if !strings.Contains(usos[1].RechazoMotivo, "columna 4, que no tiene nombre") {
		t.Errorf("el dato en la columna sin nombre no se rechazo: %q", usos[1].RechazoMotivo)
	}
}
