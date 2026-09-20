package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Hechos que el ABM del catalogo asienta en la bitacora (ADR 0006).
//
// Son constantes y no literales sueltos en la llamada porque el hecho es la
// clave por la que se consulta el libro -- hay un indice `asientos_hecho` sobre
// la columna -- y una errata en el literal no rompe nada hoy: deja un asiento
// que ninguna consulta encuentra dentro de diez anos.
const (
	// HechoObraRegistrada es el alta de una obra en el catalogo maestro.
	HechoObraRegistrada = "obra.registrada"
	// HechoObraCorregida es la correccion del bloque de metadatos.
	HechoObraCorregida = "obra.metadatos_corregidos"
)

// RefObra es el tipo de referencia de los asientos del catalogo: es la mitad
// con la que [BitacoraAuditoria.De] recupera la historia de una obra.
//
// La comparte con `declaracion.guardada`, que referencia la misma obra: De(ctx,
// RefObra, id) devuelve la historia ENTERA de esa obra -- alta, correcciones y
// declaraciones -- en el orden en que ocurrio, que es lo que un auditor lee.
const RefObra = "obra"

// Catalogo son los casos de uso del catalogo maestro de obras: el cubo contra
// el que resuelve todo matching (docs/dominio/identificadores.md).
//
// Un servicio con tres operaciones y no tres structs, por lo mismo que
// [Autenticacion]: giran sobre el mismo agregado y comparten las dependencias.
// La segregacion del ADR 0003 se conserva donde importa -lo que se inyecta son
// puertos estrechos, no un repositorio que lo sepa todo-.
//
// # Por que la bitacora es un puerto aparte
//
// [Declaraciones] no lleva [BitacoraAuditoria]: su asiento viaja dentro del
// contrato de [GestionDeclaraciones.Guardar]. Aqui NO, y es deliberado (issue
// #91): el ADR 0003 sostiene "ningun modulo escribe en la trazabilidad de
// otro" precisamente porque Asentar vive en un puerto propio; meterlo en
// [CatalogoObras] lo devolveria a un contrato compartido. El precio de tenerlo
// fuera es que el limite de transaccion hay que declararlo, y eso es lo que
// hace [UnidadDeTrabajo].
//
// # Lo que este servicio NO hace
//
// No reparte, no lee `declaraciones` y no toca dinero. Registrar una obra no
// crea derecho a cobrar: el derecho sale de la Declaracion de Obra (`R-03`),
// que entra por otro camino. Que los coautores del catalogo no lleven
// porcentaje es lo que impide construir aqui el segundo camino hasta un pago
// que `R-02` cierra.
type Catalogo struct {
	Obras    CatalogoObras
	Bitacora BitacoraAuditoria
	Unidad   UnidadDeTrabajo
	Reloj    Reloj
}

// RegistrarObra da de alta una obra en el catalogo y asienta el alta.
//
// El identificador lo trae quien llama y NO se genera aqui. Es el numero de
// obra de REDES-SYS, que se asigna fuera de este sistema, y es lo que hace que
// "un segundo alta con el mismo identificador se rechaza" sea una regla
// comprobable y no una imposibilidad de fabrica.
//
// La validacion es del dominio: [repertorio.NuevaObra] es la unica puerta, asi
// que no hay forma de que llegue al adaptador una obra sin titulo, sin genero,
// sin anio o sin un coautor con IPI. Corre ANTES de abrir la unidad: una obra
// que el dominio rechaza no merece una transaccion.
//
// La escritura y el asiento van en la MISMA unidad (ADR 0006): un alta
// confirmada cuyo asiento fallo no esta hecha, asi que las dos entran o no
// entra ninguna.
func (c Catalogo) RegistrarObra(ctx context.Context, id string, m repertorio.Metadatos, actorID string) (repertorio.Obra, error) {
	obra, err := repertorio.NuevaObra(id, m)
	if err != nil {
		return repertorio.Obra{}, err
	}

	err = c.enUnidad(ctx, func(ctx context.Context) error {
		if err := c.Obras.Registrar(ctx, obra); err != nil {
			// Sin envolver ErrObraDuplicada en un texto que lo tape: quien llama
			// lo distingue con errors.Is, y el adaptador ya le pone su contexto.
			return err
		}
		return c.asentar(ctx, HechoObraRegistrada, obra.ID(), actorID, AsientoObra{
			Despues: metadatosAsentados(obra),
		})
	})
	if err != nil {
		return repertorio.Obra{}, err
	}
	return obra, nil
}

// ActualizarMetadatosObra corrige los metadatos de una obra. Nunca su
// identificador.
//
// # Por que reemplaza el bloque entero y no campo a campo
//
// El identificador viaja por la ruta y los metadatos por el cuerpo, completos.
// Eso es lo que da la propiedad que pide el issue -el id no esta entre lo que
// se puede mandar, asi que no se puede cambiar ni por descuido- sin pagar el
// precio de un merge parcial: distinguir "el campo no vino" de "el campo vino
// vacio" obliga a punteros en cada campo y a un leer-modificar-escribir con su
// carrera. La invariante hay que revalidarla entera de todas formas, porque
// una obra sin genero no es una obra medio valida.
//
// # Por que se lee la obra antes de escribirla
//
// Porque el asiento tiene que dejar ver QUE CAMBIO y no solo que hubo un
// PATCH. Con el bloque reemplazado entero, un asiento que dijera "actualizada"
// no explica nada: el estado anterior ya no esta en ninguna tabla -- `obras` se
// sobreescribe y `obra_coautores` se borra y se reescribe --, asi que si no
// queda en el payload no queda en ningun sitio. La lectura va DENTRO de la
// unidad, con el mismo ctx, para que lo que se asienta como "antes" sea lo que
// esta transaccion sustituyo y no una version que ya habia cambiado.
//
// Devuelve ErrNoEncontrado si la obra no existe. No la crea: un PATCH que
// inserta convierte un id mal escrito en una obra fantasma del catalogo, y
// contra el catalogo resuelve todo el matching.
//
// # Por que se bloquea la fila antes de leerla
//
// Dos PATCH concurrentes sobre la MISMA obra, bajo READ COMMITTED: sin
// cerrojo, T2 podria leer con PorID el estado A mientras T1 todavia no
// confirma, T1 escribe B y confirma, y T2 escribe C encima de B asentando
// antes=A, despues=C. El tramo real A-a-B-a-C se pierde para siempre, porque
// el estado anterior no sobrevive en ninguna otra tabla (ver el comentario de
// arriba). [CatalogoObras.Bloquear] toma el cerrojo de fila ANTES de PorID:
// T2 se queda esperando el commit de T1 y solo entonces lee, asi que su
// "antes" es lo que esta transaccion de verdad sustituyo.
func (c Catalogo) ActualizarMetadatosObra(ctx context.Context, id string, m repertorio.Metadatos, actorID string) (repertorio.Obra, error) {
	// Se construye una obra completa y valida ANTES de tocar la base: es el
	// mismo constructor que el alta, asi que una obra corregida cumple lo
	// mismo que una recien creada.
	obra, err := repertorio.NuevaObra(id, m)
	if err != nil {
		return repertorio.Obra{}, err
	}

	err = c.enUnidad(ctx, func(ctx context.Context) error {
		if err := c.Obras.Bloquear(ctx, id); err != nil {
			return err
		}
		anterior, err := c.Obras.PorID(ctx, id)
		if err != nil {
			return err
		}
		if err := c.Obras.Actualizar(ctx, obra); err != nil {
			return err
		}
		antes, despues := metadatosAsentados(anterior), metadatosAsentados(obra)
		return c.asentar(ctx, HechoObraCorregida, obra.ID(), actorID, AsientoObra{
			Antes:   &antes,
			Despues: despues,
			Cambios: camposCambiados(antes, despues),
		})
	})
	if err != nil {
		return repertorio.Obra{}, err
	}
	return obra, nil
}

// asentar serializa el payload y lo escribe. El error de [BitacoraAuditoria]
// NO se descarta en ningun camino: el ADR 0006 declara el asiento "parte de la
// definicion de hecho de cada caso de uso", asi que un caso de uso cuyo
// asiento fallo no esta hecho, y devolverlo es lo que revierte la unidad.
//
// actorID vacio se rechaza aqui, no solo en el adaptador. [postgres.asentar]
// lo escribe con NULLIF sobre la cadena vacia, y `asientos.actor_id` es
// nullable -- un asiento sin firmar es una fila valida para la base, pero no
// para el ADR 0006, que
// exige saber QUIEN hizo cada hecho. Por HTTP nunca llega vacio (sale de la
// sesion, ver httpapi/obras.go), pero el contrato de este caso de uso es el
// que lo sostiene, no el adaptador que llame antes.
func (c Catalogo) asentar(ctx context.Context, hecho, obraID, actorID string, p AsientoObra) error {
	if actorID == "" {
		return fmt.Errorf("asentar %q sobre la obra %q: actorID vacio", hecho, obraID)
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("serializar el asiento de la obra %q: %w", obraID, err)
	}
	if err := c.Bitacora.Asentar(ctx, Asiento{
		Hecho:   hecho,
		RefTipo: RefObra,
		RefID:   obraID,
		ActorID: actorID,
		Payload: payload,
		// El instante entra por el puerto: el adaptador no llama a time.Now()
		// (ADR 0002), y asi una prueba puede fijar el `cuando` del asiento.
		Cuando: c.Reloj.Ahora(),
	}); err != nil {
		return fmt.Errorf("asentar %q sobre la obra %q: %w", hecho, obraID, err)
	}
	return nil
}

// enUnidad es [UnidadDeTrabajo.EnUnidad] con una guarda: un Catalogo cableado
// a medias -- cargar_test.go, en semilla, construye Catalogo{Obras: store} a
// proposito para las dos lecturas que no necesitan nada mas -- devuelve un
// error legible en vez de un nil pointer dereference en cuanto RegistrarObra
// o ActualizarMetadatosObra intenten escribir.
//
// Comprueba las CUATRO dependencias que un camino de escritura toca -Obras,
// Unidad, Bitacora y Reloj-, no solo Unidad: un
// Catalogo{Obras: ..., Unidad: ..., Bitacora: ...} sin Reloj paniqueaba
// igual, solo que mas tarde -- DENTRO de la transaccion ya abierta, porque
// [Catalogo.asentar] llama c.Reloj.Ahora() -- y con un mensaje ("catalogo mal
// cableado: falta UnidadDeTrabajo") que apuntaba a la dependencia que no era.
// Las cuatro se comprueban ANTES de abrir la unidad, asi que una escritura
// con cualquiera de las cuatro ausente falla limpio sin tocar la base.
func (c Catalogo) enUnidad(ctx context.Context, fn func(context.Context) error) error {
	switch {
	case c.Obras == nil:
		return errors.New("catalogo mal cableado: falta CatalogoObras")
	case c.Unidad == nil:
		return errors.New("catalogo mal cableado: falta UnidadDeTrabajo")
	case c.Bitacora == nil:
		return errors.New("catalogo mal cableado: falta BitacoraAuditoria")
	case c.Reloj == nil:
		return errors.New("catalogo mal cableado: falta Reloj")
	}
	return c.Unidad.EnUnidad(ctx, fn)
}

// HistorialObra devuelve los asientos de una obra, del mas antiguo al mas
// nuevo. Es la lectura del ADR 0006 sobre el catalogo: alta, correcciones y
// -- porque comparten [RefObra] -- tambien lo que asentaron sus declaraciones.
//
// Una obra sin asientos devuelve una lista vacia y ningun error, mismo
// criterio que [Catalogo.BuscarObras]: no haber pasado nada todavia no es un
// fallo.
func (c Catalogo) HistorialObra(ctx context.Context, obraID string) ([]Asiento, error) {
	asientos, err := c.Bitacora.De(ctx, RefObra, obraID)
	if err != nil {
		return nil, fmt.Errorf("historial de la obra %q: %w", obraID, err)
	}
	return asientos, nil
}

// ObraPorID devuelve una obra del catalogo, o ErrNoEncontrado.
func (c Catalogo) ObraPorID(ctx context.Context, id string) (repertorio.Obra, error) {
	// Sin envolver de nuevo: el adaptador ya nombra la obra y la operacion en
	// su propio error (ver [postgres.Store.PorID]). Hacerlo tambien aqui
	// duplicaba el "obra %q" -- una vez del caso de uso, otra del adaptador --
	// en el mismo mensaje sin anadir nada que errors.Is no pueda ver ya.
	return c.Obras.PorID(ctx, id)
}

// BuscarObras resuelve una consulta del catalogo.
//
// Un filtro vacio devuelve la primera pagina del catalogo: "sin recorte" de
// titulo/genero/IPI/anio sigue siendo un recorte mas, y la paginacion es el
// tope que evita servir el catalogo entero de REDES SGC de un golpe. El
// defecto vive aqui -no en cada adaptador- para que cualquier
// [CatalogoObras] lo herede y se pueda comprobar sin levantar Postgres.
func (c Catalogo) BuscarObras(ctx context.Context, f FiltroObras) ([]repertorio.Obra, error) {
	f.Paginacion = f.ConDefecto()
	obras, err := c.Obras.Buscar(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("buscar obras: %w", err)
	}
	return obras, nil
}

// ---------------------------------------------------------------------------
// La forma del payload
//
// Son tipos propios y no [repertorio.Metadatos] serializado directamente. Los
// modelos del dominio no llevan etiquetas json a proposito -- lo dice
// httpapi/obras.go sobre la forma de red, y vale igual aqui --, asi que
// marshalearlos escribiria los nombres de los CAMPOS DE GO en el libro: un
// `Coautores[].Rol` renombrado en el dominio cambiaria en silencio la forma de
// un registro que el ADR 0006 manda conservar diez anos y que tiene que seguir
// siendo legible por una persona al final de ese plazo.

// AsientoObra es el payload JSONB de los asientos del ABM del catalogo.
//
// Antes va nil en el alta -- no habia nada antes -- y con el bloque completo en
// la correccion. Guardar los dos bloques ENTEROS y no solo los campos que
// cambiaron es lo que permite reconstruir la obra tal como estaba sin
// recorrer la cadena desde el principio; Cambios es el resumen de lectura,
// no la fuente.
type AsientoObra struct {
	Antes   *MetadatosAsentados `json:"antes,omitempty"`
	Despues MetadatosAsentados  `json:"despues"`
	// Cambios nombra los campos que difieren, en orden fijo. Vacio en el alta.
	Cambios []string `json:"cambios,omitempty"`
}

// MetadatosAsentados es el bloque de metadatos de una obra tal como queda
// escrito en el libro.
type MetadatosAsentados struct {
	Titulo    string            `json:"titulo"`
	Genero    string            `json:"genero"`
	Anio      int               `json:"anio"`
	Tipo      string            `json:"tipo"`
	IDA       string            `json:"ida"`
	EIDR      string            `json:"eidr"`
	IMDB      string            `json:"imdb"`
	Coautores []CoautorAsentado `json:"coautores"`
}

// CoautorAsentado es un coautor del catalogo en el libro. Sin porcentaje, como
// en [repertorio.Coautor]: el catalogo es identidad, no reparto (`R-02`).
type CoautorAsentado struct {
	IPI    string `json:"ipi"`
	Nombre string `json:"nombre"`
	Rol    string `json:"rol"`
}

// metadatosAsentados traduce una obra a la forma del libro.
//
// Los coautores salen ORDENADOS por (IPI, rol), el mismo orden en que los
// devuelve la lectura del catalogo (ver lateralCoautores en
// postgres/catalogo.go). Sin ordenar, el bloque dependeria del orden en que
// quien llama los mando -- el de un cuerpo HTTP, que no significa nada -- y
// dos asientos identicos se verian distintos: el ADR 0005 exige que esto sea
// reproducible, y [camposCambiados] anunciaria un cambio de coautores en
// cada PATCH que no cambio ninguno. (IPI, rol) ya es clave unica por obra
// -repertorio.normalizarCoautores la exige-, asi que este orden es total y
// no hace falta un tercer campo de desempate.
func metadatosAsentados(o repertorio.Obra) MetadatosAsentados {
	m := o.Metadatos()
	coautores := make([]CoautorAsentado, 0, len(m.Coautores))
	for _, c := range m.Coautores {
		coautores = append(coautores, CoautorAsentado{
			IPI: c.IPI, Nombre: c.Nombre, Rol: string(c.Rol),
		})
	}
	slices.SortFunc(coautores, func(a, b CoautorAsentado) int {
		if n := strings.Compare(a.IPI, b.IPI); n != 0 {
			return n
		}
		return strings.Compare(a.Rol, b.Rol)
	})
	return MetadatosAsentados{
		Titulo:    m.Titulo,
		Genero:    m.Genero,
		Anio:      m.Anio,
		Tipo:      string(m.Tipo),
		IDA:       m.IDA,
		EIDR:      m.EIDR,
		IMDB:      m.IMDB,
		Coautores: coautores,
	}
}

// camposCambiados nombra lo que difiere entre dos bloques, en orden fijo.
//
// Es el resumen que hace legible el asiento de una correccion sin comparar dos
// objetos a ojo, y ademas es consultable: `asientos.payload` tiene un indice
// GIN, asi que "todas las correcciones que tocaron el titulo" es una consulta.
// Nil -- y no una lista vacia -- cuando no cambio nada, para que `omitempty` lo
// deje fuera del JSON: un PATCH idempotente deja su asiento, pero no finge un
// cambio que no hubo.
func camposCambiados(antes, despues MetadatosAsentados) []string {
	var cambios []string
	anotar := func(nombre string, distinto bool) {
		if distinto {
			cambios = append(cambios, nombre)
		}
	}
	anotar("titulo", antes.Titulo != despues.Titulo)
	anotar("genero", antes.Genero != despues.Genero)
	anotar("anio", antes.Anio != despues.Anio)
	anotar("tipo", antes.Tipo != despues.Tipo)
	anotar("ida", antes.IDA != despues.IDA)
	anotar("eidr", antes.EIDR != despues.EIDR)
	anotar("imdb", antes.IMDB != despues.IMDB)
	anotar("coautores", !slices.Equal(antes.Coautores, despues.Coautores))
	return cambios
}
