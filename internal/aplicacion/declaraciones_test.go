package aplicacion

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/afiliacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// gestionFalsa cuenta cuantas veces le tocaron la base. Es lo que hace
// comprobable que la validacion del dominio corre ANTES y no despues, igual
// que catalogoFalso.
type gestionFalsa struct {
	guardadas             int
	declRecibida          repertorio.Declaracion
	ahoraRecibida         time.Time
	actorIDRecibido       string
	versionADevolver      int
	vigenteDesdeADevolver time.Time
	historial             []VersionDeclaracion
	vigente               VersionDeclaracion
	err                   error
}

// Guardar devuelve, por defecto, el mismo ahora que recibe -asi el doble
// falso se comporta como el caso comun, sin vigenteDesdeADevolver de por
// medio, en las pruebas que no estan verificando el ajuste del puerto-.
func (g *gestionFalsa) Guardar(_ context.Context, d repertorio.Declaracion, ahora time.Time, actorID string) (int, time.Time, error) {
	g.guardadas++
	g.declRecibida = d
	g.ahoraRecibida = ahora
	g.actorIDRecibido = actorID
	if g.err != nil {
		return 0, time.Time{}, g.err
	}
	version := g.versionADevolver
	if version == 0 {
		version = 1
	}
	vigenteDesde := ahora
	if !g.vigenteDesdeADevolver.IsZero() {
		vigenteDesde = g.vigenteDesdeADevolver
	}
	return version, vigenteDesde, nil
}

func (g *gestionFalsa) Historial(_ context.Context, _ string) ([]VersionDeclaracion, error) {
	return g.historial, g.err
}

func (g *gestionFalsa) VigenteEn(_ context.Context, _ string, _ time.Time) (VersionDeclaracion, error) {
	return g.vigente, g.err
}

// VigentesDeObras no la usa el editor de splits -la lee el catalogo para decir
// en que estado esta cada obra (ver catalogo_test.go)-, pero el doble
// implementa el puerto entero, asi que tiene que estar.
func (g *gestionFalsa) VigentesDeObras(_ context.Context, _ []string) (map[string]VersionDeclaracion, error) {
	return nil, g.err
}

func partesValidas() []repertorio.Parte {
	return []repertorio.Parte{
		{TitularID: "t1", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(60)},
		{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(40)},
	}
}

// sociedadDelPadron es una productora: existe en el padron y no puede recibir
// reparto (`R-01`, `RD 4.5`). Sin IPI, que es lo que el propio invariante del
// padron admite para una persona juridica.
func sociedadDelPadron(t *testing.T) afiliacion.Titular {
	t.Helper()
	return titularDePrueba(t, "t2", "Productora del Caribe S.A.S.", "", false, afiliacion.ClaseAdministrado)
}

// padronConsultado dice si el caso de uso llego a leer el padron. El doble
// -ver titulares_test.go- guarda el filtro que le llego, y el filtro con que
// este caso de uso pregunta no es el valor cero: lleva los ids que la
// declaracion nombra. Un filtro sin ids es, entonces, "no se pregunto nada".
func padronConsultado(p *padronFalso) bool {
	return len(p.filtroRecibido.IDs) != 0
}

// GuardarSplits delega TODO el trabajo de guardar y asentar en un unico
// metodo de puerto (ver el comentario de GestionDeclaraciones en puertos.go):
// esta prueba comprueba que le llegan los datos correctos, no que orqueste
// una escritura y un asiento por separado -eso ya no existe, y es a proposito.
func TestGuardarSplitsValidaYDelegaEnElPuerto(t *testing.T) {
	gestion := &gestionFalsa{versionADevolver: 1}
	momento := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	d := Declaraciones{Gestion: gestion, Padron: &padronFalso{}, Reloj: relojFijo{instante: momento}}

	vd, err := d.GuardarSplits(t.Context(), "obra-1", partesValidas(), "usr-admin")
	if err != nil {
		t.Fatalf("GuardarSplits: %v", err)
	}
	if vd.Version != 1 || !vd.VigenteDesde.Equal(momento) {
		t.Fatalf("version = %+v", vd)
	}
	if gestion.guardadas != 1 {
		t.Fatalf("se esperaba 1 escritura, hubo %d", gestion.guardadas)
	}
	if gestion.declRecibida.ObraID != "obra-1" {
		t.Fatalf("al puerto le llego otra obra: %q", gestion.declRecibida.ObraID)
	}
	if gestion.actorIDRecibido != "usr-admin" {
		t.Fatalf("actor = %q", gestion.actorIDRecibido)
	}
}

// GuardarSplits tiene que devolver el vigente_desde que el PUERTO dice que
// escribio, no el instante de Reloj.Ahora() que le mando: el puerto puede
// ajustarlo (ver postgres.Store.Guardar) para que no choque con la version
// que cierra, y devolver el valor local en vez del real es precisamente el
// bloqueante que esta prueba existe para cazar.
func TestGuardarSplitsDevuelveElVigenteDesdeDelPuertoNoElDelReloj(t *testing.T) {
	momento := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	ajustado := momento.Add(time.Microsecond)
	gestion := &gestionFalsa{versionADevolver: 2, vigenteDesdeADevolver: ajustado}
	d := Declaraciones{Gestion: gestion, Padron: &padronFalso{}, Reloj: relojFijo{instante: momento}}

	vd, err := d.GuardarSplits(t.Context(), "obra-1", partesValidas(), "usr-admin")
	if err != nil {
		t.Fatalf("GuardarSplits: %v", err)
	}
	if !vd.VigenteDesde.Equal(ajustado) {
		t.Fatalf("VigenteDesde = %s, se esperaba el valor ajustado del puerto %s", vd.VigenteDesde, ajustado)
	}
}

// La validacion del dominio corre antes de tocar el puerto: unas partes que
// suman mas de 100 no llegan ni a intentarse. Y tampoco gastan la consulta al
// padron: lo que se puede decidir sin salir del nucleo se decide primero.
func TestGuardarSplitsInvalidoNoTocaElPuerto(t *testing.T) {
	gestion := &gestionFalsa{}
	padron := &padronFalso{}
	d := Declaraciones{Gestion: gestion, Padron: padron, Reloj: relojFijo{}}

	partes := []repertorio.Parte{
		{TitularID: "t1", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(60)},
		{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(60)},
	}

	_, err := d.GuardarSplits(t.Context(), "obra-1", partes, "usr-admin")
	if !errors.Is(err, repertorio.ErrDeclaracionInvalida) {
		t.Fatalf("se esperaba ErrDeclaracionInvalida, se obtuvo %v", err)
	}
	if gestion.guardadas != 0 {
		t.Fatal("se intento guardar una declaracion que el dominio rechaza")
	}
	if padronConsultado(padron) {
		t.Fatal("se consulto el padron por unas partes que no forman una declaracion")
	}
}

// Issue #109 (Roy, revisando la #106): un porcentaje positivo por debajo de
// la mitad del ultimo decimal pasaba NuevaDeclaracion (es > 0 en la
// precision arbitraria de Go) y llegaba hasta el puerto, donde NUMERIC(8,4)
// lo redondeaba a 0.0000 y reventaba un CHECK que el handler HTTP no sabe
// traducir -sale como 500, no como el 400 que promete el contrato. Esta
// prueba fija el limite en la capa de caso de uso: GuardarSplits tiene que
// rechazarlo ANTES de tocar el puerto, igual que con una suma > 100.
func TestGuardarSplitsRechazaPorcentajeQueNumericRedondeariaACero(t *testing.T) {
	gestion := &gestionFalsa{}
	padron := &padronFalso{}
	d := Declaraciones{Gestion: gestion, Padron: padron, Reloj: relojFijo{}}

	partes := []repertorio.Parte{
		{TitularID: "t1", IPI: "IPI-1", Porcentaje: decimal.RequireFromString("0.00004")},
		{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.RequireFromString("99.99996")},
	}

	_, err := d.GuardarSplits(t.Context(), "obra-1", partes, "usr-admin")
	if !errors.Is(err, repertorio.ErrDeclaracionInvalida) {
		t.Fatalf("se esperaba ErrDeclaracionInvalida, se obtuvo %v", err)
	}
	if gestion.guardadas != 0 {
		t.Fatal("se intento guardar un porcentaje que NUMERIC(8,4) redondearia a 0 en silencio")
	}
	if padronConsultado(padron) {
		t.Fatal("se consulto el padron por unas partes que no forman una declaracion")
	}
}

// Si Guardar falla -incluido un fallo al asentar, que ahora vive DENTRO de
// esa misma llamada (ver [Store.Guardar] en postgres/declaraciones.go)-,
// GuardarSplits solo tiene que propagar el error: ya no hay una segunda
// escritura de la que deshacerse en este nivel. La prueba de que no queda una
// version huerfana es de integracion, contra Postgres real:
// TestGuardarRevierteLaVersionSiElAsientoFalla.
func TestGuardarSplitsPropagaElErrorDelPuerto(t *testing.T) {
	gestion := &gestionFalsa{err: errors.New("version y asiento fallaron juntos")}
	d := Declaraciones{Gestion: gestion, Padron: &padronFalso{}, Reloj: relojFijo{}}

	_, err := d.GuardarSplits(t.Context(), "obra-1", partesValidas(), "usr-admin")
	if err == nil {
		t.Fatal("se esperaba un error cuando el puerto falla")
	}
}

func TestGuardarSplitsPropagaNoEncontrado(t *testing.T) {
	gestion := &gestionFalsa{err: ErrNoEncontrado}
	d := Declaraciones{Gestion: gestion, Padron: &padronFalso{}, Reloj: relojFijo{}}

	_, err := d.GuardarSplits(t.Context(), "obra-inexistente", partesValidas(), "usr-admin")
	if !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// El defecto que esta prueba cierra: una declaracion con una sociedad dentro se
// guardaba con 200 y el reparto la rechazaba al pagar. Ahora `R-01`
// (`RD 4.5`) se comprueba ANTES de escribir, y la escritura no llega a
// ocurrir.
//
// El padron devuelve las DOS filas que la declaracion nombra -una que si puede
// recibir reparto y otra que no-, que es lo que devuelve la consulta acotada
// por ids: el rechazo tiene que salir de la segunda y solo de la segunda, no de
// que todo lo que vuelve este excluido.
func TestGuardarSplitsRechazaUnaParteDeQuienNoPuedeRecibirReparto(t *testing.T) {
	gestion := &gestionFalsa{}
	// El IPI de ana es el que `partesValidas()` declara para t1. No es un
	// detalle del Fixture: desde que el caso de uso concilia el IPI contra el
	// padron, una fila del padron con OTRO numero haria fallar la comprobacion
	// nueva y esta prueba dejaria de medir el veredicto de `R-01`, que es lo
	// unico que dice medir. El caso "el IPI no cuadra" tiene su propia prueba.
	ana := titularDePrueba(t, "t1", "Ana Escritora", "IPI-1", true, afiliacion.ClaseSocio)
	padron := &padronFalso{titulares: []afiliacion.Titular{ana, sociedadDelPadron(t)}}
	d := Declaraciones{Gestion: gestion, Padron: padron, Reloj: relojFijo{}}

	// partesValidas() trae t1 y t2, y t2 es la sociedad del padron.
	_, err := d.GuardarSplits(t.Context(), "obra-1", partesValidas(), "usr-admin")
	if !errors.Is(err, ErrTitularNoEsPersonaNatural) {
		t.Fatalf("se esperaba ErrTitularNoEsPersonaNatural, se obtuvo %v", err)
	}
	// Quien edita recibe un mensaje fijo; el nombre de la fila del padron es
	// para el log de quien opera, y sin el ese log no dice a quien rechazo.
	if !strings.Contains(err.Error(), "t2") || !strings.Contains(err.Error(), "Productora del Caribe S.A.S.") {
		t.Fatalf("el error no nombra la fila del padron que lo provoco: %v", err)
	}
	if gestion.guardadas != 0 {
		t.Fatal("se guardo una declaracion con un titular que no puede recibir reparto")
	}
}

// El defecto del 🔴: el IPI declarado no se conciliaba con el del padron y la
// declaracion se guardaba con 200. Reproducido contra la pila real con
// `tit-ana + IPI-00000002` sobre un padron que dice `IPI-00000001`.
func TestGuardarSplitsRechazaUnIPIQueNoEsElDelPadron(t *testing.T) {
	gestion := &gestionFalsa{}
	ana := titularDePrueba(t, "t1", "Ana Escritora", "IPI-00000001", true, afiliacion.ClaseSocio)
	padron := &padronFalso{titulares: []afiliacion.Titular{ana}}
	d := Declaraciones{Gestion: gestion, Padron: padron, Reloj: relojFijo{}}

	partes := []repertorio.Parte{
		{TitularID: "t1", IPI: "IPI-00000002", Porcentaje: decimal.NewFromInt(100)},
	}
	_, err := d.GuardarSplits(t.Context(), "obra-1", partes, "usr-admin")
	if !errors.Is(err, ErrIPIQueNoCuadra) {
		t.Fatalf("se esperaba ErrIPIQueNoCuadra, se obtuvo %v", err)
	}
	// Los dos numeros viajan en el error del nucleo para el log de quien opera.
	// El mensaje que ve quien edita es fijo y NO los repite: ver el handler.
	if !strings.Contains(err.Error(), "IPI-00000002") || !strings.Contains(err.Error(), "IPI-00000001") {
		t.Fatalf("el error no trae los dos IPI para el log: %v", err)
	}
	if gestion.guardadas != 0 {
		t.Fatal("se guardo una version con un IPI que no es el del padron")
	}
}

// Control positivo del arreglo del 🔴: conciliar no puede volverse rechazar. Si
// la comparacion se pasara de estricta, esta prueba cae -y cae junto con ella la
// razon por la que el arreglo es una puerta y no un cambio de negocio.
func TestGuardarSplitsGuardaSiElIPICuadra(t *testing.T) {
	gestion := &gestionFalsa{versionADevolver: 1}
	ana := titularDePrueba(t, "t1", "Ana Escritora", "IPI-00000001", true, afiliacion.ClaseSocio)
	padron := &padronFalso{titulares: []afiliacion.Titular{ana}}
	d := Declaraciones{Gestion: gestion, Padron: padron, Reloj: relojFijo{}}

	partes := []repertorio.Parte{
		{TitularID: "t1", IPI: "IPI-00000001", Porcentaje: decimal.NewFromInt(100)},
	}
	vd, err := d.GuardarSplits(t.Context(), "obra-1", partes, "usr-admin")
	if err != nil {
		t.Fatalf("GuardarSplits con el IPI del padron: %v", err)
	}
	if vd.Version != 1 {
		t.Fatalf("version = %d, se esperaba 1", vd.Version)
	}
	if gestion.guardadas != 1 {
		t.Fatalf("guardadas = %d, se esperaba 1", gestion.guardadas)
	}
}

// El orden de las dos comprobaciones, que es una decision y no un descuido: a un
// titular que no puede recibir reparto NO se le concilia el IPI. La sociedad del
// padron no tiene IPI y la parte declara uno, asi que con el orden invertido el
// rechazo diria "el IPI no cuadra" y mandaria a corregir un numero cuando lo que
// pasa es que esa parte no puede cobrar nunca.
func TestGuardarSplitsAnteUnIPIAjenoDeUnaSociedadGanaR01(t *testing.T) {
	gestion := &gestionFalsa{}
	padron := &padronFalso{titulares: []afiliacion.Titular{sociedadDelPadron(t)}}
	d := Declaraciones{Gestion: gestion, Padron: padron, Reloj: relojFijo{}}

	partes := []repertorio.Parte{
		{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(100)},
	}
	_, err := d.GuardarSplits(t.Context(), "obra-1", partes, "usr-admin")
	if !errors.Is(err, ErrTitularNoEsPersonaNatural) {
		t.Fatalf("se esperaba ErrTitularNoEsPersonaNatural, se obtuvo %v", err)
	}
	if errors.Is(err, ErrIPIQueNoCuadra) {
		t.Fatal("el IPI de una sociedad se concilio: R-01 tiene que ganar")
	}
	if gestion.guardadas != 0 {
		t.Fatal("se guardo una declaracion con una sociedad dentro")
	}
}

// La consulta tiene que llevar los ids que la declaracion NOMBRA, y nada que
// acote por otra cosa. Es lo que la hace depender del tamano de la peticion y
// no del padron: con LimiteSinTope -lo que hacia antes- cada guardado leia el
// padron entero para decidir sobre un punado de titulares, y con el padron real
// cargado eso es cada guardado del editor.
func TestGuardarSplitsPreguntaSoloPorLosTitularesQueNombraLaDeclaracion(t *testing.T) {
	gestion := &gestionFalsa{}
	padron := &padronFalso{}
	d := Declaraciones{Gestion: gestion, Padron: padron, Reloj: relojFijo{}}

	partes := []repertorio.Parte{
		{TitularID: "t3", IPI: "IPI-3", Porcentaje: decimal.NewFromInt(50)},
		{TitularID: "t1", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(30)},
		{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(20)},
	}
	if _, err := d.GuardarSplits(t.Context(), "obra-1", partes, "usr-admin"); err != nil {
		t.Fatalf("GuardarSplits: %v", err)
	}
	// Igualdad exacta: son los tres ids nombrados, en el orden en que la
	// declaracion los nombra y ninguno repetido. Una lista con un id de mas o
	// con uno de menos no pasa esta comprobacion.
	if got := padron.filtroRecibido.IDs; !slices.Equal(got, []string{"t3", "t1", "t2"}) {
		t.Fatalf("ids = %v, se esperaba [t3 t1 t2]", got)
	}
	// Y sin el filtro de persona natural: lo que tiene que volver son las filas
	// de verdad, porque el veredicto lo da `PuedeRecibirReparto()` y no una
	// copia de la regla escrita en la consulta.
	if padron.filtroRecibido.PersonaNatural != nil {
		t.Fatalf("se acoto el padron por persona natural (%v): la regla se decidiria en la consulta y no en la entidad",
			*padron.filtroRecibido.PersonaNatural)
	}
	if padron.filtroRecibido.Limite == LimiteSinTope {
		t.Fatal("se pidio el padron entero: la consulta tiene que estar acotada por el tamano de la declaracion")
	}
	if padron.filtroRecibido.Limite != len(partes) {
		t.Fatalf("Limite = %d, se esperaba %d: el tope es el numero de titulares nombrados",
			padron.filtroRecibido.Limite, len(partes))
	}
}

// Los ids repetidos no se pueden provocar por GuardarSplits:
// [repertorio.NuevaDeclaracion] rechaza un titular repetido antes de llegar
// aqui. Se prueba donde vive la propiedad -la funcion que arma la consulta-,
// porque lo que no puede pasar es que el padron reciba el mismo id dos veces:
// son dos preguntas por la misma fila.
func TestExigirPuedenRecibirRepartoNoRepiteLosIds(t *testing.T) {
	padron := &padronFalso{}
	d := Declaraciones{Gestion: &gestionFalsa{}, Padron: padron, Reloj: relojFijo{}}

	partes := []repertorio.Parte{
		{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(60)},
		{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(20)},
		{TitularID: "t1", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(20)},
	}

	if err := d.exigirPuedenRecibirReparto(t.Context(), partes); err != nil {
		t.Fatalf("exigirPuedenRecibirReparto: %v", err)
	}
	if got := padron.filtroRecibido.IDs; !slices.Equal(got, []string{"t2", "t1"}) {
		t.Fatalf("ids = %v, se esperaba [t2 t1]", got)
	}
	if padron.filtroRecibido.Limite != 2 {
		t.Fatalf("Limite = %d, se esperaba 2", padron.filtroRecibido.Limite)
	}
}

// Un fallo al leer el padron NO es un rechazo de la regla. Si se confundieran,
// el adaptador HTTP diria 400 -"tus datos estan mal"- cuando lo que esta roto
// es el sistema, y ademas dejaria la declaracion sin guardar por un fallo
// transitorio.
func TestGuardarSplitsNoEscribeSiElPadronNoResponde(t *testing.T) {
	fallo := errors.New("la base no responde")
	gestion := &gestionFalsa{}
	padron := &padronFalso{err: fallo}
	d := Declaraciones{Gestion: gestion, Padron: padron, Reloj: relojFijo{}}

	_, err := d.GuardarSplits(t.Context(), "obra-1", partesValidas(), "usr-admin")
	if !errors.Is(err, fallo) {
		t.Fatalf("se esperaba el fallo del padron, se obtuvo %v", err)
	}
	if errors.Is(err, ErrTitularNoEsPersonaNatural) || errors.Is(err, ErrTitularInexistente) ||
		errors.Is(err, ErrIPIQueNoCuadra) {
		t.Fatalf("un fallo de infraestructura salio como rechazo de la regla: %v", err)
	}
	if gestion.guardadas != 0 {
		t.Fatal("se guardo la declaracion sin haber podido comprobar quien puede recibir reparto")
	}
}

// Un titular_id que no esta en el padron NO es "no es persona natural": el
// rechazo de R-01 solo cubre a quien existe en el padron y no puede recibir
// reparto. La ausencia la delata la clave foranea al escribir -es lo que el
// adaptador de Postgres traduce a ErrTitularInexistente-, asi que aqui la
// escritura SI tiene que llegar al puerto.
//
// El doble no devuelve ninguna fila, que es lo que devolveria la base: la
// consulta pide un id que no existe, y esa consulta no puede traer filas que
// nadie pidio.
func TestGuardarSplitsDejaElTitularInexistenteAlEscribir(t *testing.T) {
	gestion := &gestionFalsa{err: ErrTitularInexistente}
	padron := &padronFalso{}
	d := Declaraciones{Gestion: gestion, Padron: padron, Reloj: relojFijo{}}

	partes := []repertorio.Parte{
		{TitularID: "t-typo", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(100)},
	}

	_, err := d.GuardarSplits(t.Context(), "obra-1", partes, "usr-admin")
	if !errors.Is(err, ErrTitularInexistente) {
		t.Fatalf("se esperaba ErrTitularInexistente, se obtuvo %v", err)
	}
	if errors.Is(err, ErrTitularNoEsPersonaNatural) || errors.Is(err, ErrIPIQueNoCuadra) {
		t.Fatalf("un titular inexistente salio como si el padron lo hubiera rechazado: %v", err)
	}
	// Se pregunto por el id que la declaracion nombra, y por ninguno mas: un
	// padron que no tiene esa fila responde con la lista vacia que se acaba de
	// ver, no con un error.
	if got := padron.filtroRecibido.IDs; !slices.Equal(got, []string{"t-typo"}) {
		t.Fatalf("ids = %v, se esperaba [t-typo]", got)
	}
	if gestion.guardadas != 1 {
		t.Fatal("no se llego a escribir: la ausencia la decide la clave foranea, no este caso de uso")
	}
}

func TestHistorialPasaAlPuerto(t *testing.T) {
	quiero := []VersionDeclaracion{{Version: 1}, {Version: 2}}
	gestion := &gestionFalsa{historial: quiero}
	d := Declaraciones{Gestion: gestion, Reloj: relojFijo{}}

	got, err := d.Historial(t.Context(), "obra-1")
	if err != nil {
		t.Fatalf("Historial: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("historial = %+v", got)
	}
}

func TestVigenteEnPasaElMomento(t *testing.T) {
	momento := time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)
	quiero := VersionDeclaracion{Version: 3}
	gestion := &gestionFalsa{vigente: quiero}
	d := Declaraciones{Gestion: gestion, Reloj: relojFijo{}}

	got, err := d.VigenteEn(t.Context(), "obra-1", momento)
	if err != nil {
		t.Fatalf("VigenteEn: %v", err)
	}
	if got.Version != 3 {
		t.Fatalf("version = %d", got.Version)
	}
}

// Un IPI declarado con espacios sobrantes es el mismo IPI que el del padron: el
// padron guarda recortado y la declaracion se normaliza al entrar.
func TestGuardarSplitsConciliaElIPIIgnorandoEspaciosSobrantes(t *testing.T) {
	gestion := &gestionFalsa{versionADevolver: 1}
	ana := titularDePrueba(t, "t1", "Ana Escritora", "IPI-00000001", true, afiliacion.ClaseSocio)
	padron := &padronFalso{titulares: []afiliacion.Titular{ana}}
	d := Declaraciones{Gestion: gestion, Padron: padron, Reloj: relojFijo{}}

	partes := []repertorio.Parte{
		{TitularID: "t1", IPI: "IPI-00000001 ", Porcentaje: decimal.NewFromInt(100)},
	}
	if _, err := d.GuardarSplits(t.Context(), "obra-1", partes, "usr-admin"); err != nil {
		t.Fatalf("GuardarSplits con IPI con espacio sobrante: %v", err)
	}
	if got := gestion.declRecibida.Partes[0].IPI; got != "IPI-00000001" {
		t.Fatalf("se persistio el IPI %q, se esperaba recortado", got)
	}
}

// Un titular_id con espacios no puede esquivar R-01: la consulta al padron
// lleva el id recortado, vuelve la fila de la sociedad y se rechaza.
func TestGuardarSplitsAplicaR01AUnTitularIDConEspacios(t *testing.T) {
	gestion := &gestionFalsa{}
	padron := &padronFalso{titulares: []afiliacion.Titular{sociedadDelPadron(t)}}
	d := Declaraciones{Gestion: gestion, Padron: padron, Reloj: relojFijo{}}

	partes := []repertorio.Parte{
		{TitularID: " t2 ", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(100)},
	}
	_, err := d.GuardarSplits(t.Context(), "obra-1", partes, "usr-admin")
	if !errors.Is(err, ErrTitularNoEsPersonaNatural) {
		t.Fatalf("se esperaba ErrTitularNoEsPersonaNatural, se obtuvo %v", err)
	}
	if !slices.Equal(padron.filtroRecibido.IDs, []string{"t2"}) {
		t.Fatalf("ids consultados = %q, se esperaba [t2] recortado", padron.filtroRecibido.IDs)
	}
	if gestion.guardadas != 0 {
		t.Fatal("se guardo una declaracion con una sociedad dentro")
	}
}
