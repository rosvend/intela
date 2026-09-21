package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// catalogoFalso cuenta cuantas veces le tocaron la base. Es lo que hace
// comprobable que la validacion corre ANTES y no despues.
//
// Satisface los dos puertos que el caso de uso inyecta. Antes llevaba
// [GestionDeclaraciones] embebida en nil -cuatro metodos, y los tres que el
// catalogo no usa reventaban en cuanto se llamaran-, y eso ya no hace falta: el
// puerto del catalogo es `VigentesDeObras` y nada mas (item 5), asi que el
// doble no puede implementar de mas ni aunque quiera.
type catalogoFalso struct {
	registros      int
	actualizadas   int
	obraRecibida   repertorio.Obra
	filtroRecibido FiltroObras
	err            error

	// anterior es lo que devuelve PorID: el estado que la actualizacion
	// sustituye, y por tanto el "antes" que tiene que salir en el asiento.
	anterior      repertorio.Obra
	errPorID      error
	errAlEscribir error

	// bloqueos cuenta las llamadas a Bloquear, y errBloquear deja simular que
	// la obra no existe en ese primer paso, antes de llegar a PorID.
	bloqueos    int
	errBloquear error

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

func (c *catalogoFalso) Bloquear(_ context.Context, _ string) error {
	c.bloqueos++
	return c.errBloquear
}

func (c *catalogoFalso) Registrar(_ context.Context, o repertorio.Obra) error {
	c.registros++
	c.obraRecibida = o
	if c.errAlEscribir != nil {
		return c.errAlEscribir
	}
	return c.err
}

func (c *catalogoFalso) Actualizar(_ context.Context, o repertorio.Obra) error {
	c.actualizadas++
	c.obraRecibida = o
	if c.errAlEscribir != nil {
		return c.errAlEscribir
	}
	return c.err
}

func (c *catalogoFalso) PorID(_ context.Context, _ string) (repertorio.Obra, error) {
	if c.errPorID != nil {
		return repertorio.Obra{}, c.errPorID
	}
	if c.anterior.ID() != "" {
		return c.anterior, nil
	}
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

// bitacoraFalsa guarda lo que se asienta. El error configurable es lo que hace
// comprobable la regla del ADR 0006: si el asiento falla, el caso de uso falla.
type bitacoraFalsa struct {
	asientos []Asiento
	err      error
}

func (b *bitacoraFalsa) Asentar(_ context.Context, a Asiento) error {
	if b.err != nil {
		return b.err
	}
	b.asientos = append(b.asientos, a)
	return nil
}

func (b *bitacoraFalsa) De(_ context.Context, refTipo, refID string) ([]Asiento, error) {
	if b.err != nil {
		return nil, b.err
	}
	var out []Asiento
	for _, a := range b.asientos {
		if a.RefTipo == refTipo && a.RefID == refID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (b *bitacoraFalsa) AsientoPorID(_ context.Context, id string) (Asiento, error) {
	for _, a := range b.asientos {
		if a.ID == id {
			return a, nil
		}
	}
	return Asiento{}, ErrNoEncontrado
}

// unidadFalsa corre fn tal cual y APUNTA si termino bien. No puede revertir
// nada -no hay base que revertir sin Postgres-, pero si deja comprobar la
// unica decision que es del nucleo: que el error del asiento llega hasta el
// limite en vez de tragarse, que es lo que en el adaptador dispara el
// rollback. Que el rollback de verdad ocurra se prueba contra Postgres, en
// postgres/catalogo_auditoria_test.go.
type unidadFalsa struct {
	entradas int
	confirmo bool
}

func (u *unidadFalsa) EnUnidad(ctx context.Context, fn func(context.Context) error) error {
	u.entradas++
	if err := fn(ctx); err != nil {
		return err
	}
	u.confirmo = true
	return nil
}

// instanteDePrueba es el `cuando` de todo asiento de este fichero. Fijo y no
// time.Now(): el instante entra por el puerto Reloj (ADR 0002) justamente para
// que se pueda comprobar.
var instanteDePrueba = time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC)

// catalogoDePrueba cablea los cinco puertos como lo hace cmd/api: los tres de
// escritura y el reloj sobre el mismo doble de Obras, y el mismo doble
// tambien satisface [LectorDeDeclaraciones] -como hace cada `main` con el
// *Store-, asi que el nucleo sigue viendo interfaces distintas aunque el
// adaptador sea uno solo.
func catalogoDePrueba(repo *catalogoFalso, libro *bitacoraFalsa, unidad *unidadFalsa) Catalogo {
	return Catalogo{
		Obras:         repo,
		Bitacora:      libro,
		Unidad:        unidad,
		Reloj:         relojFijo{instante: instanteDePrueba},
		Declaraciones: repo,
	}
}

// cableado es el atajo para las pruebas a las que el asiento y la declaracion
// les dan igual.
func cableado(repo *catalogoFalso) Catalogo {
	return catalogoDePrueba(repo, &bitacoraFalsa{}, &unidadFalsa{})
}

// La ganancia del item 5, como asercion de compilacion: lo que el catalogo ve
// del padron de declaraciones es un lector, y no tiene `Guardar`. Si alguien le
// devuelve un metodo de escritura al campo `Declaraciones`, esto deja de
// compilar -que es justo lo que un comentario no consigue-.
var _ LectorDeDeclaraciones = (*catalogoFalso)(nil)

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
	cat := cableado(repo)

	obra, err := cat.RegistrarObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
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

			_, err := cableado(repo).RegistrarObra(t.Context(), "obra-1", m, actorDePrueba)
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

	_, err := cableado(repo).RegistrarObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
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

	_, err := cableado(repo).ActualizarMetadatosObra(t.Context(), "obra-1", m, actorDePrueba)
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

	obra, err := cableado(repo).ActualizarMetadatosObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
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

	if _, err := cableado(repo).BuscarObras(t.Context(), quiero); err != nil {
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

	if _, err := cableado(repo).BuscarObras(t.Context(), FiltroObras{}); err != nil {
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

	if _, err := cableado(repo).BuscarObras(t.Context(), quiero); err != nil {
		t.Fatalf("BuscarObras: %v", err)
	}
	if repo.filtroRecibido.Limite != LimiteSinTope {
		t.Fatalf("Limite = %d, se esperaba LimiteSinTope (%d)",
			repo.filtroRecibido.Limite, LimiteSinTope)
	}
}

// ---------------------------------------------------------------------------
// El asiento en bitacora (ADR 0006, issue #91)
//
// Lo que se comprueba aqui es la parte que es del NUCLEO: que se asienta, con
// que hecho, con que actor y con que payload, y que un asiento que falla
// tumba el caso de uso. Que el rollback deshaga de verdad la escritura es del
// adaptador y se prueba contra Postgres real, en
// postgres/catalogo_auditoria_test.go.

const actorDePrueba = "usr-admin"

// asientoObraDe deserializa el payload de un asiento del catalogo.
func asientoObraDe(t *testing.T, a Asiento) AsientoObra {
	t.Helper()

	var p AsientoObra
	if err := json.Unmarshal(a.Payload, &p); err != nil {
		t.Fatalf("el payload del asiento %q no es JSON: %v", a.Hecho, err)
	}
	return p
}

func TestRegistrarObraAsientaElAltaDentroDeLaUnidad(t *testing.T) {
	repo, libro, unidad := &catalogoFalso{}, &bitacoraFalsa{}, &unidadFalsa{}

	obra, err := catalogoDePrueba(repo, libro, unidad).
		RegistrarObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
	if err != nil {
		t.Fatalf("RegistrarObra: %v", err)
	}

	if unidad.entradas != 1 || !unidad.confirmo {
		t.Fatalf("unidad: %d entradas, confirmo = %v; se esperaba una sola, confirmada",
			unidad.entradas, unidad.confirmo)
	}
	if len(libro.asientos) != 1 {
		t.Fatalf("se esperaba 1 asiento, hubo %d", len(libro.asientos))
	}

	a := libro.asientos[0]
	if a.Hecho != HechoObraRegistrada {
		t.Fatalf("hecho = %q, se esperaba %q", a.Hecho, HechoObraRegistrada)
	}
	if a.RefTipo != RefObra || a.RefID != "obra-1" {
		t.Fatalf("referencia = %s/%s, se esperaba %s/obra-1", a.RefTipo, a.RefID, RefObra)
	}
	if a.ActorID != actorDePrueba {
		t.Fatalf("actor = %q, se esperaba %q", a.ActorID, actorDePrueba)
	}
	// El instante sale del puerto Reloj y no de time.Now() dentro del caso de
	// uso: si saliera de ahi, esta comprobacion no se podria escribir.
	if !a.Cuando.Equal(instanteDePrueba) {
		t.Fatalf("cuando = %s, se esperaba %s", a.Cuando, instanteDePrueba)
	}

	p := asientoObraDe(t, a)
	if p.Antes != nil {
		t.Fatalf("un alta no tiene estado anterior: %+v", p.Antes)
	}
	if p.Despues.Titulo != obra.Metadatos().Titulo || p.Despues.Anio != obra.Metadatos().Anio {
		t.Fatalf("payload.despues = %+v", p.Despues)
	}
	if len(p.Despues.Coautores) != 1 || p.Despues.Coautores[0].IPI != "IPI-00000001" {
		t.Fatalf("payload.despues.coautores = %+v", p.Despues.Coautores)
	}
}

// La regla del ADR 0006 que este issue vino a cumplir: el error de Asentar NO
// se descarta. Si se descartara, la unidad confirmaria y quedaria una obra
// escrita sin rastro -- y es la ultima llamada del caso de uso, asi que nada
// mas lo delataria.
func TestRegistrarObraFallaSiElAsientoFalla(t *testing.T) {
	seCayoElLibro := errors.New("la bitacora no responde")
	repo := &catalogoFalso{}
	libro := &bitacoraFalsa{err: seCayoElLibro}
	unidad := &unidadFalsa{}

	_, err := catalogoDePrueba(repo, libro, unidad).
		RegistrarObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
	if !errors.Is(err, seCayoElLibro) {
		t.Fatalf("se esperaba el error del asiento, se obtuvo %v", err)
	}
	if unidad.confirmo {
		t.Fatal("la unidad se confirmo con el asiento fallido: la obra quedaria huerfana")
	}
}

func TestActualizarMetadatosObraFallaSiElAsientoFalla(t *testing.T) {
	seCayoElLibro := errors.New("la bitacora no responde")
	repo := &catalogoFalso{anterior: obraDePrueba(t, metadatosValidos())}
	libro := &bitacoraFalsa{err: seCayoElLibro}
	unidad := &unidadFalsa{}

	_, err := catalogoDePrueba(repo, libro, unidad).
		ActualizarMetadatosObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
	if !errors.Is(err, seCayoElLibro) {
		t.Fatalf("se esperaba el error del asiento, se obtuvo %v", err)
	}
	if unidad.confirmo {
		t.Fatal("la unidad se confirmo con el asiento fallido")
	}
}

// El criterio de aceptacion 3: del asiento de una correccion se tiene que
// poder reconstruir QUE cambio. Con el bloque reemplazado entero, el estado
// anterior no queda en ninguna tabla; si no esta en el payload no esta en
// ningun sitio.
func TestActualizarMetadatosObraAsientaAntesDespuesYCambios(t *testing.T) {
	antes := metadatosValidos()
	repo := &catalogoFalso{anterior: obraDePrueba(t, antes)}
	libro, unidad := &bitacoraFalsa{}, &unidadFalsa{}

	despues := metadatosValidos()
	despues.Titulo = "La Casa de las Dos Palmas (restaurada)"
	despues.Anio = 1992
	despues.Coautores = append(despues.Coautores,
		repertorio.Coautor{Nombre: "Beto Libretista", IPI: "IPI-00000002", Rol: repertorio.RolLibretista})

	if _, err := (catalogoDePrueba(repo, libro, unidad)).
		ActualizarMetadatosObra(t.Context(), "obra-1", despues, actorDePrueba); err != nil {
		t.Fatalf("ActualizarMetadatosObra: %v", err)
	}

	if len(libro.asientos) != 1 {
		t.Fatalf("se esperaba 1 asiento, hubo %d", len(libro.asientos))
	}
	a := libro.asientos[0]
	if a.Hecho != HechoObraCorregida {
		t.Fatalf("hecho = %q, se esperaba %q", a.Hecho, HechoObraCorregida)
	}

	p := asientoObraDe(t, a)
	if p.Antes == nil {
		t.Fatal("una correccion sin estado anterior no explica nada")
	}
	if p.Antes.Titulo != antes.Titulo || p.Antes.Anio != antes.Anio {
		t.Fatalf("payload.antes = %+v, se esperaba el estado que se sustituyo", *p.Antes)
	}
	if p.Despues.Titulo != despues.Titulo || p.Despues.Anio != despues.Anio {
		t.Fatalf("payload.despues = %+v", p.Despues)
	}
	quiero := []string{"titulo", "anio", "coautores"}
	if !slices.Equal(p.Cambios, quiero) {
		t.Fatalf("payload.cambios = %v, se esperaba %v", p.Cambios, quiero)
	}
}

// Un PATCH que manda exactamente lo mismo deja su asiento -- la bitacora es
// append-only y un hecho ocurrido dos veces deja dos asientos -- pero no
// inventa un cambio que no hubo. El orden de los coautores no cuenta como
// cambio: el cuerpo HTTP los manda en el orden que quiere.
func TestActualizarMetadatosObraSinCambiosNoInventaNinguno(t *testing.T) {
	antes := metadatosValidos()
	antes.Coautores = append(antes.Coautores,
		repertorio.Coautor{Nombre: "Beto Libretista", IPI: "IPI-00000002", Rol: repertorio.RolLibretista})

	repo := &catalogoFalso{anterior: obraDePrueba(t, antes)}
	libro, unidad := &bitacoraFalsa{}, &unidadFalsa{}

	mismos := metadatosValidos()
	mismos.Coautores = []repertorio.Coautor{
		{Nombre: "Beto Libretista", IPI: "IPI-00000002", Rol: repertorio.RolLibretista},
		{Nombre: "Ana Escritora", IPI: "IPI-00000001", Rol: repertorio.RolGuionista},
	}

	if _, err := (catalogoDePrueba(repo, libro, unidad)).
		ActualizarMetadatosObra(t.Context(), "obra-1", mismos, actorDePrueba); err != nil {
		t.Fatalf("ActualizarMetadatosObra: %v", err)
	}
	if len(libro.asientos) != 1 {
		t.Fatalf("se esperaba 1 asiento, hubo %d", len(libro.asientos))
	}
	if p := asientoObraDe(t, libro.asientos[0]); len(p.Cambios) != 0 {
		t.Fatalf("payload.cambios = %v, se esperaba ninguno", p.Cambios)
	}
}

// Una obra que no esta en el catalogo no se actualiza ni se asienta: sin
// estado anterior no hay correccion que explicar, y un PATCH que insertara
// convertiria un id mal escrito en una obra fantasma.
func TestActualizarMetadatosObraInexistenteNoAsienta(t *testing.T) {
	repo := &catalogoFalso{errPorID: ErrNoEncontrado}
	libro, unidad := &bitacoraFalsa{}, &unidadFalsa{}

	_, err := catalogoDePrueba(repo, libro, unidad).
		ActualizarMetadatosObra(t.Context(), "obra-fantasma", metadatosValidos(), actorDePrueba)
	if !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
	if repo.actualizadas != 0 {
		t.Fatal("se escribio una obra que no estaba en el catalogo")
	}
	if len(libro.asientos) != 0 {
		t.Fatalf("no se esperaba ningun asiento: %+v", libro.asientos)
	}
}

// Si la escritura falla, no se asienta: el asiento describiria un hecho que no
// ocurrio.
func TestRegistrarObraNoAsientaSiLaEscrituraFalla(t *testing.T) {
	repo := &catalogoFalso{errAlEscribir: ErrObraDuplicada}
	libro, unidad := &bitacoraFalsa{}, &unidadFalsa{}

	_, err := catalogoDePrueba(repo, libro, unidad).
		RegistrarObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
	if !errors.Is(err, ErrObraDuplicada) {
		t.Fatalf("se esperaba ErrObraDuplicada, se obtuvo %v", err)
	}
	if len(libro.asientos) != 0 {
		t.Fatalf("se asento un alta que no ocurrio: %+v", libro.asientos)
	}
	if unidad.confirmo {
		t.Fatal("la unidad se confirmo con la escritura fallida")
	}
}

// ---------------------------------------------------------------------------
// El cerrojo antes de reconstruir (bloqueante 4)

// ActualizarMetadatosObra tiene que bloquear la fila ANTES de leerla: es lo
// que impide que la lectura adelante el commit de un PATCH concurrente sobre
// la misma obra (ver el comentario del metodo en catalogo.go). El doble no
// puede probar la serializacion de verdad -eso es
// TestActualizarMetadatosObraConcurrenteAsientaLaCadenaCompleta, contra
// Postgres, en postgres/catalogo_auditoria_test.go-, pero si puede probar que
// el caso de uso PIDE el cerrojo, y que lo pide antes que la lectura.
func TestActualizarMetadatosObraBloqueaAntesDeLeer(t *testing.T) {
	repo := &catalogoFalso{anterior: obraDePrueba(t, metadatosValidos())}
	cat := cableado(repo)

	if _, err := cat.ActualizarMetadatosObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba); err != nil {
		t.Fatalf("ActualizarMetadatosObra: %v", err)
	}
	if repo.bloqueos != 1 {
		t.Fatalf("se esperaba 1 llamada a Bloquear, hubo %d", repo.bloqueos)
	}
}

// Si Bloquear falla -tipicamente porque la obra no existe-, ni PorID ni
// Actualizar se llegan a intentar: el caso de uso falla en el primer paso.
func TestActualizarMetadatosObraNoLeeSiBloquearFalla(t *testing.T) {
	repo := &catalogoFalso{errBloquear: ErrNoEncontrado, errPorID: errors.New("PorID no deberia llamarse")}
	cat := cableado(repo)

	_, err := cat.ActualizarMetadatosObra(t.Context(), "obra-fantasma", metadatosValidos(), actorDePrueba)
	if !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
	if repo.actualizadas != 0 {
		t.Fatal("se escribio una obra cuyo cerrojo fallo")
	}
}

// ---------------------------------------------------------------------------
// Guardas del caso de uso

// Un Catalogo cableado a medias -como el Catalogo{Obras: store} que arma
// semilla/cargar_test.go para las dos lecturas que no necesitan nada mas-
// tiene que fallar con un error legible en cuanto se intenta ESCRIBIR con
// cualquiera de las cuatro dependencias ausente, no con un nil pointer
// dereference. Las cuatro por separado: enUnidad las comprueba todas antes
// de abrir la unidad (ver su comentario en catalogo.go), y antes solo
// comprobaba Unidad -un Catalogo sin Reloj paniqueaba igual, mas tarde y con
// un mensaje que apuntaba a la dependencia equivocada.
func TestEscrituraConDependenciaFaltanteFallaSinPanicar(t *testing.T) {
	completo := func() Catalogo {
		return Catalogo{
			Obras:    &catalogoFalso{anterior: obraDePrueba(t, metadatosValidos())},
			Bitacora: &bitacoraFalsa{},
			Unidad:   &unidadFalsa{},
			Reloj:    relojFijo{instante: instanteDePrueba},
		}
	}
	casos := map[string]func(*Catalogo){
		"sin Obras":    func(c *Catalogo) { c.Obras = nil },
		"sin Bitacora": func(c *Catalogo) { c.Bitacora = nil },
		"sin Unidad":   func(c *Catalogo) { c.Unidad = nil },
		"sin Reloj":    func(c *Catalogo) { c.Reloj = nil },
	}

	for nombre, romper := range casos {
		t.Run(nombre, func(t *testing.T) {
			cat := completo()
			romper(&cat)

			if err := sinPanic(t, func() error {
				_, err := cat.RegistrarObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
				return err
			}); err == nil {
				t.Fatal("RegistrarObra: se esperaba un error, no nil")
			}
			if err := sinPanic(t, func() error {
				_, err := cat.ActualizarMetadatosObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
				return err
			}); err == nil {
				t.Fatal("ActualizarMetadatosObra: se esperaba un error, no nil")
			}
		})
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

	obras, err := cableado(repo).BuscarObras(t.Context(), FiltroObras{})
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

	obras, err := cableado(repo).BuscarObras(t.Context(), FiltroObras{})
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

	obras, err := cableado(repo).BuscarObras(t.Context(), FiltroObras{})
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
				return cableado(repo).RegistrarObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
			},
			estado: "incompleta", suma: decimal.Zero, version: nil, consultas: 0,
		},
		"ActualizarMetadatosObra": {
			llamar: func(_ *testing.T, repo *catalogoFalso) (ObraDelCatalogo, error) {
				return cableado(repo).ActualizarMetadatosObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
			},
			estado: "completa", suma: decimal.NewFromInt(100), version: &tres, consultas: 1,
		},
		"ObraPorID": {
			llamar: func(_ *testing.T, repo *catalogoFalso) (ObraDelCatalogo, error) {
				return cableado(repo).ObraPorID(t.Context(), "obra-1")
			},
			estado: "completa", suma: decimal.NewFromInt(100), version: &tres, consultas: 1,
		},
		"BuscarObras": {
			llamar: func(t *testing.T, repo *catalogoFalso) (ObraDelCatalogo, error) {
				obras, err := cableado(repo).BuscarObras(t.Context(), FiltroObras{})
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

// sinPanic ejecuta fn y CONVIERTE cualquier panic en un t.Fatal legible, en
// vez de dejar que tumbe el binario de pruebas entero: es lo que hace que
// una regresion a "vuelve a paniquear" se vea como el fallo de ESTA prueba y
// no como un cuelgue de todo el paquete.
func sinPanic(t *testing.T, fn func() error) error {
	t.Helper()
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic: %v", r)
			}
		}()
		err = fn()
	}()
	return err
}

// Un actorID vacio no puede producir un asiento sin firmar: el caso de uso es
// quien sostiene el contrato del ADR 0006, no el adaptador HTTP que lo llame
// antes (bitacora.go usa NULLIF sobre la cadena vacia y actor_id es nullable
// en la base).
// Un actor de solo espacios cuenta como vacio: el NULLIF del adaptador solo
// casa con la cadena vacia, asi que " " se escribiria como actor literal y
// solo lo pararia la clave foranea contra `usuarios`, ya como un 500 generico.
func TestRegistrarObraConActorVacioFalla(t *testing.T) {
	for nombre, actor := range map[string]string{"vacio": "", "solo espacios": "  "} {
		t.Run(nombre, func(t *testing.T) {
			repo := &catalogoFalso{}
			libro, unidad := &bitacoraFalsa{}, &unidadFalsa{}

			_, err := catalogoDePrueba(repo, libro, unidad).
				RegistrarObra(t.Context(), "obra-1", metadatosValidos(), actor)
			if !errors.Is(err, ErrActorAusente) {
				t.Fatalf("err = %v, se esperaba ErrActorAusente", err)
			}
			if len(libro.asientos) != 0 {
				t.Fatalf("se asento un hecho sin actor: %+v", libro.asientos)
			}
			if unidad.confirmo {
				t.Fatal("la unidad se confirmo con un asiento sin firmar")
			}
		})
	}
}

func TestActualizarMetadatosObraConActorVacioFalla(t *testing.T) {
	repo := &catalogoFalso{anterior: obraDePrueba(t, metadatosValidos())}
	libro, unidad := &bitacoraFalsa{}, &unidadFalsa{}

	_, err := catalogoDePrueba(repo, libro, unidad).
		ActualizarMetadatosObra(t.Context(), "obra-1", metadatosValidos(), "")
	if err == nil {
		t.Fatal("se esperaba un error con actorID vacio")
	}
	if len(libro.asientos) != 0 {
		t.Fatalf("se asento un hecho sin actor: %+v", libro.asientos)
	}
}

// obraDePrueba construye la entidad por el constructor del dominio.
func obraDePrueba(t *testing.T, m repertorio.Metadatos) repertorio.Obra {
	t.Helper()

	o, err := repertorio.NuevaObra("obra-1", m)
	if err != nil {
		t.Fatalf("construir la obra de prueba: %v", err)
	}
	return o
}

// ---------------------------------------------------------------------------
// El alta compone el estado localmente en vez de releerlo (item 2), y esa
// composicion tiene que dar LO MISMO que el camino largo: si divergiera, la
// respuesta del alta diria del estado de la obra recien creada algo distinto de
// lo que dice el listado de esa misma obra un instante despues.
func TestElAltaComponeElMismoEstadoQueLaLectura(t *testing.T) {
	repo := &catalogoFalso{}
	cat := cableado(repo)

	alta, err := cat.RegistrarObra(t.Context(), "obra-1", metadatosValidos(), actorDePrueba)
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

	if _, err := cableado(repo).BuscarObras(t.Context(), FiltroObras{}); err == nil {
		t.Fatal("se esperaba el error del puerto de declaraciones")
	}
}
