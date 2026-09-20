package aplicacion

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// catalogoFalso cuenta cuantas veces le tocaron la base. Es lo que hace
// comprobable que la validacion corre ANTES y no despues.
//
// Satisface los dos puertos que el caso de uso inyecta. Los tres metodos de
// [GestionDeclaraciones] que el catalogo NO usa -Guardar, Historial y
// VigenteEn- entran por la interfaz embebida sin valor: si el catalogo llegara
// a llamarlos, revienta aqui en vez de pasar la prueba con un doble que miente
// sobre lo que el catalogo hace.
type catalogoFalso struct {
	GestionDeclaraciones

	registros      int
	actualizadas   int
	obraRecibida   repertorio.Obra
	filtroRecibido FiltroObras
	err            error

	// obras es lo que devuelve Buscar. Una pagina, no una obra.
	obras []repertorio.Obra

	// vigentes es lo que devuelve el puerto de declaraciones, y consultas
	// cuenta cuantas veces se le llamo: con N obras tiene que ser UNA -es la
	// razon de ser de [GestionDeclaraciones.VigentesDeObras]-.
	vigentes    map[string]VersionDeclaracion
	errVigentes error
	consultas   int
	idsPedidos  []string
}

func (c *catalogoFalso) Registrar(_ context.Context, o repertorio.Obra) error {
	c.registros++
	c.obraRecibida = o
	return c.err
}

func (c *catalogoFalso) Actualizar(_ context.Context, o repertorio.Obra) error {
	c.actualizadas++
	c.obraRecibida = o
	return c.err
}

func (c *catalogoFalso) PorID(_ context.Context, _ string) (repertorio.Obra, error) {
	return c.obraRecibida, c.err
}

func (c *catalogoFalso) Buscar(_ context.Context, f FiltroObras) ([]repertorio.Obra, error) {
	c.filtroRecibido = f
	return c.obras, c.err
}

func (c *catalogoFalso) VigentesDeObras(_ context.Context, ids []string) (map[string]VersionDeclaracion, error) {
	c.consultas++
	c.idsPedidos = ids
	return c.vigentes, c.errVigentes
}

// catalogoDePrueba cablea los dos puertos en el mismo doble: el tipo satisface
// los dos, y el nucleo sigue viendo dos interfaces distintas.
func catalogoDePrueba(repo *catalogoFalso) Catalogo {
	return Catalogo{Obras: repo, Declaraciones: repo}
}

func metadatosValidos() repertorio.Metadatos {
	return repertorio.Metadatos{
		Titulo: "La Casa de las Dos Palmas",
		Genero: "Drama",
		Anio:   1991,
		Tipo:   repertorio.TipoSerie,
		Coautores: []repertorio.Coautor{
			{Nombre: "Ana Escritora", IPI: "IPI-00000001", Rol: repertorio.RolGuionista},
		},
	}
}

func TestRegistrarObraConstruyeLaEntidadYLaGuarda(t *testing.T) {
	repo := &catalogoFalso{}
	cat := catalogoDePrueba(repo)

	obra, err := cat.RegistrarObra(t.Context(), "obra-1", metadatosValidos())
	if err != nil {
		t.Fatalf("RegistrarObra: %v", err)
	}
	if obra.ID() != "obra-1" {
		t.Fatalf("ID = %q", obra.ID())
	}
	if repo.registros != 1 {
		t.Fatalf("se esperaba 1 escritura, hubo %d", repo.registros)
	}
	if repo.obraRecibida.ID() != "obra-1" {
		t.Fatalf("al puerto le llego otra obra: %q", repo.obraRecibida.ID())
	}
}

// La invariante se comprueba en el nucleo, no en la base: una obra sin genero
// no llega ni a intentarse. Si llegara, el CHECK la rechazaria con un mensaje
// de restriccion en vez de decir que campo falta.
func TestRegistrarObraInvalidaNoTocaElPuerto(t *testing.T) {
	casos := map[string]func(*repertorio.Metadatos){
		"sin titulo":    func(m *repertorio.Metadatos) { m.Titulo = "" },
		"sin genero":    func(m *repertorio.Metadatos) { m.Genero = "" },
		"sin anio":      func(m *repertorio.Metadatos) { m.Anio = 0 },
		"sin coautores": func(m *repertorio.Metadatos) { m.Coautores = nil },
		"coautor sin IPI": func(m *repertorio.Metadatos) {
			m.Coautores[0].IPI = ""
		},
	}

	for nombre, romper := range casos {
		t.Run(nombre, func(t *testing.T) {
			repo := &catalogoFalso{}
			m := metadatosValidos()
			romper(&m)

			_, err := catalogoDePrueba(repo).RegistrarObra(t.Context(), "obra-1", m)
			if !errors.Is(err, repertorio.ErrObraInvalida) {
				t.Fatalf("se esperaba ErrObraInvalida, se obtuvo %v", err)
			}
			if repo.registros != 0 {
				t.Fatal("se intento escribir una obra que el dominio rechaza")
			}
		})
	}
}

// El centinela del duplicado sube sin envolver en un texto que lo tape: el
// adaptador HTTP lo distingue con errors.Is para responder 409.
func TestRegistrarObraPropagaElDuplicado(t *testing.T) {
	repo := &catalogoFalso{err: ErrObraDuplicada}

	_, err := catalogoDePrueba(repo).RegistrarObra(t.Context(), "obra-1", metadatosValidos())
	if !errors.Is(err, ErrObraDuplicada) {
		t.Fatalf("se esperaba ErrObraDuplicada, se obtuvo %v", err)
	}
}

// ActualizarMetadatosObra revalida con el mismo constructor que el alta: una
// obra corregida cumple lo mismo que una recien creada.
func TestActualizarMetadatosObraRevalida(t *testing.T) {
	repo := &catalogoFalso{}
	m := metadatosValidos()
	m.Coautores[0].Rol = "director" // RD 7.3.3: no genera derecho de autor

	_, err := catalogoDePrueba(repo).ActualizarMetadatosObra(t.Context(), "obra-1", m)
	if !errors.Is(err, repertorio.ErrObraInvalida) {
		t.Fatalf("se esperaba ErrObraInvalida, se obtuvo %v", err)
	}
	if repo.actualizadas != 0 {
		t.Fatal("se intento actualizar con unos metadatos que el dominio rechaza")
	}
}

// El id es el que llega por parametro, y es el unico que puede ser: los
// metadatos no tienen campo donde meter otro.
func TestActualizarMetadatosObraConservaElIdentificador(t *testing.T) {
	repo := &catalogoFalso{}

	obra, err := catalogoDePrueba(repo).ActualizarMetadatosObra(t.Context(), "obra-1", metadatosValidos())
	if err != nil {
		t.Fatalf("ActualizarMetadatosObra: %v", err)
	}
	if obra.ID() != "obra-1" || repo.obraRecibida.ID() != "obra-1" {
		t.Fatalf("id = %q / %q", obra.ID(), repo.obraRecibida.ID())
	}
	if repo.actualizadas != 1 || repo.registros != 0 {
		t.Fatalf("actualizar no puede dar de alta: %d actualizaciones, %d altas",
			repo.actualizadas, repo.registros)
	}
}

func TestBuscarObrasPasaElFiltroTalCual(t *testing.T) {
	repo := &catalogoFalso{}
	quiero := FiltroObras{
		Titulo:     "palmas",
		Genero:     "Drama",
		IPI:        "IPI-1",
		Anio:       1991,
		Paginacion: Paginacion{Limite: 25, Desplazamiento: 10},
	}

	if _, err := catalogoDePrueba(repo).BuscarObras(t.Context(), quiero); err != nil {
		t.Fatalf("BuscarObras: %v", err)
	}
	if repo.filtroRecibido != quiero {
		t.Fatalf("filtro = %+v, se esperaba %+v", repo.filtroRecibido, quiero)
	}
}

// La garantia de "filtro vacio = primera pagina" vive en el caso de uso, no
// en cada adaptador: asi cualquier CatalogoObras la hereda y se comprueba
// sin Postgres.
func TestBuscarObrasAplicaPaginacionPorDefecto(t *testing.T) {
	repo := &catalogoFalso{}

	if _, err := catalogoDePrueba(repo).BuscarObras(t.Context(), FiltroObras{}); err != nil {
		t.Fatalf("BuscarObras: %v", err)
	}
	if repo.filtroRecibido.Limite != LimiteObrasPorDefecto {
		t.Fatalf("Limite = %d, se esperaba %d", repo.filtroRecibido.Limite, LimiteObrasPorDefecto)
	}
	if repo.filtroRecibido.Desplazamiento != 0 {
		t.Fatalf("Desplazamiento = %d, se esperaba 0", repo.filtroRecibido.Desplazamiento)
	}
}

// LimiteSinTope es una eleccion explicita: ConDefecto no la sustituye.
func TestBuscarObrasRespetaLimiteSinTope(t *testing.T) {
	repo := &catalogoFalso{}
	quiero := FiltroObras{Paginacion: Paginacion{Limite: LimiteSinTope}}

	if _, err := catalogoDePrueba(repo).BuscarObras(t.Context(), quiero); err != nil {
		t.Fatalf("BuscarObras: %v", err)
	}
	if repo.filtroRecibido.Limite != LimiteSinTope {
		t.Fatalf("Limite = %d, se esperaba LimiteSinTope (%d)",
			repo.filtroRecibido.Limite, LimiteSinTope)
	}
}

// ---------------------------------------------------------------------------
// El estado de la declaracion en el catalogo (D-008)

// obraConID construye una obra valida y distinta por identificador.
func obraConID(t *testing.T, id string) repertorio.Obra {
	t.Helper()
	m := metadatosValidos()
	m.Titulo = "Obra " + id
	o, err := repertorio.NuevaObra(id, m)
	if err != nil {
		t.Fatalf("construir la obra %q: %v", id, err)
	}
	return o
}

// parteDePrueba es una parte valida: cada indice es un titular distinto, con
// su IPI. El estado de la declaracion sale de la suma, no del numero de
// partes.
func parteDePrueba(indice int, porcentaje int64) repertorio.Parte {
	return repertorio.Parte{
		TitularID:  fmt.Sprintf("tit-%d", indice),
		IPI:        fmt.Sprintf("IPI-%08d", indice),
		Porcentaje: decimal.NewFromInt(porcentaje),
	}
}

func vigenteDePrueba(obraID string, version int, partes ...repertorio.Parte) VersionDeclaracion {
	return VersionDeclaracion{
		Version:     version,
		Declaracion: repertorio.Declaracion{ObraID: obraID, Partes: partes},
	}
}

// Lo que este caso comprueba es la distincion que motivo el campo
// version_vigente: una obra SIN declaracion y una declarada a medias dan el
// mismo estado -`incompleta` es un estado valido del negocio bajo R-04, no un
// error-, y lo unico que las separa es que en la primera no hay ninguna
// version. Si la version no viajara, la pantalla tendria que pintar
// "incompleta" sobre una obra que nadie declaro.
func TestObraSinDeclaracionSeDistingueDeUnaIncompleta(t *testing.T) {
	sinDeclaracion := obraConID(t, "obra-sin-declaracion")
	incompleta := obraConID(t, "obra-incompleta")

	repo := &catalogoFalso{
		obras: []repertorio.Obra{sinDeclaracion, incompleta},
		vigentes: map[string]VersionDeclaracion{
			"obra-incompleta": vigenteDePrueba("obra-incompleta", 2, parteDePrueba(0, 60)),
		},
	}

	obras, err := catalogoDePrueba(repo).BuscarObras(t.Context(), FiltroObras{})
	if err != nil {
		t.Fatalf("BuscarObras: %v", err)
	}
	if len(obras) != 2 {
		t.Fatalf("se esperaban 2 obras, llegaron %d", len(obras))
	}

	sin := obras[0]
	if sin.ID() != "obra-sin-declaracion" {
		t.Fatalf("el orden de la pagina cambio: %q", sin.ID())
	}
	if sin.EstadoDecl != "incompleta" {
		t.Fatalf("estado de una obra sin declarar = %q, se esperaba incompleta (R-04)", sin.EstadoDecl)
	}
	if sin.VersionVigente != nil {
		t.Fatalf("version vigente de una obra sin declarar = %v, se esperaba nil", *sin.VersionVigente)
	}
	if !sin.SumaPorcentajes.IsZero() {
		t.Fatalf("suma de una obra sin declarar = %s, se esperaba 0", sin.SumaPorcentajes)
	}

	conDeclaracion := obras[1]
	if conDeclaracion.EstadoDecl != "incompleta" {
		t.Fatalf("estado de una declaracion de 60 = %q, se esperaba incompleta", conDeclaracion.EstadoDecl)
	}
	if conDeclaracion.VersionVigente == nil || *conDeclaracion.VersionVigente != 2 {
		t.Fatalf("version vigente = %v, se esperaba 2", conDeclaracion.VersionVigente)
	}
	if !conDeclaracion.SumaPorcentajes.Equal(decimal.NewFromInt(60)) {
		t.Fatalf("suma = %s, se esperaba 60", conDeclaracion.SumaPorcentajes)
	}
}

// Una pagina con N obras se resuelve con UNA consulta de declaraciones: es la
// razon de ser de [GestionDeclaraciones.VigentesDeObras], y una version que
// preguntara obra por obra pasaria este mismo caso con N consultas.
func TestBuscarObrasLeeLaDeclaracionDeLaPaginaEnUnaConsulta(t *testing.T) {
	repo := &catalogoFalso{
		obras: []repertorio.Obra{
			obraConID(t, "obra-1"), obraConID(t, "obra-2"), obraConID(t, "obra-3"),
		},
		vigentes: map[string]VersionDeclaracion{
			"obra-1": vigenteDePrueba("obra-1", 1, parteDePrueba(0, 60), parteDePrueba(1, 40)),
			"obra-3": vigenteDePrueba("obra-3", 7, parteDePrueba(0, 25)),
		},
	}

	obras, err := catalogoDePrueba(repo).BuscarObras(t.Context(), FiltroObras{})
	if err != nil {
		t.Fatalf("BuscarObras: %v", err)
	}
	if repo.consultas != 1 {
		t.Fatalf("consultas de declaraciones = %d, se esperaba 1 para toda la pagina", repo.consultas)
	}
	if len(repo.idsPedidos) != 3 {
		t.Fatalf("ids pedidos = %v, se esperaban los tres de la pagina", repo.idsPedidos)
	}

	if obras[0].EstadoDecl != "completa" || !obras[0].SumaPorcentajes.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("obra 1 = %s / %s, se esperaba completa / 100", obras[0].EstadoDecl, obras[0].SumaPorcentajes)
	}
	if obras[0].VersionVigente == nil || *obras[0].VersionVigente != 1 {
		t.Fatalf("version vigente de la obra 1 = %v, se esperaba 1", obras[0].VersionVigente)
	}
	// obra-2 no aparece en el mapa del puerto: no tiene declaracion.
	if obras[1].VersionVigente != nil || obras[1].EstadoDecl != "incompleta" {
		t.Fatalf("obra 2 = %+v, se esperaba sin declaracion", obras[1])
	}
	if obras[2].VersionVigente == nil || *obras[2].VersionVigente != 7 {
		t.Fatalf("version vigente de la obra 3 = %v, se esperaba 7", obras[2].VersionVigente)
	}
}

// Una pagina sin resultados no va a la base a preguntar por cero obras.
func TestBuscarObrasSinResultadosNoConsultaDeclaraciones(t *testing.T) {
	repo := &catalogoFalso{}

	obras, err := catalogoDePrueba(repo).BuscarObras(t.Context(), FiltroObras{})
	if err != nil {
		t.Fatalf("BuscarObras: %v", err)
	}
	if obras == nil {
		t.Fatal("una pagina vacia tiene que salir como [] y no como nil")
	}
	if repo.consultas != 0 {
		t.Fatalf("consultas = %d, se esperaba ninguna para una pagina vacia", repo.consultas)
	}
}

// Las CUATRO respuestas que devuelven una obra salen por el mismo camino: si
// una sola dejara de componer el estado, el contrato prometeria tres campos
// que esa respuesta no trae. Es el guardia del pre-mortem de este paso.
func TestLasCuatroRespuestasDelCatalogoLlevanElEstado(t *testing.T) {
	vigentes := map[string]VersionDeclaracion{
		"obra-1": vigenteDePrueba("obra-1", 3, parteDePrueba(0, 70), parteDePrueba(1, 30)),
	}
	tres := 3

	// Las expectativas son POR CASO, y no las mismas para los cuatro, porque el
	// alta no es el mismo caso: es el unico que corre sobre una fila de `obras`
	// que acaba de crear el mismo, asi que no puede tener declaracion que leer.
	// Antes de este paso el alta compartia las expectativas de las tres lecturas
	// y las cumplia... porque el doble le devolvia una declaracion de un mapa
	// que en produccion seria imposible: la fila no existia. `consultas: 0` es
	// justo lo que el alta tiene que dejar de hacer.
	casos := map[string]struct {
		llamar    func(t *testing.T, repo *catalogoFalso) (ObraDelCatalogo, error)
		estado    string
		suma      decimal.Decimal
		version   *int
		consultas int
	}{
		"RegistrarObra": {
			llamar: func(_ *testing.T, repo *catalogoFalso) (ObraDelCatalogo, error) {
				return catalogoDePrueba(repo).RegistrarObra(t.Context(), "obra-1", metadatosValidos())
			},
			estado: "incompleta", suma: decimal.Zero, version: nil, consultas: 0,
		},
		"ActualizarMetadatosObra": {
			llamar: func(_ *testing.T, repo *catalogoFalso) (ObraDelCatalogo, error) {
				return catalogoDePrueba(repo).ActualizarMetadatosObra(t.Context(), "obra-1", metadatosValidos())
			},
			estado: "completa", suma: decimal.NewFromInt(100), version: &tres, consultas: 1,
		},
		"ObraPorID": {
			llamar: func(_ *testing.T, repo *catalogoFalso) (ObraDelCatalogo, error) {
				return catalogoDePrueba(repo).ObraPorID(t.Context(), "obra-1")
			},
			estado: "completa", suma: decimal.NewFromInt(100), version: &tres, consultas: 1,
		},
		"BuscarObras": {
			llamar: func(t *testing.T, repo *catalogoFalso) (ObraDelCatalogo, error) {
				obras, err := catalogoDePrueba(repo).BuscarObras(t.Context(), FiltroObras{})
				if err != nil {
					return ObraDelCatalogo{}, err
				}
				if len(obras) != 1 {
					t.Fatalf("se esperaba 1 obra en la pagina, llegaron %d", len(obras))
				}
				return obras[0], nil
			},
			estado: "completa", suma: decimal.NewFromInt(100), version: &tres, consultas: 1,
		},
	}

	for nombre, caso := range casos {
		t.Run(nombre, func(t *testing.T) {
			repo := &catalogoFalso{
				obraRecibida: obraConID(t, "obra-1"),
				obras:        []repertorio.Obra{obraConID(t, "obra-1")},
				vigentes:     vigentes,
			}

			obra, err := caso.llamar(t, repo)
			if err != nil {
				t.Fatalf("%s: %v", nombre, err)
			}
			if obra.EstadoDecl != caso.estado {
				t.Fatalf("estado = %q, se esperaba %q", obra.EstadoDecl, caso.estado)
			}
			if !obra.SumaPorcentajes.Equal(caso.suma) {
				t.Fatalf("suma = %s, se esperaba %s", obra.SumaPorcentajes, caso.suma)
			}
			if caso.version == nil {
				if obra.VersionVigente != nil {
					t.Fatalf("version vigente = %v, se esperaba nil en un alta",
						*obra.VersionVigente)
				}
			} else if obra.VersionVigente == nil || *obra.VersionVigente != *caso.version {
				t.Fatalf("version vigente = %v, se esperaba %d", obra.VersionVigente, *caso.version)
			}
			if repo.consultas != caso.consultas {
				t.Fatalf("consultas = %d, se esperaba %d", repo.consultas, caso.consultas)
			}
		})
	}
}

// El alta compone el estado localmente en vez de releerlo (item 2), y esa
// composicion tiene que dar LO MISMO que el camino largo: si divergiera, la
// respuesta del alta diria del estado de la obra recien creada algo distinto de
// lo que dice el listado de esa misma obra un instante despues.
func TestElAltaComponeElMismoEstadoQueLaLectura(t *testing.T) {
	repo := &catalogoFalso{}
	cat := catalogoDePrueba(repo)

	alta, err := cat.RegistrarObra(t.Context(), "obra-1", metadatosValidos())
	if err != nil {
		t.Fatalf("RegistrarObra: %v", err)
	}

	// El camino largo sobre esa misma obra en un mundo sin declaraciones: es
	// exactamente lo que veria `GET /obras`.
	lectura := proyectarObra(alta.Obra, map[string]VersionDeclaracion{})

	if alta.EstadoDecl != lectura.EstadoDecl {
		t.Fatalf("estado del alta = %q, el de la lectura = %q", alta.EstadoDecl, lectura.EstadoDecl)
	}
	if !alta.SumaPorcentajes.Equal(lectura.SumaPorcentajes) {
		t.Fatalf("suma del alta = %s, la de la lectura = %s", alta.SumaPorcentajes, lectura.SumaPorcentajes)
	}
	if alta.VersionVigente != lectura.VersionVigente {
		t.Fatalf("version del alta = %v, la de la lectura = %v", alta.VersionVigente, lectura.VersionVigente)
	}
	// Y la prueba del item 2: el alta no releyo lo que acababa de escribir.
	if repo.consultas != 0 {
		t.Fatalf("consultas = %d: el alta releyo lo que acababa de escribir", repo.consultas)
	}
}

// La suma y el estado son dos datos distintos, y el catalogo manda los dos:
// una declaracion de 100 con una parte sin IPI suma 100 y NO esta completa.
// Deducir uno del otro mentiria en un sentido o en el otro, y el motor de
// reparto se guia por el estado (R-04, RD 13.1.3).
func TestLaSumaNoSeDeduceDelEstadoNiAlReves(t *testing.T) {
	obra := obraConID(t, "obra-sin-ipi")
	sinIPI := []repertorio.Parte{
		parteDePrueba(0, 60),
		{TitularID: "tit-1", IPI: "", Porcentaje: decimal.NewFromInt(40)},
	}

	proyectada := proyectarObra(obra, map[string]VersionDeclaracion{
		"obra-sin-ipi": vigenteDePrueba("obra-sin-ipi", 1, sinIPI...),
	})

	if !proyectada.SumaPorcentajes.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("suma = %s, se esperaba 100", proyectada.SumaPorcentajes)
	}
	if proyectada.EstadoDecl != "incompleta" {
		t.Fatalf("estado = %q: una parte sin IPI deja la declaracion incompleta aunque sume 100",
			proyectada.EstadoDecl)
	}
}

// Un error de la lectura de declaraciones no se traga: el catalogo no puede
// devolver una obra a la que le falta el estado, porque el estado ausente se
// leeria como "sin declaracion".
func TestBuscarObrasPropagaElFalloDeDeclaraciones(t *testing.T) {
	repo := &catalogoFalso{
		obras:       []repertorio.Obra{obraConID(t, "obra-1")},
		errVigentes: errors.New("la base no responde"),
	}

	if _, err := catalogoDePrueba(repo).BuscarObras(t.Context(), FiltroObras{}); err == nil {
		t.Fatal("se esperaba el error del puerto de declaraciones")
	}
}
