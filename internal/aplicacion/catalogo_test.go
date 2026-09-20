package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// catalogoFalso cuenta cuantas veces le tocaron la base. Es lo que hace
// comprobable que la validacion corre ANTES y no despues.
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
	return nil, c.err
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

// catalogoDePrueba cablea los cuatro puertos como lo hace cmd/api.
func catalogoDePrueba(repo *catalogoFalso, libro *bitacoraFalsa, unidad *unidadFalsa) Catalogo {
	return Catalogo{
		Obras:    repo,
		Bitacora: libro,
		Unidad:   unidad,
		Reloj:    relojFijo{instante: instanteDePrueba},
	}
}

// cableado es el atajo para las pruebas a las que el asiento les da igual.
func cableado(repo *catalogoFalso) Catalogo {
	return catalogoDePrueba(repo, &bitacoraFalsa{}, &unidadFalsa{})
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
func TestRegistrarObraConActorVacioFalla(t *testing.T) {
	repo := &catalogoFalso{}
	libro, unidad := &bitacoraFalsa{}, &unidadFalsa{}

	_, err := catalogoDePrueba(repo, libro, unidad).
		RegistrarObra(t.Context(), "obra-1", metadatosValidos(), "")
	if err == nil {
		t.Fatal("se esperaba un error con actorID vacio")
	}
	if len(libro.asientos) != 0 {
		t.Fatalf("se asento un hecho sin actor: %+v", libro.asientos)
	}
	if unidad.confirmo {
		t.Fatal("la unidad se confirmo con un asiento sin firmar")
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
