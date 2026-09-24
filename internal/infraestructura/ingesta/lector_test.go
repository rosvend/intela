package ingesta

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

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
	ids0 := aplicacion.LeerIDsFuente(usos[0].IDsFuente)
	if ids0[aplicacion.ClaveIDFicha] != "55174" {
		t.Errorf("ids_fuente[0] = %q, se esperaba id_ficha=55174", usos[0].IDsFuente)
	}
	if usos[0].DuracionMin.String() != "45" {
		t.Errorf("duracion_min[0] = %s", usos[0].DuracionMin)
	}
	if usos[0].Modalidad != reparto.TV {
		t.Errorf("modalidad[0] = %q", usos[0].Modalidad)
	}

	// El titulo original llega hasta el uso (#32, review de PR #146): el escalon
	// 3 lo prueba ademas del emitido, y el perfil dice que difieren en 16 de 59.
	distintos := 0
	for _, u := range usos {
		if u.TituloOrig != "" && u.TituloOrig != u.Titulo {
			distintos++
		}
	}
	if distintos != 16 {
		t.Errorf("filas con titulo original distinto = %d, el perfil dice 16", distintos)
	}

	// La granularidad es la EMISION: 29 ID_Ficha distintos en 59 filas. Si el
	// adaptador colapsara o rechazara las repetidas, este recuento seria 29.
	fichas := map[string]struct{}{}
	for _, u := range usos {
		fichas[aplicacion.LeerIDsFuente(u.IDsFuente)[aplicacion.ClaveIDFicha]] = struct{}{}
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
	ids0 := aplicacion.LeerIDsFuente(usos[0].IDsFuente)
	if ids0[aplicacion.ClaveNetflixID] != "80197856" {
		t.Errorf("ids_fuente[0] = %q, se esperaba netflix_id=80197856", usos[0].IDsFuente)
	}
	if ids0[aplicacion.ClaveShowID] == "" {
		t.Errorf("ids_fuente[0] no trae show_id: %q", usos[0].IDsFuente)
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
		ids[aplicacion.LeerIDsFuente(u.IDsFuente)[aplicacion.ClaveNetflixID]] = struct{}{}
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
	// 3/3: la sexta fila viene sin titulo, y desde #113 la rechaza el propio
	// adaptador por ser columna requerida, con linea y columna. Antes entraba
	// limpia aqui y solo la paraba validarUso, aguas abajo y sin linea.
	if buenas != 3 || malas != 3 {
		t.Fatalf("buenas/malas = %d/%d, se esperaba 3/3: %+v", buenas, malas, motivos(usos))
	}
	if m := usos[5].RechazoMotivo; !strings.Contains(m, "fila 7") || !strings.Contains(m, `"Titulo"`) {
		t.Errorf("fila 7 sin titulo: %q", m)
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

// El titulo original viaja del archivo al uso, y donde la celda viene vacia el
// uso lo guarda vacio: "la fuente no lo trae" (#32).
func TestCaracolCSVLlevaElTituloOriginalAlUso(t *testing.T) {
	t.Parallel()

	usos, err := lector(t, MapaCaracol(), aplicacion.FormatoCSV).Leer(leerFixture(t, "caracol.csv"))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if usos[2].Titulo != "El diario de Diana" || usos[2].TituloOrig != "Diana's Diary" {
		t.Errorf("fila 3: titulo=%q original=%q", usos[2].Titulo, usos[2].TituloOrig)
	}
	// La fila sin titulo se rechaza igual, y el original vacio no lo cambia.
	if usos[5].TituloOrig != "" {
		t.Errorf("fila 6: el original tenia que quedar vacio, fue %q", usos[5].TituloOrig)
	}
}

// Titulo_original no es requerida: una entrega sin esa columna se lee entera,
// con el original vacio. Exigirla rechazaria un archivo bueno por perder solo
// recall.
func TestCaracolSinTituloOriginalNoSeRechaza(t *testing.T) {
	t.Parallel()

	csv := "Canal,Titulo,TIPO,ID_Ficha,Fecha,Hora,Duracion_total\n" +
		"CARACOL,Rebelde,SE,55174,20241231,0:00,45\n"
	usos, err := lector(t, MapaCaracol(), aplicacion.FormatoCSV).Leer([]byte(csv))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if len(usos) != 1 || usos[0].RechazoMotivo != "" {
		t.Fatalf("la fila tenia que entrar: %+v", motivos(usos))
	}
	if usos[0].Titulo != "Rebelde" || usos[0].TituloOrig != "" {
		t.Errorf("titulo=%q original=%q", usos[0].Titulo, usos[0].TituloOrig)
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
	ids0 := aplicacion.LeerIDsFuente(usos[0].IDsFuente)
	if ids0[aplicacion.ClaveNetflixID] != "80197856" {
		t.Errorf("el netflix_id se estropeo al pasar por JSON: %q", usos[0].IDsFuente)
	}
	if ids0[aplicacion.ClaveShowID] != "80141259" {
		t.Errorf("el show_id no viajo: %q", usos[0].IDsFuente)
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

// El mismo caso de punta a punta, por el Lector: el motivo tiene que mandar
// al cliente a la linea 5, no a la 3.
func TestCineCSVReportaLaLineaDelRechazoTrasLineasEnBlanco(t *testing.T) {
	t.Parallel()

	datos := "titulo,id,taquilla\nBuena,PX-1,1\n\n\nMala,PX-2,no-es-numero\n"
	usos, err := lector(t, MapaCine(), aplicacion.FormatoCSV).Leer([]byte(datos))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if len(usos) != 2 {
		t.Fatalf("usos = %d, se esperaban 2", len(usos))
	}
	if !strings.HasPrefix(usos[1].RechazoMotivo, "fila 5,") {
		t.Fatalf("motivo = %q, se esperaba la linea 5", usos[1].RechazoMotivo)
	}
}

// El docstring de TablaJSON prometia que un valor compuesto en una columna
// mapeada se rechaza NOMBRANDO el campo, y no ocurria: `{"titulo":{"x":1}}`
// persistia titulo = `{"x":1}` e `"id":[1,2]` persistia
// ids_fuente = `id_pelicula=[1,2]`, sin motivo (issue #113, punto 4).
func TestJSONRechazaLosValoresCompuestosNombrandoElCampo(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre   string
		datos    string
		enMotivo []string
	}{
		{
			nombre:   "objeto en el titulo",
			datos:    `[{"titulo":{"x":1},"id":"PX-1","taquilla":1}]`,
			enMotivo: []string{"fila 2", "titulo", `"titulo"`, "compuesto"},
		},
		{
			nombre:   "array en el identificador",
			datos:    `[{"titulo":"Pelicula X","id":[1,2],"taquilla":1}]`,
			enMotivo: []string{"fila 2", "ids_fuente", `"id"`, "compuesto"},
		},
		{
			nombre:   "objeto en la metrica",
			datos:    `[{"titulo":"Pelicula X","id":"PX-1","taquilla":{"valor":1}}]`,
			enMotivo: []string{"fila 2", "taquilla", "compuesto"},
		},
		{
			// Opcional y mapeada: no es requerida, pero se persistiria igual.
			nombre:   "array en una columna opcional mapeada",
			datos:    `[{"titulo":"Pelicula X","id":"PX-1","taquilla":1,"moneda":["COP"]}]`,
			enMotivo: []string{"fila 2", "moneda", `"moneda"`, "compuesto"},
		},
		{
			nombre:   "segundo registro, para la linea",
			datos:    `[{"titulo":"A","id":"A-1","taquilla":1},{"titulo":"B","id":{},"taquilla":1}]`,
			enMotivo: []string{"fila 3", "ids_fuente", "compuesto"},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			usos, err := lector(t, MapaCine(), aplicacion.FormatoJSON).Leer([]byte(c.datos))
			if err != nil {
				t.Fatalf("un valor compuesto es un rechazo de fila: %v", err)
			}
			m := usos[len(usos)-1].RechazoMotivo
			if m == "" {
				t.Fatalf("la fila deberia rechazarse: %+v", usos[len(usos)-1])
			}
			for _, quiere := range c.enMotivo {
				if !strings.Contains(m, quiere) {
					t.Errorf("el motivo no dice %q: %s", quiere, m)
				}
			}
		})
	}
}

// Un compuesto en una columna que el mapa NO usa da igual: no se persiste.
func TestJSONIgnoraLosCompuestosDeColumnasNoMapeadas(t *testing.T) {
	t.Parallel()

	datos := `[{"titulo":"Pelicula X","id":"PX-1","taquilla":1,"extra":{"a":[1]}}]`
	usos, err := lector(t, MapaCine(), aplicacion.FormatoJSON).Leer([]byte(datos))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if usos[0].RechazoMotivo != "" {
		t.Fatalf("una columna no mapeada no rechaza la fila: %s", usos[0].RechazoMotivo)
	}
}

// La parte cruda del compuesto va en el motivo para poder pedirlo, pero
// recortada: un objeto de un export puede traer kilobytes, y el motivo es
// una linea del log de rechazos que alguien lee.
func TestJSONRecortaElCompuestoDentroDelMotivo(t *testing.T) {
	t.Parallel()

	largo := `{"texto":"` + strings.Repeat("x", 5000) + `"}`
	datos := `[{"titulo":` + largo + `,"id":"PX-1","taquilla":1}]`
	usos, err := lector(t, MapaCine(), aplicacion.FormatoJSON).Leer([]byte(datos))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	m := usos[0].RechazoMotivo
	if !strings.Contains(m, "compuesto") || !strings.Contains(m, `{"texto":"xxx`) {
		t.Fatalf("el motivo no nombra el compuesto: %.200s", m)
	}
	if len(m) > 400 {
		t.Errorf("motivo de %d bytes: el crudo no se recorto", len(m))
	}

	// El recorte es por RUNAS. `["x` son 3 bytes y cada `ñ` son 2: cortar por
	// bytes en el 80 parte una `ñ` por la mitad y deja UTF-8 invalido en el
	// log de rechazos, que Postgres rechaza en una columna TEXT.
	datos = `[{"titulo":"Pelicula X","id":"PX-1","taquilla":1,"moneda":["x` +
		strings.Repeat("ñ", 100) + `"]}]`
	usos, err = lector(t, MapaCine(), aplicacion.FormatoJSON).Leer([]byte(datos))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if m := usos[0].RechazoMotivo; !utf8.ValidString(m) || !strings.Contains(m, "ñ...") {
		t.Errorf("el recorte partio una runa: %q", m)
	}
}

// Los dos archivos reales, exportados a CSV de las formas en que Excel y una
// persona lo hacen. Todas entran enteras menos la mezclada, en la que solo
// cae la fila que no escribe como las demas (J1 de la tercera auditoria:
// antes caian TODAS las otras, con un motivo que decia un ancho falso).
func TestLosArchivosRealesEnCSVEntranEnterosEnTodasSusFormas(t *testing.T) {
	t.Parallel()

	reales := []struct {
		nombre string
		ruta   string
		mapa   Mapa
		filas  int
	}{
		{"caracol", rutaCaracol, MapaCaracol(), filasCaracol},
		{"netflix", rutaNetflix, MapaNetflix(), filasNetflix},
	}
	for _, r := range reales {
		for _, forma := range []string{"tal-cual", "coma-final", "cabecera-con-coma", "mezcla"} {
			t.Run(r.nombre+"/"+forma, func(t *testing.T) {
				t.Parallel()
				datos := csvDeXLSX(t, r.ruta, forma)
				usos, err := lector(t, r.mapa, aplicacion.FormatoCSV).Leer(datos)
				if err != nil {
					t.Fatalf("Leer: %v", err)
				}
				if len(usos) != r.filas {
					t.Fatalf("usos = %d, se esperaban %d", len(usos), r.filas)
				}
				for i, u := range usos {
					if forma == "mezcla" && u.Linea == 8 {
						quiere := fmt.Sprintf("%d de %d filas de este archivo traen", r.filas-1, r.filas)
						if !strings.Contains(u.RechazoMotivo, quiere) {
							t.Errorf("la fila minoritaria: %q, se esperaba %q", u.RechazoMotivo, quiere)
						}
						continue
					}
					if u.RechazoMotivo != "" {
						t.Fatalf("fila %d (linea %d) rechazada: %s", i, u.Linea, u.RechazoMotivo)
					}
				}
			})
		}
	}
}

// csvDeXLSX exporta la primera hoja de un .xlsx a CSV de una de cuatro formas:
// tal cual; con la coma final de la columna sin nombre que declara el rango
// usado en TODAS las filas; solo en la cabecera; o en la cabecera y en UNA
// fila de datos (la linea 8).
func csvDeXLSX(t *testing.T, ruta, forma string) []byte {
	t.Helper()
	libro, err := excelize.OpenFile(ruta)
	if err != nil {
		t.Fatalf("abrir %s: %v", ruta, err)
	}
	defer func() { _ = libro.Close() }()
	filas, err := libro.GetRows(libro.GetSheetList()[0])
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	ancho := 0
	for _, f := range filas {
		ancho = max(ancho, len(f))
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	for i, f := range filas {
		conComa := forma == "coma-final" ||
			(forma == "cabecera-con-coma" && i == 0) ||
			(forma == "mezcla" && (i == 0 || i == 7))
		if conComa {
			for len(f) < ancho+1 {
				f = append(f, "")
			}
		}
		if err := w.Write(f); err != nil {
			t.Fatalf("csv: %v", err)
		}
	}
	w.Flush()
	return buf.Bytes()
}
