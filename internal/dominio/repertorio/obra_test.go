package repertorio

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// valida devuelve unos metadatos que SI construyen una obra. Cada prueba de
// abajo rompe uno solo, para que el fallo apunte al campo y no a la suma.
func valida() Metadatos {
	return Metadatos{
		Titulo: "La Casa de las Dos Palmas",
		Genero: "Drama",
		Anio:   1991,
		Tipo:   TipoSerie,
		IDA:    "IDA-1",
		Coautores: []Coautor{
			{Nombre: "Ana Escritora", IPI: "IPI-00000001", Rol: RolGuionista},
		},
	}
}

// El criterio del issue: la construccion rechaza que falte un campo
// obligatorio. Los cuatro del catalogo -titulo, genero, anio y el IPI, que
// entra por el coautor- mas el tipo, que el esquema tiene NOT NULL con CHECK.
func TestNuevaObraRechazaLoQueFalta(t *testing.T) {
	casos := []struct {
		nombre string
		id     string
		romper func(*Metadatos)
	}{
		{"sin identificador", "", nil},
		{"identificador en blanco", "   ", nil},
		{"sin titulo", "obra-1", func(m *Metadatos) { m.Titulo = "" }},
		{"titulo en blanco", "obra-1", func(m *Metadatos) { m.Titulo = "  \t " }},
		{"sin genero", "obra-1", func(m *Metadatos) { m.Genero = "" }},
		{"sin anio", "obra-1", func(m *Metadatos) { m.Anio = 0 }},
		{"anio negativo", "obra-1", func(m *Metadatos) { m.Anio = -1991 }},
		{"sin tipo", "obra-1", func(m *Metadatos) { m.Tipo = "" }},
		{"tipo que no esta en el reglamento", "obra-1", func(m *Metadatos) { m.Tipo = "documental" }},
		{"sin coautores", "obra-1", func(m *Metadatos) { m.Coautores = nil }},
		{"lista de coautores vacia", "obra-1", func(m *Metadatos) { m.Coautores = []Coautor{} }},
		{"coautor sin nombre", "obra-1", func(m *Metadatos) { m.Coautores[0].Nombre = "" }},
		{"coautor sin IPI", "obra-1", func(m *Metadatos) { m.Coautores[0].IPI = "" }},
		{"coautor con IPI en blanco", "obra-1", func(m *Metadatos) { m.Coautores[0].IPI = " " }},
		{"coautor sin rol", "obra-1", func(m *Metadatos) { m.Coautores[0].Rol = "" }},
		// RD 7.3.3 deja fuera por su nombre a directores, actores, revisores
		// y ejecutivos: el conjunto cerrado de roles es lo que impide que
		// entren al catalogo como coautores.
		{"coautor con un rol que no genera derecho", "obra-1", func(m *Metadatos) {
			m.Coautores[0].Rol = "director"
		}},
		{"el mismo IPI dos veces en el mismo rol", "obra-1", func(m *Metadatos) {
			m.Coautores = append(m.Coautores, m.Coautores[0])
		}},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			m := valida()
			if c.romper != nil {
				c.romper(&m)
			}

			_, err := NuevaObra(c.id, m)
			if !errors.Is(err, ErrObraInvalida) {
				t.Fatalf("se esperaba ErrObraInvalida, se obtuvo %v", err)
			}
		})
	}
}

func TestNuevaObraAceptaLoCompleto(t *testing.T) {
	o, err := NuevaObra("obra-1", valida())
	if err != nil {
		t.Fatalf("NuevaObra: %v", err)
	}
	if o.ID() != "obra-1" {
		t.Fatalf("ID() = %q", o.ID())
	}
	if m := o.Metadatos(); m.Titulo != "La Casa de las Dos Palmas" || m.Genero != "Drama" || m.Anio != 1991 {
		t.Fatalf("metadatos mal guardados: %+v", m)
	}
}

// El criterio de aceptacion del issue: dos o mas coautores con roles
// autorales distintos. Y la misma persona puede figurar en dos roles: es su
// obra, la escribio y la adapto.
func TestUnaObraAdmiteVariosCoautoresConRolesDistintos(t *testing.T) {
	m := valida()
	m.Coautores = []Coautor{
		{Nombre: "Ana Escritora", IPI: "IPI-00000001", Rol: RolGuionista},
		{Nombre: "Beto Libretista", IPI: "IPI-00000002", Rol: RolLibretista},
		{Nombre: "Ana Escritora", IPI: "IPI-00000001", Rol: RolAdaptador},
	}

	o, err := NuevaObra("obra-1", m)
	if err != nil {
		t.Fatalf("tres coautores con pares (IPI, rol) distintos son validos: %v", err)
	}

	roles := map[RolAutoral]int{}
	for _, c := range o.Coautores() {
		roles[c.Rol]++
	}
	if len(roles) != 3 {
		t.Fatalf("se esperaban 3 roles autorales distintos, hay %d: %v", len(roles), roles)
	}
}

// El identificador es inmutable y lo impone el tipo: no hay setter, y la
// unica operacion que cambia algo -ConMetadatos- devuelve una obra con el
// MISMO id. Cambiarlo reasignaria en silencio los autores y el dinero de una
// obra a otra.
func TestElIdentificadorSobreviveALaCorreccionDeMetadatos(t *testing.T) {
	o, err := NuevaObra("obra-1", valida())
	if err != nil {
		t.Fatalf("NuevaObra: %v", err)
	}

	corregida, err := o.ConMetadatos(Metadatos{
		Titulo:    "Otro titulo",
		Genero:    "Comedia",
		Anio:      2001,
		Tipo:      TipoTelenovela,
		Coautores: []Coautor{{Nombre: "Beto", IPI: "IPI-2", Rol: RolLibretista}},
	})
	if err != nil {
		t.Fatalf("ConMetadatos: %v", err)
	}

	if corregida.ID() != "obra-1" {
		t.Fatalf("ID() = %q tras corregir, se esperaba \"obra-1\"", corregida.ID())
	}
	if corregida.Metadatos().Titulo != "Otro titulo" {
		t.Fatal("ConMetadatos no aplico los metadatos nuevos")
	}
	// La original no se toca: ConMetadatos devuelve otra obra, no muta.
	if o.Metadatos().Titulo != "La Casa de las Dos Palmas" {
		t.Fatal("ConMetadatos muto el receptor")
	}
}

// Que no exista un setter no se puede comprobar en tiempo de ejecucion; lo que
// si se comprueba es lo que lo sostiene: el campo esta sin exportar, asi que
// ningun paquete de fuera puede escribirlo.
func TestElCampoDelIdentificadorNoEstaExportado(t *testing.T) {
	tipo := reflect.TypeOf(Obra{})
	for i := range tipo.NumField() {
		if campo := tipo.Field(i); campo.IsExported() {
			t.Fatalf("Obra.%s esta exportado: un campo publico es un setter sin nombre", campo.Name)
		}
	}
}

// Devolver la rebanada interna dejaria reescribir el rol de un coautor DENTRO
// de la obra, sin pasar por ninguna validacion.
func TestLosCoautoresSalenClonados(t *testing.T) {
	o, err := NuevaObra("obra-1", valida())
	if err != nil {
		t.Fatalf("NuevaObra: %v", err)
	}

	fuera := o.Coautores()
	fuera[0].Rol = "director"
	fuera[0].IPI = ""

	if dentro := o.Coautores()[0]; dentro.Rol != RolGuionista || dentro.IPI != "IPI-00000001" {
		t.Fatalf("la obra se dejo modificar desde fuera: %+v", dentro)
	}
}

// La entrada tambien se clona: si no, quien construyo la obra conserva un
// puntero a sus coautores y puede vaciarlos despues de la validacion.
func TestLaObraNoComparteLaRebanadaDeEntrada(t *testing.T) {
	m := valida()
	o, err := NuevaObra("obra-1", m)
	if err != nil {
		t.Fatalf("NuevaObra: %v", err)
	}

	m.Coautores[0].IPI = ""

	if o.Coautores()[0].IPI != "IPI-00000001" {
		t.Fatal("la obra comparte la rebanada que le pasaron")
	}
}

func TestNuevaObraRecortaLosEspacios(t *testing.T) {
	m := valida()
	m.Titulo = "  La Casa de las Dos Palmas  "
	m.Genero = " Drama "
	m.Coautores[0].Nombre = " Ana Escritora "
	m.Coautores[0].IPI = " IPI-00000001 "

	o, err := NuevaObra("  obra-1  ", m)
	if err != nil {
		t.Fatalf("NuevaObra: %v", err)
	}
	if o.ID() != "obra-1" {
		t.Fatalf("ID() = %q", o.ID())
	}
	if got := o.Metadatos().Titulo; got != "La Casa de las Dos Palmas" {
		t.Fatalf("Titulo = %q", got)
	}
	if got := o.Coautores()[0].IPI; got != "IPI-00000001" {
		t.Fatalf("IPI = %q", got)
	}
}

// ConMetadatos revalida con el mismo constructor: una obra corregida cumple lo
// mismo que una recien creada, o no existe.
func TestConMetadatosRechazaLoInvalido(t *testing.T) {
	o, err := NuevaObra("obra-1", valida())
	if err != nil {
		t.Fatalf("NuevaObra: %v", err)
	}

	sinCoautores := valida()
	sinCoautores.Coautores = nil

	if _, err := o.ConMetadatos(sinCoautores); !errors.Is(err, ErrObraInvalida) {
		t.Fatalf("se esperaba ErrObraInvalida, se obtuvo %v", err)
	}
}

// El catalogo es identidad y metadata. Los porcentajes salen SOLO de la
// Declaracion de Obra (`R-02`, `R-03`): si alguien anade un campo de dinero a
// Coautor o a Metadatos, existirian dos caminos hasta un pago y el segundo no
// lo firma ninguna Declaracion. Esta prueba se pone roja el dia que pase.
func TestElCatalogoNoTieneDondeGuardarDinero(t *testing.T) {
	prohibidas := []string{"porcentaje", "importe", "monto", "valor", "pago", "bruto", "neto"}

	for _, tipo := range []reflect.Type{reflect.TypeOf(Coautor{}), reflect.TypeOf(Metadatos{})} {
		for i := range tipo.NumField() {
			campo := tipo.Field(i)
			nombre := strings.ToLower(campo.Name)
			for _, prohibida := range prohibidas {
				if strings.Contains(nombre, prohibida) {
					t.Fatalf("%s.%s parece un campo de dinero: el catalogo no reparte (R-02, R-03)",
						tipo.Name(), campo.Name)
				}
			}
			if strings.Contains(campo.Type.String(), "decimal") {
				t.Fatalf("%s.%s es un decimal: el unico decimal del repertorio es el porcentaje de una Parte",
					tipo.Name(), campo.Name)
			}
		}
	}
}
