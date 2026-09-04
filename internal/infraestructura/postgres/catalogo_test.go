package postgres

import (
	"errors"
	"slices"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// nuevaObra construye una obra valida para las pruebas. El ajuste opcional
// rompe o cambia lo que cada caso necesite, para que el resto quede fijo.
func nuevaObra(t *testing.T, id string, ajustar ...func(*repertorio.Metadatos)) repertorio.Obra {
	t.Helper()

	m := repertorio.Metadatos{
		Titulo: "Senoritas de Uribe",
		Genero: "Comedia",
		Anio:   1997,
		Tipo:   repertorio.TipoSerie,
		Coautores: []repertorio.Coautor{
			{Nombre: "Ana Escritora", IPI: "IPI-00000001", Rol: repertorio.RolGuionista},
			{Nombre: "Beto Libretista", IPI: "IPI-00000002", Rol: repertorio.RolLibretista},
		},
	}
	for _, f := range ajustar {
		f(&m)
	}

	o, err := repertorio.NuevaObra(id, m)
	if err != nil {
		t.Fatalf("construir la obra de prueba: %v", err)
	}
	return o
}

func ids(obras []repertorio.Obra) []string {
	out := make([]string, 0, len(obras))
	for _, o := range obras {
		out = append(out, o.ID())
	}
	return out
}

// ---------------------------------------------------------------------------
// Unidad: no necesita base

// Sin escapar, buscar "100%" pide todo lo que empiece por 100 y buscar "_"
// pide el catalogo entero: el comodin del usuario se mezclaria con el nuestro.
func TestPatronContieneEscapaLosComodines(t *testing.T) {
	casos := map[string]string{
		"":        "",
		"casa":    "%casa%",
		"100%":    `%100\%%`,
		"a_b":     `%a\_b%`,
		`c:\ruta`: `%c:\\ruta%`,
	}
	for entrada, quiero := range casos {
		if got := patronContiene(entrada); got != quiero {
			t.Errorf("patronContiene(%q) = %q, se esperaba %q", entrada, got, quiero)
		}
	}
}

// ---------------------------------------------------------------------------
// CRUD

func TestRegistrarYLeerLaObraCompleta(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	quiero := nuevaObra(t, "obra-nueva", func(m *repertorio.Metadatos) {
		m.IDA, m.EIDR, m.IMDB = "IDA-9", "EIDR-9", "tt9999"
	})
	if err := s.Registrar(ctx, quiero); err != nil {
		t.Fatalf("Registrar: %v", err)
	}

	tengo, err := s.PorID(ctx, "obra-nueva")
	if err != nil {
		t.Fatalf("PorID: %v", err)
	}

	if tengo.ID() != quiero.ID() {
		t.Fatalf("ID = %q, se esperaba %q", tengo.ID(), quiero.ID())
	}
	m := tengo.Metadatos()
	if m.Titulo != "Senoritas de Uribe" || m.Genero != "Comedia" || m.Anio != 1997 {
		t.Fatalf("metadatos mal escaneados: %+v", m)
	}
	if m.Tipo != repertorio.TipoSerie {
		t.Fatalf("Tipo = %q", m.Tipo)
	}
	if m.IDA != "IDA-9" || m.EIDR != "EIDR-9" || m.IMDB != "tt9999" {
		t.Fatalf("identificadores globales mal escaneados: %+v", m)
	}

	// Los dos coautores, con sus roles autorales distintos, sobreviven el
	// viaje de ida y vuelta. Es el criterio de aceptacion del issue.
	coautores := tengo.Coautores()
	if len(coautores) != 2 {
		t.Fatalf("se esperaban 2 coautores, llegaron %d: %+v", len(coautores), coautores)
	}
	if coautores[0].Rol == coautores[1].Rol {
		t.Fatalf("los dos coautores tienen el mismo rol: %+v", coautores)
	}
	if coautores[0].Nombre != "Ana Escritora" || coautores[0].IPI != "IPI-00000001" {
		t.Fatalf("coautor mal escaneado: %+v", coautores[0])
	}
}

// El criterio del issue, literal: un segundo alta con el mismo identificador
// se rechaza. Lo decide la clave primaria, no un SELECT previo.
func TestRegistrarRechazaElIdentificadorDuplicado(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	if err := s.Registrar(ctx, nuevaObra(t, "obra-nueva")); err != nil {
		t.Fatalf("primer alta: %v", err)
	}

	otra := nuevaObra(t, "obra-nueva", func(m *repertorio.Metadatos) {
		m.Titulo = "Una obra completamente distinta"
	})
	err := s.Registrar(ctx, otra)
	if !errors.Is(err, aplicacion.ErrObraDuplicada) {
		t.Fatalf("se esperaba ErrObraDuplicada, se obtuvo %v", err)
	}

	// Y el rechazo no dejo nada a medias: la obra sigue siendo la primera.
	tengo, err := s.PorID(ctx, "obra-nueva")
	if err != nil {
		t.Fatalf("PorID: %v", err)
	}
	if tengo.Metadatos().Titulo != "Senoritas de Uribe" {
		t.Fatalf("el alta rechazada piso la obra original: %q", tengo.Metadatos().Titulo)
	}
}

// El alta es atomica por contrato: si los coautores no entran, la obra
// tampoco. Una obra sin coautores no la puede reconstruir NuevaObra, asi que
// quedaria escrita y no se podria leer.
func TestRegistrarEsAtomico(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	// Un CHECK que solo puede violar esta prueba: el fallo tiene que venir del
	// INSERT de coautores, DESPUES de que la obra ya este escrita dentro de la
	// transaccion. Un dato que el dominio rechazara nunca llegaria hasta aqui.
	if _, err := pool.Exec(ctx,
		`ALTER TABLE obra_coautores
		   ADD CONSTRAINT coautor_imposible CHECK (nombre <> 'Coautor Imposible')`); err != nil {
		t.Fatalf("anadir el CHECK: %v", err)
	}

	obra := nuevaObra(t, "obra-atomica", func(m *repertorio.Metadatos) {
		m.Coautores = []repertorio.Coautor{
			{Nombre: "Coautor Imposible", IPI: "IPI-00000009", Rol: repertorio.RolGuionista},
		}
	})
	if err := s.Registrar(ctx, obra); err == nil {
		t.Fatal("se esperaba que el CHECK abortara el alta")
	}

	var cuantas int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM obras WHERE id = $1`, "obra-atomica").Scan(&cuantas); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if cuantas != 0 {
		t.Fatal("la obra quedo escrita sin sus coautores: la transaccion no revirtio")
	}
}

func TestPorIDDeUnaObraQueNoExiste(t *testing.T) {
	s, _ := sembrar(t)

	_, err := s.PorID(t.Context(), "obra-que-no-existe")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// Actualizar corrige los metadatos y reemplaza los coautores. El
// identificador es el del WHERE y no entra en el SET.
func TestActualizarReemplazaMetadatosYCoautores(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	if err := s.Registrar(ctx, nuevaObra(t, "obra-nueva")); err != nil {
		t.Fatalf("Registrar: %v", err)
	}

	corregida := nuevaObra(t, "obra-nueva", func(m *repertorio.Metadatos) {
		m.Titulo = "Senoritas de Uribe (remasterizada)"
		m.Genero = "Drama"
		m.Anio = 1998
		m.Tipo = repertorio.TipoTelenovela
		m.Coautores = []repertorio.Coautor{
			{Nombre: "Carla Adaptadora", IPI: "IPI-00000003", Rol: repertorio.RolAdaptador},
		}
	})
	if err := s.Actualizar(ctx, corregida); err != nil {
		t.Fatalf("Actualizar: %v", err)
	}

	tengo, err := s.PorID(ctx, "obra-nueva")
	if err != nil {
		t.Fatalf("PorID: %v", err)
	}
	if tengo.ID() != "obra-nueva" {
		t.Fatalf("la actualizacion cambio el identificador: %q", tengo.ID())
	}
	m := tengo.Metadatos()
	if m.Titulo != "Senoritas de Uribe (remasterizada)" || m.Genero != "Drama" || m.Anio != 1998 {
		t.Fatalf("metadatos sin actualizar: %+v", m)
	}
	// Los dos coautores viejos ya no estan: el bloque se reemplaza entero.
	if len(m.Coautores) != 1 || m.Coautores[0].IPI != "IPI-00000003" {
		t.Fatalf("coautores sin reemplazar: %+v", m.Coautores)
	}
}

// Cero filas afectadas es "esa obra no existe", y un UPDATE no lo dice por
// error: dice que fue bien y no toco nada.
func TestActualizarUnaObraQueNoExisteEsNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)

	err := s.Actualizar(t.Context(), nuevaObra(t, "obra-que-no-existe"))
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// Y no la crea. Un PATCH que inserta convierte un id mal escrito en una obra
// fantasma del catalogo contra el que resuelve todo el matching.
func TestActualizarNoCreaLaObra(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	_ = s.Actualizar(ctx, nuevaObra(t, "obra-fantasma"))

	var cuantas int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM obras WHERE id = $1`, "obra-fantasma").Scan(&cuantas); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if cuantas != 0 {
		t.Fatal("Actualizar creo una obra que no existia")
	}
}

// ---------------------------------------------------------------------------
// Busqueda: los tres caminos del criterio de aceptacion, mas el anio

func TestBuscarPorTituloParcial(t *testing.T) {
	s, _ := sembrar(t)

	// Un trozo del medio y en otra caja: es lo que resuelve el indice GIN de
	// trigramas con ILIKE '%...%'.
	obras, err := s.Buscar(t.Context(), aplicacion.FiltroObras{Titulo: "dos palmas"})
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if got := ids(obras); !slices.Equal(got, []string{obraCompleta}) {
		t.Fatalf("ids = %v, se esperaba [%s]", got, obraCompleta)
	}
}

func TestBuscarPorGenero(t *testing.T) {
	s, _ := sembrar(t)

	obras, err := s.Buscar(t.Context(), aplicacion.FiltroObras{Genero: "Drama"})
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if got := ids(obras); !slices.Equal(got, []string{obraCompleta, obraIncompleta}) {
		t.Fatalf("ids = %v, se esperaba [%s %s]", got, obraCompleta, obraIncompleta)
	}
}

// El IPI identifica PERSONAS, no obras: la busqueda cruza contra los coautores
// del catalogo (docs/dominio/identificadores.md).
func TestBuscarPorIPIDeUnCoautor(t *testing.T) {
	s, _ := sembrar(t)

	obras, err := s.Buscar(t.Context(), aplicacion.FiltroObras{IPI: "IPI-00000001"})
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if got := ids(obras); !slices.Equal(got, []string{obraCompleta, obraSinDeclaracion}) {
		t.Fatalf("ids = %v, se esperaba [%s %s]", got, obraCompleta, obraSinDeclaracion)
	}
}

func TestBuscarPorAnio(t *testing.T) {
	s, _ := sembrar(t)

	obras, err := s.Buscar(t.Context(), aplicacion.FiltroObras{Anio: 1991})
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if got := ids(obras); !slices.Equal(got, []string{obraCompleta, obraSinIPI}) {
		t.Fatalf("ids = %v, se esperaba [%s %s]", got, obraCompleta, obraSinIPI)
	}
}

// Los filtros se combinan con Y, no con O. Con O, un genero que no cuadra
// devolveria obras de todas formas y el buscador mentiria.
func TestBuscarCombinaLosFiltrosConY(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	obras, err := s.Buscar(ctx, aplicacion.FiltroObras{Genero: "Drama", Anio: 1991})
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if got := ids(obras); !slices.Equal(got, []string{obraCompleta}) {
		t.Fatalf("ids = %v, se esperaba [%s]", got, obraCompleta)
	}

	obras, err = s.Buscar(ctx, aplicacion.FiltroObras{Genero: "Drama", Anio: 2005})
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if len(obras) != 0 {
		t.Fatalf("Drama de 2005 no existe, llegaron %v", ids(obras))
	}
}

// Un filtro vacio es el listado del catalogo. Y el orden es explicito: el ADR
// 0005 exige que una corrida se reproduzca bit a bit.
func TestBuscarSinFiltroDevuelveElCatalogoOrdenado(t *testing.T) {
	s, _ := sembrar(t)

	obras, err := s.Buscar(t.Context(), aplicacion.FiltroObras{})
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if len(obras) != 4 {
		t.Fatalf("se esperaban las 4 obras sembradas, llegaron %d", len(obras))
	}
	if got := ids(obras); !slices.IsSorted(got) {
		t.Fatalf("el catalogo no viene ordenado por id: %v", got)
	}
}

// Una busqueda sin coincidencias devuelve lista vacia y NINGUN error. "No hay
// filas" solo es ErrNoEncontrado cuando se pidio una fila concreta.
func TestBuscarSinCoincidenciasNoEsError(t *testing.T) {
	s, _ := sembrar(t)

	obras, err := s.Buscar(t.Context(), aplicacion.FiltroObras{Titulo: "zzzz"})
	if err != nil {
		t.Fatalf("una busqueda vacia no es un error: %v", err)
	}
	if len(obras) != 0 {
		t.Fatalf("se esperaba lista vacia, llegaron %v", ids(obras))
	}
}

// El comodin que escriba quien busca es texto, no sintaxis: "%" no puede
// significar "todo el catalogo".
func TestBuscarNoInterpretaLosComodinesDelUsuario(t *testing.T) {
	s, _ := sembrar(t)

	obras, err := s.Buscar(t.Context(), aplicacion.FiltroObras{Titulo: "%"})
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if len(obras) != 0 {
		t.Fatalf("un '%%' literal no esta en ningun titulo, llegaron %v", ids(obras))
	}
}

// ---------------------------------------------------------------------------
// El esquema, comprobado contra el catalogo de la base y no contra la
// migracion: si alguien anade una columna de dinero, esto se pone rojo.

// El catalogo dice QUIEN ESCRIBIO la obra; `declaraciones` dice a quien se le
// paga y cuanto. Un porcentaje aqui abriria un segundo camino hasta un pago
// que no firma ninguna Declaracion de Obra (`R-02`, `R-03`, `RD 7.3.1`).
func TestLaTablaDeCoautoresNoTieneColumnaDeDinero(t *testing.T) {
	_, pool := sembrar(t)

	filas, err := pool.Query(t.Context(),
		`SELECT column_name, data_type
		   FROM information_schema.columns
		  WHERE table_name = 'obra_coautores'
		  ORDER BY column_name`)
	if err != nil {
		t.Fatalf("consultar el catalogo: %v", err)
	}
	defer filas.Close()

	permitidas := []string{"ipi", "nombre", "obra_id", "rol"}
	var columnas []string
	for filas.Next() {
		var nombre, tipo string
		if err := filas.Scan(&nombre, &tipo); err != nil {
			t.Fatalf("escanear: %v", err)
		}
		if tipo == "numeric" {
			t.Fatalf("obra_coautores.%s es NUMERIC: el catalogo no reparte", nombre)
		}
		columnas = append(columnas, nombre)
	}
	if err := filas.Err(); err != nil {
		t.Fatalf("recorrer: %v", err)
	}
	if !slices.Equal(columnas, permitidas) {
		t.Fatalf("columnas = %v, se esperaba %v", columnas, permitidas)
	}
}
