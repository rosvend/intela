package postgres

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

// El fixture compartido de semilla_test.go solo trae personas NATURALES -las
// dos que necesitan las pruebas de declaraciones-, y sin una persona juridica
// en el padron no se puede probar ni el filtro `persona_natural` ni la regla
// R-01 de punta a punta. Se anade aqui, en la prueba que la necesita, y no en
// semilla_test.go: una productora no le hace falta a nadie mas.
//
// Las dos juridicas van con IPI y SIN IPI a proposito:
//
//   - La primera lleva IPI para que el filtro por persona natural no pueda
//     pasar por un filtro por "IPI vacio".
//   - La segunda lo lleva vacio porque el CHECK `titular_natural_tiene_ipi`
//     solo obliga a las personas naturales: si la lectura del padron fuera mas
//     estricta que el esquema, esta fila -legitima- no se podria leer y el
//     editor de reparto se quedaria sin padron.
const (
	titularProductora = "tit-productora"
	titularCadena     = "tit-cadena"
)

func sembrarPersonasJuridicas(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO titulares (id, nombre, ipi, persona_natural, clase, email) VALUES
		   ($1, 'Productora del Caribe S.A.S.', 'IPI-00000077', FALSE, 'administrado', 'contacto@productora.co'),
		   ($2, 'Cadena del Norte S.A.',         '',             FALSE, 'administrado', '')`,
		titularProductora, titularCadena); err != nil {
		t.Fatalf("sembrar las personas juridicas: %v", err)
	}
}

func idsTitulares(titulares []afiliacion.Titular) []string {
	out := make([]string, 0, len(titulares))
	for _, tit := range titulares {
		out = append(out, tit.ID())
	}
	return out
}

func padronCompleto(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()

	s, pool := sembrar(t)
	sembrarPersonasJuridicas(t, pool)
	return s, pool
}

// ---------------------------------------------------------------------------
// Lectura del padron

// Sin filtro devuelve el padron con las dos clases de titular dentro y cada
// fila reconstruida por el constructor del dominio, no como columnas sueltas.
func TestBuscarTitularesDevuelveElPadronOrdenado(t *testing.T) {
	s, _ := padronCompleto(t)

	titulares, err := s.BuscarTitulares(t.Context(), aplicacion.FiltroTitulares{})
	if err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	quiero := []string{titularAna, titularBeto, titularCadena, titularProductora}
	if got := idsTitulares(titulares); !slices.Equal(got, quiero) {
		t.Fatalf("ids = %v, se esperaba %v", got, quiero)
	}

	ana := titulares[0]
	if ana.Nombre() != "Ana Escritora" || ana.IPI() != "IPI-00000001" ||
		!ana.PersonaNatural() || ana.Clase() != afiliacion.ClaseSocio {
		t.Fatalf("la socia se leyo mal: %+v", ana)
	}
	// La cadena no tiene IPI y aun asi es una entrada del padron: el esquema
	// no exige IPI a quien no es persona natural.
	cadena := titulares[2]
	if cadena.IPI() != "" || cadena.PersonaNatural() || cadena.Clase() != afiliacion.ClaseAdministrado {
		t.Fatalf("la persona juridica sin IPI se leyo mal: %+v", cadena)
	}
}

// Por el padron se busca sin saber el nombre exacto: "escritora" tiene que
// encontrar a Ana Escritora, y en cualquier caja.
func TestBuscarTitularesPorNombreParcialSinDistinguirMayusculas(t *testing.T) {
	s, _ := padronCompleto(t)

	casos := map[string][]string{
		"escritora":  {titularAna},
		"ESCRITORA":  {titularAna},
		"del caribe": {titularProductora},
		"Escritor":   {titularAna},
		"libretista": {titularBeto},
	}
	for nombre, quiero := range casos {
		t.Run(nombre, func(t *testing.T) {
			titulares, err := s.BuscarTitulares(t.Context(), aplicacion.FiltroTitulares{Nombre: nombre})
			if err != nil {
				t.Fatalf("BuscarTitulares: %v", err)
			}
			if got := idsTitulares(titulares); !slices.Equal(got, quiero) {
				t.Fatalf("ids = %v, se esperaba %v", got, quiero)
			}
		})
	}
}

func TestBuscarTitularesPorIPIExacto(t *testing.T) {
	s, _ := padronCompleto(t)

	titulares, err := s.BuscarTitulares(t.Context(), aplicacion.FiltroTitulares{IPI: "IPI-00000002"})
	if err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	if got := idsTitulares(titulares); !slices.Equal(got, []string{titularBeto}) {
		t.Fatalf("ids = %v, se esperaba [%s]", got, titularBeto)
	}

	// Exacto y no parcial: un trozo de IPI no es un IPI.
	titulares, err = s.BuscarTitulares(t.Context(), aplicacion.FiltroTitulares{IPI: "IPI-0000000"})
	if err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	if len(titulares) != 0 {
		t.Fatalf("un trozo de IPI encontro %v", idsTitulares(titulares))
	}
}

// El filtro que hace falta para explicar R-01: quien esta en el padron y NO
// puede recibir reparto. La productora lleva IPI, asi que lo que la separa no
// es el IPI sino la persona natural.
func TestBuscarTitularesFiltraPorPersonaNatural(t *testing.T) {
	s, _ := padronCompleto(t)
	si, no := true, false

	casos := []struct {
		nombre string
		filtro *bool
		quiero []string
	}{
		{"solo personas naturales", &si, []string{titularAna, titularBeto}},
		{"solo personas juridicas", &no, []string{titularCadena, titularProductora}},
		{"sin filtro", nil, []string{titularAna, titularBeto, titularCadena, titularProductora}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			titulares, err := s.BuscarTitulares(t.Context(), aplicacion.FiltroTitulares{PersonaNatural: c.filtro})
			if err != nil {
				t.Fatalf("BuscarTitulares: %v", err)
			}
			if got := idsTitulares(titulares); !slices.Equal(got, c.quiero) {
				t.Fatalf("ids = %v, se esperaba %v", got, c.quiero)
			}
		})
	}
}

// Y la regla R-01 leida de la base, no de un doble: la productora esta en el
// padron y su titular dice que no puede recibir reparto.
func TestElTitularLeidoDelPadronSabeSiPuedeRecibirReparto(t *testing.T) {
	s, _ := padronCompleto(t)
	no := false

	titulares, err := s.BuscarTitulares(t.Context(), aplicacion.FiltroTitulares{PersonaNatural: &no})
	if err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	if len(titulares) != 2 {
		t.Fatalf("se esperaban las 2 personas juridicas, llegaron %d", len(titulares))
	}
	for _, tit := range titulares {
		if tit.PuedeRecibirReparto() {
			t.Fatalf("%s (%s) puede recibir reparto: R-01 no se esta aplicando", tit.ID(), tit.Nombre())
		}
	}
}

// Los filtros se combinan con Y, no con O: con O, un nombre que no cuadra
// devolveria titulares de todas formas y el buscador mentiria.
func TestBuscarTitularesCombinaLosFiltrosConY(t *testing.T) {
	s, _ := padronCompleto(t)
	si := true

	casos := []struct {
		nombre string
		filtro aplicacion.FiltroTitulares
		quiero []string
	}{
		{
			"nombre y persona natural",
			aplicacion.FiltroTitulares{Nombre: "escritora", PersonaNatural: &si},
			[]string{titularAna},
		},
		{
			"nombre y persona natural que no cuadran",
			aplicacion.FiltroTitulares{Nombre: "caribe", PersonaNatural: &si},
			nil,
		},
		{
			"nombre e IPI",
			aplicacion.FiltroTitulares{Nombre: "escritora", IPI: "IPI-00000002"},
			nil,
		},
		{
			"IPI y persona natural",
			aplicacion.FiltroTitulares{IPI: "IPI-00000077", PersonaNatural: &si},
			nil,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			titulares, err := s.BuscarTitulares(t.Context(), c.filtro)
			if err != nil {
				t.Fatalf("BuscarTitulares: %v", err)
			}
			if got := idsTitulares(titulares); !slices.Equal(got, c.quiero) {
				t.Fatalf("ids = %v, se esperaba %v", got, c.quiero)
			}
		})
	}
}

func TestBuscarTitularesRespetaLimiteYDesplazamiento(t *testing.T) {
	s, _ := padronCompleto(t)

	titulares, err := s.BuscarTitulares(t.Context(), aplicacion.FiltroTitulares{
		Paginacion: aplicacion.Paginacion{Limite: 2, Desplazamiento: 1},
	})
	if err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	quiero := []string{titularBeto, titularCadena}
	if got := idsTitulares(titulares); !slices.Equal(got, quiero) {
		t.Fatalf("ids = %v, se esperaba %v", got, quiero)
	}
}

// LimiteSinTope es el centinela de "el padron entero" que comparte con el
// catalogo. El adaptador tiene que traducirlo a NO poner LIMIT: mandarle -1 a
// Postgres es un error de sintaxis, no una pagina de todo.
func TestBuscarTitularesConLimiteSinTope(t *testing.T) {
	s, _ := padronCompleto(t)

	titulares, err := s.BuscarTitulares(t.Context(), aplicacion.FiltroTitulares{
		Paginacion: aplicacion.Paginacion{Limite: aplicacion.LimiteSinTope},
	})
	if err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	if len(titulares) != 4 {
		t.Fatalf("se esperaba el padron entero (4), llegaron %d", len(titulares))
	}
}

// Una busqueda sin coincidencias devuelve lista vacia y NINGUN error: "no hay
// filas" no es un fallo cuando lo que se pidio fue un listado.
func TestBuscarTitularesSinCoincidenciasNoEsError(t *testing.T) {
	s, _ := padronCompleto(t)

	titulares, err := s.BuscarTitulares(t.Context(), aplicacion.FiltroTitulares{Nombre: "zzzz"})
	if err != nil {
		t.Fatalf("una busqueda vacia no es un error: %v", err)
	}
	if len(titulares) != 0 {
		t.Fatalf("se esperaba lista vacia, llegaron %v", idsTitulares(titulares))
	}
}

// El comodin que escriba quien busca es texto, no sintaxis: "%" no puede
// significar "el padron entero". Lo resuelve patronContiene, y esto comprueba
// que ESTA consulta pasa por el.
func TestBuscarTitularesNoInterpretaLosComodinesDelUsuario(t *testing.T) {
	s, _ := padronCompleto(t)

	titulares, err := s.BuscarTitulares(t.Context(), aplicacion.FiltroTitulares{Nombre: "%"})
	if err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	if len(titulares) != 0 {
		t.Fatalf("un '%%' literal no esta en ningun nombre, llegaron %v", idsTitulares(titulares))
	}
}
