package ingesta

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// mapaMinimo es un mapa valido al que cada caso le rompe una cosa.
func mapaMinimo() Mapa {
	return Mapa{
		Fuente:    "prueba",
		Modalidad: reparto.TV,
		Columnas: []Columna{
			{Campo: CampoTitulo, Nombre: "titulo", Requerida: true},
			{Campo: CampoDuracionMin, Nombre: "duracion"},
		},
	}
}

func TestValidarRechazaUnMapaMalEscritoAlConstruirlo(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		mapa   Mapa
	}{
		{"sin fuente", Mapa{Modalidad: reparto.TV, Columnas: mapaMinimo().Columnas}},
		{"sin columnas", Mapa{Fuente: "x", Modalidad: reparto.TV}},
		{"sin titulo", Mapa{
			Fuente: "x", Modalidad: reparto.TV,
			Columnas: []Columna{{Campo: CampoVistas, Nombre: "v"}},
		}},
		{"sin modalidad ni columna que la traiga", Mapa{
			Fuente:   "x",
			Columnas: []Columna{{Campo: CampoTitulo, Nombre: "t"}},
		}},
		{"campo canonico inventado", Mapa{
			Fuente: "x", Modalidad: reparto.TV,
			Columnas: []Columna{
				{Campo: CampoTitulo, Nombre: "t"},
				{Campo: Campo("importe"), Nombre: "plata"},
			},
		}},
		{"dos columnas al mismo campo", Mapa{
			Fuente: "x", Modalidad: reparto.TV,
			Columnas: []Columna{
				{Campo: CampoTitulo, Nombre: "t"},
				{Campo: CampoTitulo, Nombre: "titulo_original"},
			},
		}},
		{"columna de origen repetida", Mapa{
			Fuente: "x", Modalidad: reparto.TV,
			Columnas: []Columna{
				{Campo: CampoTitulo, Nombre: "name"},
				{Campo: CampoVistas, Nombre: "name"},
			},
		}},
		{"ids_fuente sin clave del contrato", Mapa{
			Fuente: "x", Modalidad: reparto.TV,
			Columnas: []Columna{
				{Campo: CampoTitulo, Nombre: "t"},
				{Campo: CampoIDsFuente, Nombre: "ID_Ficha"},
			},
		}},
		{"clave de ids_fuente desconocida", Mapa{
			Fuente: "x", Modalidad: reparto.TV,
			Columnas: []Columna{
				{Campo: CampoTitulo, Nombre: "t"},
				{Campo: CampoIDsFuente, Nombre: "ID_Ficha", ClaveID: "ID_Ficha"},
			},
		}},
		{"clave de ids_fuente repetida", Mapa{
			Fuente: "x", Modalidad: reparto.TV,
			Columnas: []Columna{
				{Campo: CampoTitulo, Nombre: "t"},
				{Campo: CampoIDsFuente, Nombre: "a", ClaveID: aplicacion.ClaveIDFicha},
				{Campo: CampoIDsFuente, Nombre: "b", ClaveID: aplicacion.ClaveIDFicha},
			},
		}},
		{"clave de ids_fuente en un campo que no lo es", Mapa{
			Fuente: "x", Modalidad: reparto.TV,
			Columnas: []Columna{
				{Campo: CampoTitulo, Nombre: "t", ClaveID: aplicacion.ClaveIDFicha},
			},
		}},
		{"columna sin nombre", Mapa{
			Fuente: "x", Modalidad: reparto.TV,
			Columnas: []Columna{{Campo: CampoTitulo, Nombre: "  "}},
		}},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			if err := c.mapa.Validar(); !errors.Is(err, aplicacion.ErrReporteInvalido) {
				t.Fatalf("err = %v, se esperaba ErrReporteInvalido", err)
			}
		})
	}

	if err := mapaMinimo().Validar(); err != nil {
		// Sin este caso, una Validar() que devolviera error siempre pasaria
		// los de arriba.
		t.Fatalf("el mapa minimo deberia ser valido: %v", err)
	}
}

func TestValidarAceptaVariosIDsFuenteConClavesDistintas(t *testing.T) {
	t.Parallel()

	m := Mapa{
		Fuente: "netflix", Modalidad: reparto.OTT,
		Columnas: []Columna{
			{Campo: CampoTitulo, Nombre: "show_name"},
			{Campo: CampoIDsFuente, Nombre: "show_id", ClaveID: aplicacion.ClaveShowID},
			{Campo: CampoIDsFuente, Nombre: "series_id", ClaveID: aplicacion.ClaveSeriesID},
			{Campo: CampoIDsFuente, Nombre: "netflix_id", ClaveID: aplicacion.ClaveNetflixID},
		},
	}
	if err := m.Validar(); err != nil {
		t.Fatalf("varios ids con claves distintas deberian ser validos: %v", err)
	}
}

func TestAplicarNombraTODASLasColumnasRequeridasQueFaltan(t *testing.T) {
	t.Parallel()

	m := Mapa{
		Fuente: "prueba", Modalidad: reparto.TV,
		Columnas: []Columna{
			{Campo: CampoTitulo, Nombre: "Titulo", Requerida: true},
			{Campo: CampoIDsFuente, Nombre: "ID_Ficha", Requerida: true, ClaveID: aplicacion.ClaveIDFicha},
			{Campo: CampoDuracionMin, Nombre: "Duracion_total", Requerida: true},
		},
	}
	_, err := m.Aplicar(Tabla{Columnas: []string{"Titulo"}, Filas: [][]string{{"Rebelde"}}})
	if !errors.Is(err, aplicacion.ErrReporteInvalido) {
		t.Fatalf("err = %v, se esperaba ErrReporteInvalido", err)
	}
	// De una en una hacen falta tantas rondas con el cliente como columnas
	// falten. El criterio de aceptacion pide el mensaje que nombra la columna;
	// nombrarlas todas es lo que lo hace util.
	for _, quiere := range []string{"ID_Ficha", "Duracion_total"} {
		if !strings.Contains(err.Error(), quiere) {
			t.Errorf("el error no nombra %q: %v", quiere, err)
		}
	}
	// Y la cabecera real, que es como se descubre que la columna esta con otro
	// nombre.
	if !strings.Contains(err.Error(), "El archivo trae: Titulo") {
		t.Errorf("el error no muestra la cabecera del archivo: %v", err)
	}
}

func TestAplicarAbortaSiFaltaUnaColumnaDeLaClaveDeRegistro(t *testing.T) {
	t.Parallel()

	// No es "requerida" en el sentido de Columna -- no alimenta ningun campo
	// canonico -- y aun asi tiene que abortar: sin ella la deteccion de
	// duplicados por registro deja de funcionar EN SILENCIO, y los repetidos
	// ponderarian dos veces.
	m := mapaMinimo()
	m.ClaveRegistro = []string{"titulo", "Fecha"}
	_, err := m.Aplicar(Tabla{Columnas: []string{"titulo"}, Filas: [][]string{{"Rebelde"}}})
	if !errors.Is(err, aplicacion.ErrReporteInvalido) {
		t.Fatalf("err = %v, se esperaba ErrReporteInvalido", err)
	}
	if !strings.Contains(err.Error(), "Fecha") {
		t.Errorf("el error no nombra la columna que falta: %v", err)
	}
}

func TestAplicarRechazaLaFilaNombrandoElCampoYLaColumna(t *testing.T) {
	t.Parallel()

	m := Mapa{
		Fuente: "prueba", Modalidad: reparto.TV,
		Columnas: []Columna{
			{Campo: CampoTitulo, Nombre: "titulo", Requerida: true},
			{Campo: CampoDuracionMin, Nombre: "Duracion_total"},
			{Campo: CampoEmisiones, Nombre: "emisiones"},
		},
	}

	casos := []struct {
		nombre   string
		duracion string
		emis     string
		enMotivo []string
	}{
		{
			nombre: "texto donde va un numero",
			// El caso del `1.234,56` de un export colombiano: no se adivina, se
			// rechaza nombrando el valor para poder pedir el formato bueno.
			duracion: "cuarenta y cinco",
			enMotivo: []string{"duracion_min", "Duracion_total", "cuarenta y cinco"},
		},
		{
			nombre:   "separador de miles a la europea",
			duracion: "1.234,56",
			enMotivo: []string{"duracion_min", "1.234,56"},
		},
		{
			nombre:   "recuento fraccionario",
			emis:     "2.5",
			enMotivo: []string{"emisiones", "2.5"},
		},
		{
			nombre:   "recuento que no es numero",
			emis:     "muchas",
			enMotivo: []string{"emisiones", "muchas"},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			usos, err := m.Aplicar(Tabla{
				Columnas: []string{"titulo", "Duracion_total", "emisiones"},
				Filas:    [][]string{{"Rebelde", c.duracion, c.emis}},
			})
			if err != nil {
				// Una celda mala es un rechazo DE FILA. Que aborte la entrega
				// haria inservible la ingesta: los archivos reales vienen asi.
				t.Fatalf("una celda mala no puede abortar la entrega: %v", err)
			}
			if len(usos) != 1 {
				t.Fatalf("usos = %d, se esperaba 1: la fila mala no se descarta", len(usos))
			}
			motivo := usos[0].RechazoMotivo
			if motivo == "" {
				t.Fatal("la fila deberia venir con motivo de rechazo")
			}
			for _, quiere := range c.enMotivo {
				if !strings.Contains(motivo, quiere) {
					t.Errorf("el motivo no dice %q: %s", quiere, motivo)
				}
			}
			// El log de rechazos guarda lo identificatorio para poder pedirle al
			// cliente la linea exacta: una fila rechazada sin titulo no sirve
			// de nada.
			if usos[0].Titulo != "Rebelde" {
				t.Errorf("la fila rechazada perdio el titulo: %+v", usos[0])
			}
		})
	}
}

func TestAplicarNumeraLasFilasComoLasVeElCliente(t *testing.T) {
	t.Parallel()

	m := mapaMinimo()
	usos, err := m.Aplicar(Tabla{
		Columnas: []string{"titulo", "duracion"},
		Filas:    [][]string{{"buena", "10"}, {"mala", "x"}},
	})
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	// La cabecera es la fila 1 de la hoja de calculo, asi que el segundo
	// registro es la fila 3. Un motivo que diga "fila 1" obliga a traducirlo, y
	// es la clase de detalle que se traduce mal.
	if !strings.Contains(usos[1].RechazoMotivo, "fila 3") {
		t.Errorf("el motivo no numera la fila como la hoja: %s", usos[1].RechazoMotivo)
	}
}

// La numeracion tiene que ser la del ARCHIVO, no la de la lista que llega a
// Aplicar: el lector de formato descarta las filas enteras en blanco -- relleno
// del export, no registros -- y cada una de ellas corre la posicion de todo lo
// que viene detras.
//
// Con la numeracion sacada de la posicion, la fila mala de la linea 5 se
// reportaba como "fila 3": dos blancos delante, dos lineas de menos. El motivo
// existe para pedirle al cliente LA LINEA EXACTA que hay que arreglar, asi que
// un numero corrido lo manda a mirar una fila que esta bien -- o a la que se
// salto justamente por venir vacia.
//
// Va por TablaCSV y no por una Tabla escrita a mano a proposito: el descarte de
// blancos es lo que produce el desfase, y una tabla compuesta a mano no lo
// tiene.
func TestAplicarNumeraLaFilaDelArchivoYNoLaPosicionTrasDescartarBlancos(t *testing.T) {
	t.Parallel()

	// linea 1: cabecera
	// linea 2: buena
	// linea 3: en blanco -- Excel escribe las filas vacias de su rango asi
	// linea 4: en blanco
	// linea 5: mala
	tabla, err := TablaCSV([]byte("titulo,duracion\nbuena,10\n,\n,\nmala,x\n"))
	if err != nil {
		t.Fatalf("TablaCSV: %v", err)
	}
	// El mecanismo, antes del mensaje: las dos filas que sobreviven vienen de las
	// lineas 2 y 5, y esa correspondencia solo se puede anotar donde se descarta.
	if len(tabla.Filas) != 2 || tabla.Linea(0) != 2 || tabla.Linea(1) != 5 {
		t.Fatalf("lineas = %v con %d filas, se esperaban [2 5]", tabla.Lineas, len(tabla.Filas))
	}

	usos, err := mapaMinimo().Aplicar(tabla)
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if len(usos) != 2 {
		t.Fatalf("usos = %d, se esperaban 2", len(usos))
	}
	if usos[0].RechazoMotivo != "" {
		t.Fatalf("la primera fila esta bien: %s", usos[0].RechazoMotivo)
	}
	if !strings.Contains(usos[1].RechazoMotivo, "fila 5") {
		t.Errorf("el motivo no cita la linea del archivo: %s", usos[1].RechazoMotivo)
	}
	// Y no la posicion en la lista ya filtrada, que es el numero equivocado que
	// se leia antes.
	if strings.Contains(usos[1].RechazoMotivo, "fila 3") {
		t.Errorf("el motivo numera sobre la lista filtrada: %s", usos[1].RechazoMotivo)
	}
}

func TestAplicarAceptaLosPlaceholdersComoHuecoDeclarado(t *testing.T) {
	t.Parallel()

	m := mapaMinimo()
	for _, v := range []string{"", "--", "N/A", "  ", "null"} {
		usos, err := m.Aplicar(Tabla{
			Columnas: []string{"titulo", "duracion"},
			Filas:    [][]string{{"Rebelde", v}},
		})
		if err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		if usos[0].RechazoMotivo != "" {
			// `--` esta medido en `episode_nbr` de Netflix. Un hueco declarado
			// no es un dato roto.
			t.Errorf("%q no deberia rechazar la fila: %s", v, usos[0].RechazoMotivo)
		}
		if !usos[0].DuracionMin.IsZero() {
			t.Errorf("%q deberia dejar la medida en cero, hay %s", v, usos[0].DuracionMin)
		}
	}
}

func TestAplicarAceptaUnEnteroEscritoComoDecimalExacto(t *testing.T) {
	t.Parallel()

	// `2172` desde una hoja de calculo y `2172.0` desde un JSON exportado por
	// pandas son el mismo dato; rechazar el segundo seria rechazarlo por el
	// formato del archivo.
	m := Mapa{
		Fuente: "prueba", Modalidad: reparto.TV,
		Columnas: []Columna{
			{Campo: CampoTitulo, Nombre: "titulo", Requerida: true},
			{Campo: CampoEmisiones, Nombre: "emisiones"},
		},
	}
	usos, err := m.Aplicar(Tabla{
		Columnas: []string{"titulo", "emisiones"},
		Filas:    [][]string{{"Rebelde", "4.0"}},
	})
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if usos[0].RechazoMotivo != "" {
		t.Fatalf("4.0 deberia entrar como 4: %s", usos[0].RechazoMotivo)
	}
	if usos[0].Emisiones != 4 {
		t.Errorf("emisiones = %d, se esperaba 4", usos[0].Emisiones)
	}
}

func TestAplicarDetectaElRegistroRepetidoSinConfundirDosEmisiones(t *testing.T) {
	t.Parallel()

	// La trampa de la parrilla: la granularidad es la EMISION. Con `ID_Ficha`
	// sola como clave, 30 de las 59 filas del archivo real de Caracol acabarian
	// en el log de rechazos siendo emisiones legitimas.
	m := Mapa{
		Fuente: "caracol", Modalidad: reparto.TV,
		Columnas:      []Columna{{Campo: CampoTitulo, Nombre: "Titulo", Requerida: true}},
		ClaveRegistro: []string{"ID_Ficha", "Fecha", "Hora"},
	}
	usos, err := m.Aplicar(Tabla{
		Columnas: []string{"Titulo", "ID_Ficha", "Fecha", "Hora"},
		Filas: [][]string{
			{"Rebelde", "55174", "20241231", "0:00"},
			{"Rebelde", "55174", "20241231", "12:00"}, // otra emision: entra
			{"Rebelde", "55174", "20241231", "0:00"},  // la misma: se rechaza
		},
	})
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if usos[0].RechazoMotivo != "" || usos[1].RechazoMotivo != "" {
		t.Fatalf("dos emisiones del mismo programa no son un duplicado: %q / %q",
			usos[0].RechazoMotivo, usos[1].RechazoMotivo)
	}
	if usos[2].RechazoMotivo == "" {
		t.Fatal("la tercera fila es la primera repetida y deberia rechazarse")
	}
	// El motivo dice con QUE fila choca y por que valores: sin eso, quien lo
	// lee tiene que buscar la repeticion a mano en un archivo de 59 filas.
	for _, quiere := range []string{"fila 2", "ID_Ficha=55174", "Hora=0:00"} {
		if !strings.Contains(usos[2].RechazoMotivo, quiere) {
			t.Errorf("el motivo no dice %q: %s", quiere, usos[2].RechazoMotivo)
		}
	}
}

// Columna.Requerida se aplica POR FILA (issue #113, punto 2): su docstring lo
// prometia y solo se comprobaba la cabecera. Una celda vacia -- o con un
// placeholder, que en una columna requerida es lo mismo -- en una columna
// requerida es un rechazo de fila con linea y columna, no un uso que entra
// sin identificador o con la metrica en cero sin dejar rastro.
//
// Antes esta prueba afirmaba lo contrario para el titulo ("lo decide
// validarUso"). Se invierte a proposito: validarUso solo ve el blanco, no el
// `--` ni el `N/A`, y su motivo no dice ni la linea ni la columna. Sigue
// estando detras como red para las filas que no pasan por un Mapa (el seed).
func TestAplicarRechazaLaCeldaVaciaDeUnaColumnaRequerida(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre   string
		mapa     Mapa
		columnas []string
		fila     []string
		enMotivo []string
		// noEnMotivo: una celda vacia no se describe como placeholder (ni con
		// un `("")` que no dice nada), y un placeholder no como celda vacia.
		noEnMotivo []string
	}{
		{
			nombre:     "cine sin id: la cascada no puede casar ni aprender alias",
			mapa:       MapaCine(),
			columnas:   []string{"titulo", "id", "taquilla"},
			fila:       []string{"Pelicula X", "", "100"},
			enMotivo:   []string{"fila 2", "ids_fuente", `"id"`, "requerida", "la celda viene vacia"},
			noEnMotivo: []string{"placeholder", `("")`},
		},
		{
			nombre:     "cine sin taquilla: no pondera nada y no dejaba rastro",
			mapa:       MapaCine(),
			columnas:   []string{"titulo", "id", "taquilla"},
			fila:       []string{"Pelicula X", "PX-1", " "},
			enMotivo:   []string{"fila 2", "taquilla", `"taquilla"`, "requerida", "la celda viene vacia"},
			noEnMotivo: []string{"placeholder", `("")`},
		},
		{
			nombre:     "placeholder en una columna requerida no es un hueco declarado",
			mapa:       MapaCine(),
			columnas:   []string{"titulo", "id", "taquilla"},
			fila:       []string{"Pelicula X", "PX-1", "--"},
			enMotivo:   []string{"fila 2", "taquilla", `la celda trae el placeholder "--"`},
			noEnMotivo: []string{"vacia"},
		},
		{
			nombre:   "netflix sin show_id: falta el par que sondea la cascada",
			mapa:     MapaNetflix(),
			columnas: []string{"show_name", "show_id", "series_id", "netflix_id", "stream_starts"},
			fila:     []string{"Show", "", "S-1", "N-1", "10"},
			enMotivo: []string{"fila 2", "ids_fuente", `"show_id"`},
		},
		{
			nombre:     "titulo vacio: lo dice el adaptador, con linea y columna",
			mapa:       mapaMinimo(),
			columnas:   []string{"titulo", "duracion"},
			fila:       []string{"", "10"},
			enMotivo:   []string{"fila 2", "titulo", `"titulo"`, "la celda viene vacia"},
			noEnMotivo: []string{"placeholder"},
		},
		{
			nombre:     "titulo con placeholder, que validarUso no ve",
			mapa:       mapaMinimo(),
			columnas:   []string{"titulo", "duracion"},
			fila:       []string{"N/A", "10"},
			enMotivo:   []string{"fila 2", "titulo", `la celda trae el placeholder "N/A"`},
			noEnMotivo: []string{"vacia"},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			usos, err := c.mapa.Aplicar(Tabla{Columnas: c.columnas, Filas: [][]string{c.fila}})
			if err != nil {
				t.Fatalf("una celda vacia es un rechazo de fila, no de entrega: %v", err)
			}
			if len(usos) != 1 {
				t.Fatalf("usos = %d, la fila no se descarta", len(usos))
			}
			motivo := usos[0].RechazoMotivo
			if motivo == "" {
				t.Fatal("la fila deberia venir rechazada con motivo")
			}
			for _, quiere := range c.enMotivo {
				if !strings.Contains(motivo, quiere) {
					t.Errorf("el motivo no dice %q: %s", quiere, motivo)
				}
			}
			for _, sobra := range c.noEnMotivo {
				if strings.Contains(motivo, sobra) {
					t.Errorf("el motivo no deberia decir %q: %s", sobra, motivo)
				}
			}
		})
	}
}

// La otra mitad: en una columna OPCIONAL el placeholder sigue siendo un hueco
// declarado (Caracol sin `Programa ID_IMDB`, Netflix sin `episode_runtime`).
func TestAplicarAceptaLaCeldaVaciaDeUnaColumnaOpcional(t *testing.T) {
	t.Parallel()

	usos, err := MapaCine().Aplicar(Tabla{
		Columnas: []string{"titulo", "id", "taquilla", "espectadores", "moneda"},
		Filas:    [][]string{{"Pelicula X", "PX-1", "100", "--", ""}},
	})
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if usos[0].RechazoMotivo != "" {
		t.Fatalf("una columna opcional vacia no rechaza la fila: %s", usos[0].RechazoMotivo)
	}
}

func TestAplicarDejaQueLaFilaDigaSuModalidad(t *testing.T) {
	t.Parallel()

	m := MapaCine()
	usos, err := m.Aplicar(Tabla{
		Columnas: []string{"titulo", "id", "modalidad", "taquilla"},
		Filas: [][]string{
			{"Pelicula X", "PX-1", "", "10000"},
			{"Serie Y", "SY-1", "HOTEL", "0"},
		},
	})
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	// Celda vacia: manda la modalidad fija del mapa.
	if usos[0].Modalidad != reparto.Cine {
		t.Errorf("modalidad[0] = %q, se esperaba %q", usos[0].Modalidad, reparto.Cine)
	}
	// Celda con valor: manda la fila, y en minusculas, que es el vocabulario
	// del CHECK del esquema. Sin la columna mapeada, esta fila entraria
	// declarada como cine sin que nadie lo notara.
	if usos[1].Modalidad != reparto.Hotel {
		t.Errorf("modalidad[1] = %q, se esperaba %q", usos[1].Modalidad, reparto.Hotel)
	}
}

// decimal.Decimal.IntPart() trunca fuera del rango de int64 sin avisar, asi
// que un recuento enorme entraba como otro numero (issue #113, punto 6).
// Latente -- ningun mapa usa emisiones hoy --, pero entra en cuanto uno lo haga.
func TestAEnteroRechazaLoQueNoCabeEnInt64(t *testing.T) {
	t.Parallel()

	for _, v := range []string{
		"9223372036854775808",   // MaxInt64 + 1: ParseInt falla con ErrRange
		"-9223372036854775809",  // MinInt64 - 1
		"9223372036854775808.0", // igual, escrito como decimal exacto
		"1e19",
		"1e30",
	} {
		n, err := aEntero(v)
		if err == nil {
			t.Errorf("aEntero(%q) = %d sin error; no cabe en int64", v, n)
			continue
		}
		if !strings.Contains(err.Error(), v) {
			t.Errorf("aEntero(%q): el error no nombra el valor: %v", v, err)
		}
		// El rango entero, no solo el maximo: para un negativo que se pasa
		// por abajo, "maximo 9223372036854775807" no explica nada.
		if rango := "entre -9223372036854775808 y 9223372036854775807"; !strings.Contains(err.Error(), rango) {
			t.Errorf("aEntero(%q): el error no dice el rango %q: %v", v, rango, err)
		}
	}
	// Los bordes si caben.
	for v, quiere := range map[string]int64{
		"9223372036854775807":    9223372036854775807,
		"-9223372036854775808":   -9223372036854775808,
		"9223372036854775807.0":  9223372036854775807,
		"-9223372036854775808.0": -9223372036854775808,
		"1e3":                    1000,
	} {
		n, err := aEntero(v)
		if err != nil || n != quiere {
			t.Errorf("aEntero(%q) = %d, %v; se esperaba %d", v, n, err, quiere)
		}
	}
}

// Un exponente enorme en notacion cientifica cuesta en proporcion a el, no
// al largo de la celda: `1e9999999` son 9 bytes y reescalarlo para Truncate,
// GreaterThan o IntPart tardaba segundos, y mas con cada cifra. Se rechaza
// antes de tocarlo, nombrando el valor, en las dos coerciones numericas.
//
// El motivo habla del valor tal como lo escribio el cliente y no del
// exponente interno de la libreria: `5.55e-17` se guarda como 555...e-32, y
// un "-32" en el motivo no esta en ninguna celda.
func TestLaCoercionNumericaRechazaUnExponenteFueraDeRango(t *testing.T) {
	t.Parallel()

	coerciones := map[string]func(string) error{
		"aEntero":  func(v string) error { _, err := aEntero(v); return err },
		"aDecimal": func(v string) error { _, err := aDecimal(v); return err },
	}
	for nombre, coercion := range coerciones {
		for v, quiere := range map[string]string{
			"1e9999999":  "pasa de 10^30",
			"-1e9999999": "pasa de 10^30",
			"1e31":       "pasa de 10^30",
			"1e-9999999": "mas de 400 cifras decimales",
			"1e-401":     "mas de 400 cifras decimales",
		} {
			inicio := time.Now()
			err := coercion(v)
			if dur := time.Since(inicio); dur > 100*time.Millisecond {
				t.Errorf("%s(%q) tardo %v; el exponente tiene que cortarse antes de reescalar", nombre, v, dur)
			}
			if err == nil {
				t.Errorf("%s(%q) sin error; el valor no cabe en ninguna columna de medida", nombre, v)
				continue
			}
			m := err.Error()
			if !strings.Contains(m, v) || !strings.Contains(m, "fuera de escala") || !strings.Contains(m, quiere) {
				t.Errorf("%s(%q): el error no nombra el valor ni dice %q: %v", nombre, v, quiere, err)
			}
			if strings.Contains(m, "exponente") {
				t.Errorf("%s(%q): el motivo cita el exponente interno: %v", nombre, v, err)
			}
		}
		// Un cero es cero con cualquier exponente, y se devuelve sin
		// reescalar: nada de lo que viene detras toca el exponente enorme.
		for _, v := range []string{"0e9999999", "0e-9999999"} {
			inicio := time.Now()
			if err := coercion(v); err != nil {
				t.Errorf("%s(%q): %v; un cero con exponente es cero", nombre, v, err)
			}
			if dur := time.Since(inicio); dur > 100*time.Millisecond {
				t.Errorf("%s(%q) tardo %v", nombre, v, dur)
			}
		}
	}
	// El borde positivo si entra. Y por abajo, cualquier literal de float64:
	// un exportador escribe `5.551115123125783e-17` por el ruido de coma
	// flotante, y validarUso lo redondea a la escala de la columna a
	// proposito. Tambien un decimal escrito a mano con muchas cifras.
	for _, v := range []string{
		"1e30", "1e-30", "0.30000000000000004", "2172.0",
		"5.551115123125783e-17", "4.440892098500626e-16",
		"1.7976931348623157e-308", "4.9406564584124654e-324",
		"0.000000000000000000000000000000001",
	} {
		d, err := aDecimal(v)
		if err != nil {
			t.Errorf("aDecimal(%q): %v; esta dentro del rango", v, err)
			continue
		}
		if !d.Equal(decimal.RequireFromString(v)) {
			t.Errorf("aDecimal(%q) = %s; cambio el valor", v, d)
		}
	}
	if d, err := aDecimal("0e9999999"); err != nil || !d.IsZero() || d.Exponent() != decimal.Zero.Exponent() {
		t.Errorf("aDecimal(%q) = %s (exponente %d), %v; se esperaba decimal.Zero, sin el exponente enorme", "0e9999999", d, d.Exponent(), err)
	}
	if n, err := aEntero("2e3"); err != nil || n != 2000 {
		t.Errorf("aEntero(%q) = %d, %v; se esperaba 2000", "2e3", n, err)
	}
}

// El mapa estampa la linea en el uso para que aplicacion pueda numerar sus
// propios motivos (issue #113, punto 3), y el motivo de duplicado dice su
// PROPIA linea ademas de la de la fila con la que choca.
func TestAplicarEstampaLaLineaEnElUsoYEnElMotivoDeDuplicado(t *testing.T) {
	t.Parallel()

	tabla, err := TablaCSV([]byte("titulo,id,taquilla\nA,PX-1,1\n\nA,PX-1,1\n"))
	if err != nil {
		t.Fatalf("TablaCSV: %v", err)
	}
	usos, err := MapaCine().Aplicar(tabla)
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if usos[0].Linea != 2 || usos[1].Linea != 4 {
		t.Fatalf("lineas = %d, %d; se esperaban 2, 4", usos[0].Linea, usos[1].Linea)
	}
	if m := usos[1].RechazoMotivo; !strings.HasPrefix(m, "fila 4: registro duplicado") || !strings.Contains(m, "fila 2") {
		t.Errorf("motivo de duplicado: %q", m)
	}
}

func TestLetraColumnaComoLaEscribeExcel(t *testing.T) {
	t.Parallel()

	for n, quiere := range map[int]string{1: "A", 4: "D", 26: "Z", 27: "AA", 49: "AW", 702: "ZZ", 703: "AAA"} {
		if got := letraColumna(n); got != quiere {
			t.Errorf("letraColumna(%d) = %q, se esperaba %q", n, got, quiere)
		}
	}
}

// La simetrica de TestAplicarRechazaLaFilaConCamposDeMas (issue #113, punto 5). Una coma PERDIDA corre
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
func TestAplicarAceptaLasColumnasFinalesSinNombreDeLaCabecera(t *testing.T) {
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

// En .xlsx, igual: la cabecera de Caracol trae una columna final vacia.
func TestAplicarAceptaLaColumnaFinalSinNombreEnXLSXYRechazaSuDato(t *testing.T) {
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
	// Una fila fuera de los anchos legitimos -- mas ancha que la cabecera o
	// mas corta que las columnas con nombre -- ya cae por su propio motivo, y
	// no puede votar: contarla en el total bajaba 20 de 22 (91 %) a 20 de 23
	// (87 %), y UNA fila mala tumbaba el archivo entero con un motivo de
	// "mezcla" que citaba una mayoria por encima del umbral.
	for nombre, intrusa := range map[string]string{
		"mas ancha que la cabecera":         "W,W-1,1,,x\n",
		"mas corta que las columnas nombre": "Z,Z-1\n",
	} {
		t.Run("20 de 22 y una fila "+nombre+": manda la mayoria", func(t *testing.T) {
			t.Parallel()
			usos, err := lector(t, MapaCine(), aplicacion.FormatoCSV).Leer([]byte(archivo(2, 20) + intrusa))
			if err != nil {
				t.Fatalf("Leer: %v", err)
			}
			if len(usos) != 23 {
				t.Fatalf("usos = %d, se esperaban 23", len(usos))
			}
			for i, u := range usos[:2] {
				if !strings.Contains(u.RechazoMotivo, "trae 3 campos y 20 de 23 filas de este archivo traen 4") {
					t.Errorf("fila %d de la minoria: %q", i, u.RechazoMotivo)
				}
			}
			for i, u := range usos[2:22] {
				if u.RechazoMotivo != "" {
					t.Errorf("fila %d de la mayoria rechazada: %s", i+2, u.RechazoMotivo)
				}
			}
			if m := usos[22].RechazoMotivo; m == "" || strings.Contains(m, "mezcla") {
				t.Errorf("la intrusa tiene que caer por su ancho, no por mezcla: %q", m)
			}
		})
	}
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

// Un registro de solo blancos (`,,,`) se descarta, y su ancho no puede
// anotarse: `anchos` es paralela a `Filas`, y un ancho de mas desalinearia el
// de TODAS las filas de detras. Con el desfase, A (corta) heredaria el ancho
// del blanco y entraria, y B (justa) heredaria el de A y caeria.
func TestAplicarNoDesalineaLosAnchosAlDescartarUnRegistroEnBlanco(t *testing.T) {
	t.Parallel()

	datos := "titulo,id,taquilla,espectadores\n,,,\nA,PX-1,5\nB,PX-2,6,7\n"
	usos, err := lector(t, MapaCine(), aplicacion.FormatoCSV).Leer([]byte(datos))
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if len(usos) != 2 {
		t.Fatalf("usos = %d, se esperaban 2 (el blanco no cuenta)", len(usos))
	}
	if m := usos[0].RechazoMotivo; usos[0].Titulo != "A" || !strings.Contains(m, "trae 3 campos") {
		t.Errorf("A deberia rechazarse por corta: %q", m)
	}
	if usos[1].Titulo != "B" || usos[1].RechazoMotivo != "" {
		t.Errorf("B deberia entrar: %q", usos[1].RechazoMotivo)
	}
}
