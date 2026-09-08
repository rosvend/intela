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
		// los siete de arriba.
		t.Fatalf("el mapa minimo deberia ser valido: %v", err)
	}
}

func TestAplicarNombraTODASLasColumnasRequeridasQueFaltan(t *testing.T) {
	t.Parallel()

	m := Mapa{
		Fuente: "prueba", Modalidad: reparto.TV,
		Columnas: []Columna{
			{Campo: CampoTitulo, Nombre: "Titulo", Requerida: true},
			{Campo: CampoIDsFuente, Nombre: "ID_Ficha", Requerida: true},
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

func TestAplicarNoInventaMotivoParaLoQueValidaElNucleo(t *testing.T) {
	t.Parallel()

	// El titulo vacio SI es un rechazo, pero lo decide `validarUso` en
	// aplicacion, una sola vez y para todas las fuentes. Repetir aqui la regla
	// daria dos criterios para el mismo campo, que es como acaban discrepando.
	usos, err := mapaMinimo().Aplicar(Tabla{
		Columnas: []string{"titulo", "duracion"},
		Filas:    [][]string{{"", "10"}},
	})
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}
	if usos[0].RechazoMotivo != "" {
		t.Errorf("el adaptador no decide sobre el titulo: %s", usos[0].RechazoMotivo)
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
