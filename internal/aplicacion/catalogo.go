package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/shopspring/decimal"

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

// LectorDeDeclaraciones es lo unico que [Catalogo] necesita de la gestion de
// declaraciones: la version vigente de un punado de obras.
//
// Se declara aqui, junto a quien la consume, y con UN solo metodo, por la misma
// razon que [PadronTitulares]: un puerto tiene que decir lo que su consumidor
// necesita y nada mas. Antes este campo era [GestionDeclaraciones] -cuatro
// metodos, uno de ellos de escritura-, y el catalogo podia llamar a `Guardar`:
// no lo hacia, pero eso solo lo sostenia un comentario, y quitarle el metodo al
// puerto convierte "no deberia escribir" en "no puede". Un `Guardar` que se
// cuele aqui deja de compilar, en vez de depender de que nadie lo escriba.
type LectorDeDeclaraciones interface {
	VigentesDeObras(ctx context.Context, obraIDs []string) (map[string]VersionDeclaracion, error)
}

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
// No reparte, no ESCRIBE `declaraciones` y no toca dinero. Registrar una obra
// no crea derecho a cobrar: el derecho sale de la Declaracion de Obra
// (`R-03`), que entra por otro camino. Que los coautores del catalogo no
// lleven porcentaje es lo que impide construir aqui el segundo camino hasta un
// pago que `R-02` cierra.
//
// Lo que si hace es LEER la declaracion vigente de las obras que sirve -por
// [LectorDeDeclaraciones], sin versiones de por medio-, porque el catalogo es
// donde un administrador ve que obras estan completas y que obras quedan
// retenidas. Los tres campos que salen de ahi son derivados y de solo lectura:
// el estado de una obra no se declara desde el catalogo.
type Catalogo struct {
	Obras    CatalogoObras
	Bitacora BitacoraAuditoria
	Unidad   UnidadDeTrabajo
	Reloj    Reloj

	// Declaraciones se lee, no se escribe desde aqui. Va aparte de [Obras] por
	// lo mismo que los dos puertos estan separados (ver [CatalogoObras]): la
	// obra y su declaracion son dos cosas distintas, y este servicio solo mira
	// la segunda para poder decir en que estado esta la primera.
	Declaraciones LectorDeDeclaraciones
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
//
// Devuelve la obra ya proyectada, con el estado de su declaracion, por el
// MISMO camino que las tres lecturas: [proyectarObra] sobre un mapa sin
// entradas devuelve exactamente lo de abajo, y por eso no hace falta releerlo.
//
// En un alta ese estado es el cero -`incompleta`, suma 0 y `version_vigente`
// nil-, y no por darlo por supuesto: una declaracion necesita la fila de
// `obras` -su clave foranea- y esta operacion es justo la que la crea, asi que
// no puede haber ninguna. Se compone aqui en vez de preguntarselo a la base
// porque preguntarselo es pedir lo que se acaba de escribir, y una lectura que
// falle despues de un alta que SI ocurrio se contesta como "no se pudo
// registrar la obra": un error sobre una escritura que ya esta hecha. La forma
// sigue siendo la de las tres lecturas porque la respuesta tiene que tener el
// MISMO numero de campos que ellas, y `catalogo_test.go` fija que la
// composicion corta y la larga dan el mismo resultado.
func (c Catalogo) RegistrarObra(ctx context.Context, id string, m repertorio.Metadatos, actorID string) (ObraDelCatalogo, error) {
	obra, err := repertorio.NuevaObra(id, m)
	if err != nil {
		return ObraDelCatalogo{}, err
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
		return ObraDelCatalogo{}, err
	}
	return ObraDelCatalogo{
		Obra:       obra,
		EstadoDecl: repertorio.Declaracion{ObraID: obra.ID()}.Estado(),
	}, nil
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
//
// Corregir los metadatos no toca la declaracion -son dos cosas distintas, y
// `R-03` deja el porcentaje fuera del catalogo-, pero la respuesta la lleva
// igual que la lectura: [conDeclaracionDeUna] se llama DESPUES de que la
// unidad confirme, no dentro de ella -es una lectura de otro puerto
// (Declaraciones), no de Obras, y no hace falta que comparta transaccion con
// el bloqueo de fila de arriba-.
func (c Catalogo) ActualizarMetadatosObra(ctx context.Context, id string, m repertorio.Metadatos, actorID string) (ObraDelCatalogo, error) {
	// Se construye una obra completa y valida ANTES de tocar la base: es el
	// mismo constructor que el alta, asi que una obra corregida cumple lo
	// mismo que una recien creada.
	obra, err := repertorio.NuevaObra(id, m)
	if err != nil {
		return ObraDelCatalogo{}, err
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
		return ObraDelCatalogo{}, err
	}
	return c.conDeclaracionDeUna(ctx, obra)
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
//
// La guarda es [exigirActor], compartida con los otros dos casos de uso que
// firman un hecho: declaraciones y recaudo (ver errores.go, donde esta escrito
// por que vive en esta capa y no en el adaptador).
func (c Catalogo) asentar(ctx context.Context, hecho, obraID, actorID string, p AsientoObra) error {
	if err := exigirActor(actorID, fmt.Sprintf("asentar %q sobre la obra %q", hecho, obraID)); err != nil {
		return err
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
// a medias -- cargar_test.go, en semilla, construye Catalogo{Obras: store,
// Declaraciones: store} a proposito para las dos lecturas que no necesitan
// nada mas -- devuelve un error legible en vez de un nil pointer dereference
// en cuanto RegistrarObra o ActualizarMetadatosObra intenten escribir.
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
func (c Catalogo) ObraPorID(ctx context.Context, id string) (ObraDelCatalogo, error) {
	// Sin envolver de nuevo: el adaptador ya nombra la obra y la operacion en
	// su propio error (ver [postgres.Store.PorID]). Hacerlo tambien aqui
	// duplicaba el "obra %q" -- una vez del caso de uso, otra del adaptador --
	// en el mismo mensaje sin anadir nada que errors.Is no pueda ver ya.
	obra, err := c.Obras.PorID(ctx, id)
	if err != nil {
		return ObraDelCatalogo{}, err
	}
	return c.conDeclaracionDeUna(ctx, obra)
}

// BuscarObras resuelve una consulta del catalogo.
//
// Un filtro vacio devuelve la primera pagina del catalogo: "sin recorte" de
// titulo/genero/IPI/anio sigue siendo un recorte mas, y la paginacion es el
// tope que evita servir el catalogo entero de REDES SGC de un golpe. El
// defecto vive aqui -no en cada adaptador- para que cualquier
// [CatalogoObras] lo herede y se pueda comprobar sin levantar Postgres.
//
// La pagina sale con el estado y la suma de cada obra, y la declaracion
// vigente de TODA la pagina se lee de una vez: ver [Catalogo.conDeclaracion].
func (c Catalogo) BuscarObras(ctx context.Context, f FiltroObras) ([]ObraDelCatalogo, error) {
	f.Paginacion = f.ConDefecto()
	obras, err := c.Obras.Buscar(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("buscar obras: %w", err)
	}
	return c.conDeclaracion(ctx, obras)
}

// conDeclaracion proyecta cada obra con lo que el sistema sabe de su
// declaracion vigente.
//
// UNA consulta para toda la lista y no una por obra: es el mismo N+1 que
// [CatalogoObras.Buscar] y [GestionDeclaraciones.VigentesDeObras] ya evitan
// cada uno por su lado, y aqui es donde se juntan.
func (c Catalogo) conDeclaracion(ctx context.Context, obras []repertorio.Obra) ([]ObraDelCatalogo, error) {
	ids := make([]string, 0, len(obras))
	for _, o := range obras {
		ids = append(ids, o.ID())
	}
	// Un slice vacio no consulta: una pagina sin resultados no puede depender
	// de que el adaptador sepa que ANY('{}') no devuelve nada.
	vigentes := map[string]VersionDeclaracion{}
	if len(ids) > 0 {
		var err error
		vigentes, err = c.Declaraciones.VigentesDeObras(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("declaraciones vigentes del catalogo: %w", err)
		}
	}

	// make no-nil: una pagina vacia sale como [] y no como nil.
	proyectadas := make([]ObraDelCatalogo, 0, len(obras))
	for _, o := range obras {
		proyectadas = append(proyectadas, proyectarObra(o, vigentes))
	}
	return proyectadas, nil
}

// conDeclaracionDeUna es [Catalogo.conDeclaracion] para una sola obra. Pasa
// por el mismo camino que la pagina a proposito: las cuatro respuestas que
// devuelven una obra tienen que decir lo mismo que el listado, sin una segunda
// traduccion que se pueda desviar.
func (c Catalogo) conDeclaracionDeUna(ctx context.Context, obra repertorio.Obra) (ObraDelCatalogo, error) {
	proyectadas, err := c.conDeclaracion(ctx, []repertorio.Obra{obra})
	if err != nil {
		return ObraDelCatalogo{}, err
	}
	return proyectadas[0], nil
}

// proyectarObra traduce "la declaracion vigente de esta obra" a lo que el
// catalogo dice de ella.
//
// Una obra que no viene en el mapa NO tiene declaracion, y ahi el estado sale
// de la Declaracion cero -cuyo Estado() es "incompleta"-, que es lo correcto
// bajo R-04: sin declaracion no se reparte nada. Lo que distingue ese caso es
// VersionVigente en nil, y por eso es un puntero y no un cero: `version 0` no
// es una version que exista (consecutivo por obra desde 1), asi que el cero no
// puede hacer de "no hay".
func proyectarObra(o repertorio.Obra, vigentes map[string]VersionDeclaracion) ObraDelCatalogo {
	vd, hay := vigentes[o.ID()]
	proyectada := ObraDelCatalogo{
		Obra:       o,
		EstadoDecl: repertorio.Declaracion{ObraID: o.ID()}.Estado(),
	}
	if !hay {
		return proyectada
	}
	version := vd.Version
	proyectada.VersionVigente = &version
	proyectada.EstadoDecl = vd.Declaracion.Estado()
	proyectada.SumaPorcentajes = sumaDePorcentajes(vd.Declaracion)
	return proyectada
}

// sumaDePorcentajes suma las partes de una declaracion.
//
// El estado NO se calcula aqui: lo dice [repertorio.Declaracion.Completa], que
// es la autoridad, y esta suma es el numero que se muestra al lado. Los dos
// pueden no coincidir -una parte sin IPI deja la declaracion incompleta con la
// suma en 100-, y por eso el catalogo manda las dos cosas y no una derivada de
// la otra.
//
// Suma en decimal y nunca en float: los porcentajes son NUMERIC(8,4) y sumarlos
// en coma flotante daria un numero distinto del que esta escrito (ADR 0010).
func sumaDePorcentajes(d repertorio.Declaracion) decimal.Decimal {
	suma := decimal.Zero
	for _, p := range d.Partes {
		suma = suma.Add(p.Porcentaje)
	}
	return suma
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
