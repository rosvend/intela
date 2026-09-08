package ingesta

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Los archivos REALES que entrego el cliente. Estan versionados en el
// repositorio (`.gitattributes` los marca binarios), asi que estas pruebas no
// necesitan red ni Docker.
//
// El perfil contra el que se comprueban esta en `docs/dominio/fuentes-datos.md`
// y es reproducible con `uv run --script src/scripts/sample.py`.
const (
	rutaCaracol = "../../../data/files/CARACOL_REDES-SGC_(COLOMBIA)_20250202.xlsx"
	rutaNetflix = "../../../data/files/Modulo identificación de Obras - Parrilla Netflix.xlsx"

	filasCaracol = 59
	filasNetflix = 49
)

func leerArchivo(t *testing.T, ruta string) []byte {
	t.Helper()
	datos, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("leer %s: %v", ruta, err)
	}
	return datos
}

func leerFixture(t *testing.T, nombre string) []byte {
	t.Helper()
	return leerArchivo(t, filepath.Join("testdata", nombre))
}

func lector(t *testing.T, m Mapa, formato string) *Lector {
	t.Helper()
	l, err := NuevoLector(m, formato)
	if err != nil {
		t.Fatalf("NuevoLector(%s, %s): %v", m.Fuente, formato, err)
	}
	return l
}

// ---------------------------------------------------------------------------
// Contra los archivos reales del cliente
// ---------------------------------------------------------------------------

func TestCaracolXLSXRealEntraEntero(t *testing.T) {
	t.Parallel()

	usos, err := lector(t, MapaCaracol(), aplicacion.FormatoXLSX).Leer(leerArchivo(t, rutaCaracol))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if len(usos) != filasCaracol {
		t.Fatalf("filas = %d, el perfil dice %d", len(usos), filasCaracol)
	}
	for i, u := range usos {
		if u.RechazoMotivo != "" {
			t.Fatalf("fila %d rechazada en un archivo que el perfil da por limpio: %s",
				i, u.RechazoMotivo)
		}
	}

	// Muestra: la primera fila del archivo real, con los tres campos que el
	// mapa dice que salen de el.
	if usos[0].Titulo != "Rebelde" {
		t.Errorf("titulo[0] = %q", usos[0].Titulo)
	}
	if usos[0].IDsFuente != "55174" {
		t.Errorf("ids_fuente[0] = %q, se esperaba el ID_Ficha", usos[0].IDsFuente)
	}
	if usos[0].DuracionMin.String() != "45" {
		t.Errorf("duracion_min[0] = %s", usos[0].DuracionMin)
	}
	if usos[0].Modalidad != reparto.TV {
		t.Errorf("modalidad[0] = %q", usos[0].Modalidad)
	}

	// La granularidad es la EMISION: 29 ID_Ficha distintos en 59 filas. Si el
	// adaptador colapsara o rechazara las repetidas, este recuento seria 29.
	fichas := map[string]struct{}{}
	for _, u := range usos {
		fichas[u.IDsFuente] = struct{}{}
	}
	if len(fichas) != 29 {
		t.Errorf("ID_Ficha distintos = %d, el perfil dice 29", len(fichas))
	}

	// Lo que el mapa deja fuera a proposito, y por que importa comprobarlo: la
	// correspondencia de genero a las cinco categorias del reglamento es una
	// pregunta abierta al cliente, y escribir `SubGenero` en `tipo_obra` la
	// inventaria.
	for i, u := range usos {
		if u.TipoObra != "" {
			t.Fatalf("fila %d trae tipo_obra %q; la correspondencia todavia no existe", i, u.TipoObra)
		}
	}
}

func TestNetflixXLSXRealEntraEntero(t *testing.T) {
	t.Parallel()

	usos, err := lector(t, MapaNetflix(), aplicacion.FormatoXLSX).Leer(leerArchivo(t, rutaNetflix))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if len(usos) != filasNetflix {
		t.Fatalf("filas = %d, el perfil dice %d", len(usos), filasNetflix)
	}
	for i, u := range usos {
		if u.RechazoMotivo != "" {
			t.Fatalf("fila %d rechazada: %s", i, u.RechazoMotivo)
		}
	}

	if usos[0].Titulo != "My Horrible Boss" {
		t.Errorf("titulo[0] = %q, se esperaba el show_name", usos[0].Titulo)
	}
	// El identificador tiene que llegar como el entero que es. Pasado por un
	// float64 saldria "8.0197856e+07" y no casaria con ningun alias nunca.
	if usos[0].IDsFuente != "80197856" {
		t.Errorf("ids_fuente[0] = %q", usos[0].IDsFuente)
	}
	if usos[0].Vistas.String() != "2172" {
		t.Errorf("vistas[0] = %s, se esperaba el stream_starts", usos[0].Vistas)
	}
	if usos[0].Modalidad != reparto.OTT {
		t.Errorf("modalidad[0] = %q", usos[0].Modalidad)
	}

	// `netflix_id` es de EPISODIO y es unico en las 49 filas; el titulo es el
	// del show y si se repite. Que los dos recuentos difieran es la prueba de
	// que no se confundieron.
	ids := map[string]struct{}{}
	for _, u := range usos {
		ids[u.IDsFuente] = struct{}{}
	}
	if len(ids) != filasNetflix {
		t.Errorf("netflix_id distintos = %d, el perfil dice %d", len(ids), filasNetflix)
	}

	// `minutos_vistos` NO se mapea: `episode_runtime` es la duracion del
	// episodio, no el tiempo visto que pide RD 9.7. Un numero plausible en la
	// columna equivocada no lo distingue nadie aguas abajo.
	for i, u := range usos {
		if !u.MinutosVistos.IsZero() {
			t.Fatalf("fila %d trae minutos_vistos %s; esa magnitud no esta en el archivo",
				i, u.MinutosVistos)
		}
	}
}

func TestElXLSXDeUnaFuenteNoSeLeeConElMapaDeOtra(t *testing.T) {
	t.Parallel()

	// Las dos fuentes no comparten ni un nombre de columna (medido). El mapa
	// cruzado tiene que rechazar el archivo ENTERO nombrando lo que falta, no
	// producir 59 filas vacias.
	_, err := lector(t, MapaNetflix(), aplicacion.FormatoXLSX).Leer(leerArchivo(t, rutaCaracol))
	if !errors.Is(err, aplicacion.ErrReporteInvalido) {
		t.Fatalf("err = %v, se esperaba ErrReporteInvalido", err)
	}
}

// ---------------------------------------------------------------------------
// La misma forma logica en CSV y en JSON
// ---------------------------------------------------------------------------

func TestCaracolCSVSigueElMismoMapaQueSuXLSX(t *testing.T) {
	t.Parallel()

	usos, err := lector(t, MapaCaracol(), aplicacion.FormatoCSV).Leer(leerFixture(t, "caracol.csv"))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if len(usos) != 6 {
		t.Fatalf("filas = %d, se esperaban 6", len(usos))
	}

	// Un lote con filas malas persiste las buenas y anota las malas. Ninguna se
	// pierde: seis entran, seis salen.
	buenas, malas := 0, 0
	for _, u := range usos {
		if u.RechazoMotivo == "" {
			buenas++
		} else {
			malas++
		}
	}
	if buenas != 4 || malas != 2 {
		t.Fatalf("buenas/malas = %d/%d, se esperaba 4/2: %+v", buenas, malas, motivos(usos))
	}
	// La cuarta fila repite la primera emision entera.
	if !strings.Contains(usos[3].RechazoMotivo, "duplicado") {
		t.Errorf("fila 4: %q", usos[3].RechazoMotivo)
	}
	// La quinta trae la duracion en letras.
	if !strings.Contains(usos[4].RechazoMotivo, "duracion_min") {
		t.Errorf("fila 5: %q", usos[4].RechazoMotivo)
	}
	// La segunda es otra emision del mismo programa: entra.
	if usos[1].RechazoMotivo != "" {
		t.Errorf("fila 2 no deberia rechazarse: %q", usos[1].RechazoMotivo)
	}
}

func TestNetflixJSONSigueElMismoMapaQueSuXLSX(t *testing.T) {
	t.Parallel()

	usos, err := lector(t, MapaNetflix(), aplicacion.FormatoJSON).Leer(leerFixture(t, "netflix.json"))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if len(usos) != 4 {
		t.Fatalf("filas = %d, se esperaban 4", len(usos))
	}
	if usos[0].IDsFuente != "80197856" {
		t.Errorf("el netflix_id se estropeo al pasar por JSON: %q", usos[0].IDsFuente)
	}
	if usos[0].Vistas.String() != "2172" {
		t.Errorf("vistas[0] = %s", usos[0].Vistas)
	}
	if usos[1].DuracionMin.String() != "41.2" {
		t.Errorf("duracion_min[1] = %s", usos[1].DuracionMin)
	}
	if !strings.Contains(usos[2].RechazoMotivo, "duplicado") {
		t.Errorf("el tercer registro repite netflix_id: %q", usos[2].RechazoMotivo)
	}
	if !strings.Contains(usos[3].RechazoMotivo, "vistas") {
		t.Errorf("el cuarto trae stream_starts en letras: %q", usos[3].RechazoMotivo)
	}
}

func TestCineCSVEsLaTerceraModalidad(t *testing.T) {
	t.Parallel()

	// La muestra sintetica del repositorio. Sirve para probar el parseo, no
	// para sacar conclusiones: las cifras estan escritas a mano.
	usos, err := lector(t, MapaCine(), aplicacion.FormatoCSV).
		Leer(leerArchivo(t, "../../../data/samples/cine.csv"))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if len(usos) != 2 {
		t.Fatalf("filas = %d, se esperaban 2", len(usos))
	}
	for i, u := range usos {
		if u.RechazoMotivo != "" {
			t.Fatalf("fila %d: %s", i, u.RechazoMotivo)
		}
		if u.Modalidad != reparto.Cine {
			t.Errorf("modalidad[%d] = %q", i, u.Modalidad)
		}
	}
	if usos[0].Taquilla.String() != "10000" {
		t.Errorf("taquilla[0] = %s", usos[0].Taquilla)
	}
}

func motivos(usos []aplicacion.UsoPersistido) []string {
	out := make([]string, 0, len(usos))
	for _, u := range usos {
		if u.RechazoMotivo != "" {
			out = append(out, u.RechazoMotivo)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// El catalogo
// ---------------------------------------------------------------------------

func TestCatalogoDelClienteRegistraLasTresFuentesEnLosTresFormatos(t *testing.T) {
	t.Parallel()

	cat, err := CatalogoDelCliente()
	if err != nil {
		// Un mapa mal escrito es un defecto del programa. Que se descubra al
		// arrancar y no cuando llega una entrega es la razon de que Catalogo
		// devuelva error.
		t.Fatalf("CatalogoDelCliente: %v", err)
	}
	quiere := []string{
		"caracol/csv", "caracol/json", "caracol/xlsx",
		"cine/csv", "cine/json", "cine/xlsx",
		"netflix/csv", "netflix/json", "netflix/xlsx",
	}
	if got := Fuentes(cat); !slices.Equal(got, quiere) {
		t.Fatalf("catalogo = %v, se esperaba %v", got, quiere)
	}
}

func TestCatalogoRechazaDosMapasParaLaMismaFuente(t *testing.T) {
	t.Parallel()

	// Silencioso si se deja pasar: ganaria el ultimo, y las entregas seguirian
	// entrando con las columnas del mapa equivocado.
	if _, err := Catalogo(MapaCaracol(), MapaCaracol()); !errors.Is(err, aplicacion.ErrReporteInvalido) {
		t.Fatalf("err = %v, se esperaba ErrReporteInvalido", err)
	}
}

func TestNuevoLectorRechazaUnFormatoDesconocido(t *testing.T) {
	t.Parallel()

	if _, err := NuevoLector(MapaCaracol(), "xls"); !errors.Is(err, aplicacion.ErrReporteInvalido) {
		t.Fatalf("err = %v, se esperaba ErrReporteInvalido", err)
	}
}

func TestFormatoDeNombre(t *testing.T) {
	t.Parallel()

	casos := map[string]string{
		"parrilla.xlsx":      aplicacion.FormatoXLSX,
		"PARRILLA.XLSX":      aplicacion.FormatoXLSX,
		"macro.xlsm":         aplicacion.FormatoXLSX,
		"reporte.csv":        aplicacion.FormatoCSV,
		"reporte.json":       aplicacion.FormatoJSON,
		"padron.xls":         "", // formato binario viejo: excelize no lo lee
		"sin_extension":      "",
		"reporte.xlsx.zip":   "",
		"archivo.raro..":     "",
		"reportes/enero.csv": aplicacion.FormatoCSV,
	}
	for nombre, quiere := range casos {
		if got := FormatoDeNombre(nombre); got != quiere {
			t.Errorf("FormatoDeNombre(%q) = %q, se esperaba %q", nombre, got, quiere)
		}
	}
}
