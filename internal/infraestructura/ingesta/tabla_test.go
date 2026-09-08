package ingesta

import (
	"errors"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Los lectores de formato se prueban SIN mapa: lo unico que tienen que hacer
// es entregar la cabecera y las filas tal como estaban, y la unica forma de
// comprobarlo es mirarlas antes de que nadie las interprete.

func TestTablaCSVQuitaElBOMDeExcel(t *testing.T) {
	t.Parallel()

	tabla, err := TablaCSV([]byte("\ufefftitulo,id\nPelicula X,PX-1\n"))
	if err != nil {
		t.Fatalf("TablaCSV: %v", err)
	}
	if tabla.Columnas[0] != "titulo" {
		// Con el BOM pegado, la columna se llama "\ufeff"+"titulo" y el archivo se
		// rechaza por falta de una columna que el cliente ve escrita.
		t.Fatalf("primera columna = %q, se esperaba %q", tabla.Columnas[0], "titulo")
	}
}

func TestTablaCSVCuadraElAnchoDeLasFilas(t *testing.T) {
	t.Parallel()

	// Fila corta, fila larga y fila en blanco: las tres formas en las que un
	// CSV real se sale del ancho de su cabecera.
	tabla, err := TablaCSV([]byte("a,b,c\n1,2\n\n4,5,6,7\n"))
	if err != nil {
		t.Fatalf("TablaCSV: %v", err)
	}
	if len(tabla.Filas) != 2 {
		t.Fatalf("filas = %d, se esperaban 2 (la vacia no cuenta): %q", len(tabla.Filas), tabla.Filas)
	}
	for i, f := range tabla.Filas {
		if len(f) != len(tabla.Columnas) {
			t.Errorf("fila %d tiene %d celdas, la cabecera %d: %q", i, len(f), len(tabla.Columnas), f)
		}
	}
	if tabla.Filas[0][2] != "" {
		t.Errorf("la celda que faltaba deberia ser vacia, es %q", tabla.Filas[0][2])
	}
	if tabla.Filas[1][2] != "6" {
		t.Errorf("la celda sobrante deberia recortarse dejando %q, hay %q", "6", tabla.Filas[1][2])
	}
}

func TestTablaCSVSinCabecera(t *testing.T) {
	t.Parallel()

	_, err := TablaCSV(nil)
	if !errors.Is(err, aplicacion.ErrReporteInvalido) {
		t.Fatalf("err = %v, se esperaba ErrReporteInvalido", err)
	}
}

func TestTablaJSONConservaElLiteralDelNumero(t *testing.T) {
	t.Parallel()

	// 80197856 es un netflix_id real. Decodificado a float64 y vuelto a texto
	// sale "8.0197856e+07", que ya no casa con ningun alias: es exactamente
	// como se estropea un identificador al pasar por un JSON.
	tabla, err := TablaJSON([]byte(`[{"netflix_id": 80197856, "vacio": null, "si": true}]`))
	if err != nil {
		t.Fatalf("TablaJSON: %v", err)
	}
	fila := tabla.Filas[0]
	celdas := map[string]string{}
	for i, c := range tabla.Columnas {
		celdas[c] = fila[i]
	}
	if celdas["netflix_id"] != "80197856" {
		t.Errorf("netflix_id = %q, se esperaba %q", celdas["netflix_id"], "80197856")
	}
	if celdas["vacio"] != "" {
		t.Errorf("null deberia ser celda vacia, es %q", celdas["vacio"])
	}
	if celdas["si"] != "true" {
		t.Errorf("true = %q", celdas["si"])
	}
}

func TestTablaJSONUneLasClavesDeTodosLosRegistrosYLasOrdena(t *testing.T) {
	t.Parallel()

	// La columna `b` solo existe en el segundo registro. Sacando la cabecera
	// del primero se perderia, y con ella una columna posiblemente requerida.
	tabla, err := TablaJSON([]byte(`[{"c": 1, "a": 2}, {"b": 3}]`))
	if err != nil {
		t.Fatalf("TablaJSON: %v", err)
	}
	if got := strings.Join(tabla.Columnas, ","); got != "a,b,c" {
		t.Fatalf("columnas = %q, se esperaba %q", got, "a,b,c")
	}
}

func TestTablaJSONRechazaContenidoPegadoAlFinal(t *testing.T) {
	t.Parallel()

	// Dos exports concatenados. Sin la comprobacion se carga el primero y se
	// dice que todo fue bien, que es la peor de las respuestas posibles.
	_, err := TablaJSON([]byte(`[{"a": 1}] [{"a": 2}]`))
	if !errors.Is(err, aplicacion.ErrReporteInvalido) {
		t.Fatalf("err = %v, se esperaba ErrReporteInvalido", err)
	}
}

func TestTablaJSONRechazaLoQueNoEsUnArrayDeObjetos(t *testing.T) {
	t.Parallel()

	for _, crudo := range []string{`{"a": 1}`, `[1, 2, 3]`, `no es json`} {
		if _, err := TablaJSON([]byte(crudo)); !errors.Is(err, aplicacion.ErrReporteInvalido) {
			t.Errorf("%q: err = %v, se esperaba ErrReporteInvalido", crudo, err)
		}
	}
}

func TestTablaXLSXRechazaBytesQueNoSonUnLibro(t *testing.T) {
	t.Parallel()

	// Un CSV renombrado a .xlsx. Tiene que fallar DICIENDOLO, no colarse: el
	// formato se deduce del nombre del fichero, que no es de fiar.
	_, err := TablaXLSX([]byte("titulo,id\nPelicula X,PX-1\n"), "")
	if !errors.Is(err, aplicacion.ErrReporteInvalido) {
		t.Fatalf("err = %v, se esperaba ErrReporteInvalido", err)
	}
}

func TestTablaXLSXNombraLasHojasQueSiHay(t *testing.T) {
	t.Parallel()

	datos := leerArchivo(t, rutaCaracol)
	_, err := TablaXLSX(datos, "Hoja que no existe")
	if !errors.Is(err, aplicacion.ErrReporteInvalido) {
		t.Fatalf("err = %v, se esperaba ErrReporteInvalido", err)
	}
	// El nombre real esta truncado a 31 caracteres por el propio Excel; sin
	// verlo en el error nadie adivina como se llama.
	if !strings.Contains(err.Error(), "CARACOL_REDES-SGC_(COLOMBIA)_20") {
		t.Errorf("el error no lista las hojas del libro: %v", err)
	}
}
