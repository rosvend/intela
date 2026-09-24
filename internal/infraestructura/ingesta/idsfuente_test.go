package ingesta

import (
	"context"
	"fmt"
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
//   - El ancho esperado se decide POR ARCHIVO y POR MAYORIA, entre el de las
//     columnas con nombre y el ancho original de la cabecera. Con empate gana
//     el mayor. Solo se rechaza la minoria: una fila que no llega es corta,
//     una que se pasa (sin salir de la cabecera) trae un campo de mas.
//   - Mas alla del ancho ORIGINAL de la cabecera, todo campo -- vacio o no --
//     hace la fila ancha.
//   - Un dato bajo una columna sin nombre rechaza la fila: no hay nombre al que
//     mandarlo.
func TestAplicarIgnoraLasColumnasFinalesSinNombreDeLaCabecera(t *testing.T) {
	t.Parallel()

	for nombre, datos := range map[string]string{
		"ninguna fila escribe la coma final": "titulo,id,taquilla,\nA,PX-1,1\nB,PX-2,2\n",
		"todas escriben la coma final":       "titulo,id,taquilla,\nA,PX-1,1,\nB,PX-2,2, \n",
		"varias sin nombre, ninguna coma":    "titulo,id,taquilla,, \nA,PX-1,1\nB,PX-2,2\n",
		"varias sin nombre, todas las comas": "titulo,id,taquilla,, \nA,PX-1,1,,\nB,PX-2,2,,\n",
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
			quiere: []string{"mezcla filas de 4 y 5 campos (1 y 1 filas)", "mezcla filas de 4 y 5 campos"},
		},
		{
			// Mezcla: la primera fila escribe las dos comas finales, la segunda
			// ninguna. No se puede saber cual de las dos perdio algo, asi que
			// la que no llega al ancho del archivo se rechaza con motivo.
			nombre: "anchos mezclados con varias columnas sin nombre",
			datos:  "titulo,id,taquilla,, \nA,PX-1,1,,\nB,PX-2,2\n",
			quiere: []string{"mezcla filas de 3 y 5 campos", "mezcla filas de 3 y 5 campos"},
		},
		{
			// J1 de la tercera auditoria: UNA fila con coma final en un archivo
			// que no la escribe. 3 de 4 es el 75 %, por debajo del umbral: no
			// se puede saber cual es la buena y caen las cuatro, con un motivo
			// que lo dice. Con 58 de 59 (Caracol) si manda la mayoria; ver
			// TestLosArchivosRealesEnCSVEntranEnterosEnTodasSusFormas.
			nombre: "una fila con coma final en un archivo corto que no la escribe",
			datos:  "titulo,id,taquilla,\nA,PX-1,1\nB,PX-2,2\nC,PX-3,3,\nD,PX-4,4\n",
			quiere: []string{"mezcla filas de 3 y 4 campos (3 y 1 filas)", "mezcla", "mezcla", "mezcla"},
		},
		{
			// El silencio que evita rechazar esa minoria: una coma de mas en el
			// titulo con la ultima celda en blanco entraria con id=furioso.
			nombre: "coma de mas con celda final en blanco bajo la columna sin nombre",
			datos:  "titulo,id,taquilla,\nA,PX-1,1\nRapido, furioso,55,\nD,PX-4,4\n",
			quiere: []string{"mezcla", "mezcla", "mezcla"},
		},
		{
			// H2 con tres filas escribiendo la coma y la cuarta perdiendola: 75 %,
			// por debajo del umbral. Caen las cuatro.
			nombre: "coma perdida con el 75 % escribiendo la coma final",
			datos:  "titulo,id,taquilla,espectadores,\nA,A-1,1,7,\nB,B-1,2,7,\nC,C-1,3,7,\nD55,100,7,\n",
			quiere: []string{"mezcla filas de 4 y 5 campos (1 y 3 filas)", "mezcla", "mezcla", "mezcla"},
		},
		{
			// K2 de la cuarta auditoria: la MAYORIA pierde la coma. Con mayoria
			// simple entraban B y C corridas y caia A, la buena. Ahora caen las
			// tres: ante la duda, ruido y no silencio.
			nombre: "la mayoria pierde la coma",
			datos:  "titulo,id,taquilla,espectadores,\nA,55,100,7,\nB55,100,7,\nC66,200,8,\n",
			quiere: []string{"mezcla filas de 4 y 5 campos (2 y 1 filas)", "mezcla", "mezcla"},
		},
		{
			// K3: empate entre una buena sin coma y una coma de mas con blanco.
			nombre: "empate entre buena y coma de mas",
			datos:  "titulo,id,taquilla,espectadores,\nA,55,100,7\nRapido, furioso,55,100,\n",
			quiere: []string{"mezcla filas de 4 y 5 campos (1 y 1 filas)", "mezcla"},
		},
		{
			// N10: el numero del motivo es el del ARCHIVO, no el de la cabecera
			// (aqui 3 y no 4).
			nombre: "fila corta en un archivo sin coma final bajo cabecera con coma final",
			datos:  "titulo,id,taquilla,\nA,PX-1,1\nB,PX-2\nC,PX-3,3\n",
			quiere: []string{"", "trae 2 campos y 2 de 3 filas de este archivo traen 3", ""},
		},
		{
			// Por debajo de las columnas con nombre la fila es corta siempre, y
			// no cuenta para la mayoria: el archivo sigue siendo de un ancho.
			nombre: "fila corta en un archivo con coma final",
			datos:  "titulo,id,taquilla,\nA,PX-1,1,\nB,\nC,PX-3,3,\n",
			quiere: []string{"", "trae 2 campos y 2 de 3 filas de este archivo traen 4", ""},
		},
		{
			nombre: "comas parciales en empate",
			datos:  "titulo,id,taquilla,, \nA,PX-1,1,\nB,PX-2,2\n",
			quiere: []string{"mezcla filas de 3 y 4 campos", "mezcla"},
		},
		{
			nombre: "tres anchos legitimos a la vez",
			datos:  "titulo,id,taquilla,,\nA,PX-1,1\nB,PX-2,2,\nC,PX-3,3,,\n",
			quiere: []string{"mezcla filas de 3, 4 y 5 campos (1, 1 y 1 filas)", "mezcla", "mezcla"},
		},
		{
			// N11: el dato sin nombre pisa el motivo de celda, aunque la fila
			// traiga ademas una requerida vacia.
			nombre: "dato sin nombre y requerida vacia en la misma fila",
			datos:  "titulo,,id,taquilla\nA,huerfano,,1\n",
			quiere: []string{"columna 2, que no tiene nombre"},
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
	// En .xlsx se nombra por su letra y no se habla de comas: no hay comas.
	m := usos[1].RechazoMotivo
	if !strings.Contains(m, "columna D, que no tiene encabezado") || strings.Contains(m, "coma") {
		t.Errorf("el dato en la columna sin encabezado no se rechazo con su motivo: %q", m)
	}
}

// J2: en JSON el hueco es una clave vacia, y " " es tan vacia como "": antes
// la primera se rechazaba y la segunda entraba sin motivo.
func TestJSONRechazaElDatoBajoUnaClaveVacia(t *testing.T) {
	t.Parallel()

	for _, clave := range []string{"", " ", `\t`} { // `\t` es el escape JSON del tabulador
		datos := `[{"titulo":"A","id":"PX-1","taquilla":1,"` + clave + `":"x"},{"titulo":"B","id":"PX-2","taquilla":2}]`
		usos, err := lector(t, MapaCine(), aplicacion.FormatoJSON).Leer([]byte(datos))
		if err != nil {
			t.Fatalf("clave %q: %v", clave, err)
		}
		m := usos[0].RechazoMotivo
		if !strings.Contains(m, "clave vacia") || strings.Contains(m, "coma") {
			t.Errorf("clave %q: %q", clave, m)
		}
		if usos[1].RechazoMotivo != "" {
			t.Errorf("clave %q: la fila sin esa clave no deberia rechazarse: %s", clave, usos[1].RechazoMotivo)
		}
	}
}

// El borde del umbral de mayoria (umbralMayoriaAncho = 0.9): con 9 de 10
// filas en un ancho manda la mayoria y cae solo la otra; con 8 de 10 no se
// puede saber y caen las diez.
func TestAplicarUmbralDeMayoriaDeAncho(t *testing.T) {
	t.Parallel()

	archivo := func(sinComa, conComa int) string {
		var b strings.Builder
		b.WriteString("titulo,id,taquilla,\n")
		for i := range sinComa {
			fmt.Fprintf(&b, "S%d,S-%d,1\n", i, i)
		}
		for i := range conComa {
			fmt.Fprintf(&b, "C%d,C-%d,1,\n", i, i)
		}
		return b.String()
	}

	t.Run("9 de 10: manda la mayoria", func(t *testing.T) {
		t.Parallel()
		usos, err := lector(t, MapaCine(), aplicacion.FormatoCSV).Leer([]byte(archivo(9, 1)))
		if err != nil {
			t.Fatalf("Leer: %v", err)
		}
		for i, u := range usos[:9] {
			if u.RechazoMotivo != "" {
				t.Errorf("fila %d de la mayoria rechazada: %s", i, u.RechazoMotivo)
			}
		}
		if m := usos[9].RechazoMotivo; !strings.Contains(m, "9 de 10 filas de este archivo traen 3") {
			t.Errorf("la minoria: %q", m)
		}
	})
	t.Run("8 de 10: caen todas", func(t *testing.T) {
		t.Parallel()
		usos, err := lector(t, MapaCine(), aplicacion.FormatoCSV).Leer([]byte(archivo(8, 2)))
		if err != nil {
			t.Fatalf("Leer: %v", err)
		}
		for i, u := range usos {
			if !strings.Contains(u.RechazoMotivo, "mezcla filas de 3 y 4 campos (8 y 2 filas)") {
				t.Errorf("fila %d: %q", i, u.RechazoMotivo)
			}
		}
	})
}

// K4: en .xlsx una celda con dato mas alla de la cabecera se nombra como la
// ve el cliente, por su referencia de Excel, y no se habla de comas.
func TestAplicarNombraLaCeldaFueraDeLaCabeceraEnXLSX(t *testing.T) {
	t.Parallel()

	datos := xlsxDeCeldas(t, map[string]string{
		"A1": "titulo", "B1": "id", "C1": "taquilla",
		"A2": "A", "B2": "PX-1", "C2": "1", "AW2": "nota",
		"A3": "B", "B3": "PX-2", "C3": "2",
	})
	usos, err := lector(t, MapaCine(), aplicacion.FormatoXLSX).Leer(datos)
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	m := usos[0].RechazoMotivo
	if !strings.Contains(m, "la celda AW2 esta fuera de la cabecera (ultima columna C)") || strings.Contains(m, "coma") {
		t.Errorf("motivo = %q", m)
	}
	if usos[1].RechazoMotivo != "" {
		t.Errorf("fila 3 rechazada: %s", usos[1].RechazoMotivo)
	}
}
