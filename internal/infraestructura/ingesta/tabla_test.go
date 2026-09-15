package ingesta

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

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
	if len(tabla.Filas[0]) != len(tabla.Columnas) {
		t.Errorf("la fila corta deberia rellenarse: %q", tabla.Filas[0])
	}
	if tabla.Filas[0][2] != "" {
		t.Errorf("la celda que faltaba deberia ser vacia, es %q", tabla.Filas[0][2])
	}
	if len(tabla.Filas[1]) != 4 || tabla.Filas[1][3] != "7" {
		t.Errorf("la celda sobrante no se puede recortar en silencio: %q", tabla.Filas[1])
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

func TestTablaXLSXConservaLaFilaFisicaTrasUnHueco(t *testing.T) {
	t.Parallel()

	// linea 1: cabecera
	// linea 2: buena
	// linea 3: vacia, omitida del XML
	// linea 4: mala
	datos := xlsxDeCeldas(t, map[string]string{
		"A1": "titulo", "B1": "duracion",
		"A2": "buena", "B2": "10",
		"A4": "mala", "B4": "x",
	})
	tabla, err := TablaXLSX(datos, "")
	if err != nil {
		t.Fatalf("TablaXLSX: %v", err)
	}
	if len(tabla.Filas) != 2 || tabla.Linea(0) != 2 || tabla.Linea(1) != 4 {
		t.Fatalf("lineas = %v con %d filas, se esperaban [2 4]", tabla.Lineas, len(tabla.Filas))
	}

	usos, err := mapaMinimo().Aplicar(tabla)
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if !strings.Contains(usos[1].RechazoMotivo, "fila 4") {
		t.Errorf("el motivo no cita la linea fisica: %s", usos[1].RechazoMotivo)
	}
	if strings.Contains(usos[1].RechazoMotivo, "fila 3") {
		t.Errorf("el motivo numera sobre la lista compactada: %s", usos[1].RechazoMotivo)
	}
}

func TestTablaXLSXRechazaUnaFilaFueraDelTopeDeExcel(t *testing.T) {
	t.Parallel()

	datos := xlsxDeCeldas(t, map[string]string{"A1": "titulo", "A2": "obra"})
	datos = parcheFilaXML(t, datos, `<row r="2"`, `<row r="1048577"`)

	_, err := TablaXLSX(datos, "")
	if !errors.Is(err, aplicacion.ErrReporteInvalido) {
		t.Fatalf("err = %v, se esperaba ErrReporteInvalido", err)
	}
	if !strings.Contains(err.Error(), "1048577") {
		t.Errorf("el error no nombra la fila fuera de rango: %v", err)
	}
}

func xlsxDeCeldas(t *testing.T, celdas map[string]string) []byte {
	t.Helper()
	libro := excelize.NewFile()
	t.Cleanup(func() { _ = libro.Close() })
	for celda, valor := range celdas {
		if err := libro.SetCellValue("Sheet1", celda, valor); err != nil {
			t.Fatalf("SetCellValue(%s): %v", celda, err)
		}
	}
	var buf bytes.Buffer
	if err := libro.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes()
}

func parcheFilaXML(t *testing.T, datos []byte, viejo, nuevo string) []byte {
	t.Helper()
	origen, err := zip.NewReader(bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	var salida bytes.Buffer
	dest := zip.NewWriter(&salida)
	for _, f := range origen.File {
		w, err := dest.Create(f.Name)
		if err != nil {
			t.Fatalf("Create(%s): %v", f.Name, err)
		}
		r, err := f.Open()
		if err != nil {
			t.Fatalf("Open(%s): %v", f.Name, err)
		}
		cuerpo, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatalf("ReadAll(%s): %v", f.Name, err)
		}
		if strings.HasPrefix(f.Name, "xl/worksheets/") && strings.HasSuffix(f.Name, ".xml") {
			cuerpo = []byte(strings.Replace(string(cuerpo), viejo, nuevo, 1))
		}
		if _, err := w.Write(cuerpo); err != nil {
			t.Fatalf("Write(%s): %v", f.Name, err)
		}
	}
	if err := dest.Close(); err != nil {
		t.Fatalf("cerrar zip: %v", err)
	}
	return salida.Bytes()
}
