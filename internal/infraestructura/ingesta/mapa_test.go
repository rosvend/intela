package ingesta

import (
	"errors"
	"strings"
	"testing"

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
	}{
		{
			nombre:   "cine sin id: la cascada no puede casar ni aprender alias",
			mapa:     MapaCine(),
			columnas: []string{"titulo", "id", "taquilla"},
			fila:     []string{"Pelicula X", "", "100"},
			enMotivo: []string{"fila 2", "ids_fuente", `"id"`, "requerida"},
		},
		{
			nombre:   "cine sin taquilla: no pondera nada y no dejaba rastro",
			mapa:     MapaCine(),
			columnas: []string{"titulo", "id", "taquilla"},
			fila:     []string{"Pelicula X", "PX-1", " "},
			enMotivo: []string{"fila 2", "taquilla", `"taquilla"`, "requerida"},
		},
		{
			nombre:   "placeholder en una columna requerida no es un hueco declarado",
			mapa:     MapaCine(),
			columnas: []string{"titulo", "id", "taquilla"},
			fila:     []string{"Pelicula X", "PX-1", "--"},
			enMotivo: []string{"fila 2", "taquilla", "--"},
		},
		{
			nombre:   "netflix sin show_id: falta el par que sondea la cascada",
			mapa:     MapaNetflix(),
			columnas: []string{"show_name", "show_id", "series_id", "netflix_id", "stream_starts"},
			fila:     []string{"Show", "", "S-1", "N-1", "10"},
			enMotivo: []string{"fila 2", "ids_fuente", `"show_id"`},
		},
		{
			nombre:   "titulo vacio: lo dice el adaptador, con linea y columna",
			mapa:     mapaMinimo(),
			columnas: []string{"titulo", "duracion"},
			fila:     []string{"", "10"},
			enMotivo: []string{"fila 2", "titulo", `"titulo"`},
		},
		{
			nombre:   "titulo con placeholder, que validarUso no ve",
			mapa:     mapaMinimo(),
			columnas: []string{"titulo", "duracion"},
			fila:     []string{"N/A", "10"},
			enMotivo: []string{"fila 2", "titulo", "N/A"},
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
