package aplicacion

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Catalogo son los casos de uso del catalogo maestro de obras: el cubo contra
// el que resuelve todo matching (docs/dominio/identificadores.md).
//
// Un servicio con tres operaciones y no tres structs, por lo mismo que
// [Autenticacion]: giran sobre el mismo agregado y comparten la unica
// dependencia. La segregacion del ADR 0003 se conserva donde importa -lo que
// se inyecta es [CatalogoObras], un puerto de cuatro metodos, no un
// repositorio que lo sepa todo-.
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
// [GestionDeclaraciones], sin versiones de por medio-, porque el catalogo es
// donde un administrador ve que obras estan completas y que obras quedan
// retenidas. Los tres campos que salen de ahi son derivados y de solo lectura:
// el estado de una obra no se declara desde el catalogo.
type Catalogo struct {
	Obras CatalogoObras

	// Declaraciones se lee, no se escribe desde aqui. Va aparte de [Obras] por
	// lo mismo que los dos puertos estan separados (ver [CatalogoObras]): la
	// obra y su declaracion son dos cosas distintas, y este servicio solo mira
	// la segunda para poder decir en que estado esta la primera.
	Declaraciones GestionDeclaraciones
}

// RegistrarObra da de alta una obra en el catalogo.
//
// El identificador lo trae quien llama y NO se genera aqui. Es el numero de
// obra de REDES-SYS, que se asigna fuera de este sistema, y es lo que hace que
// "un segundo alta con el mismo identificador se rechaza" sea una regla
// comprobable y no una imposibilidad de fabrica.
//
// La validacion es del dominio: [repertorio.NuevaObra] es la unica puerta, asi
// que no hay forma de que llegue al adaptador una obra sin titulo, sin genero,
// sin anio o sin un coautor con IPI.
//
// Devuelve la obra ya proyectada, con el estado de su declaracion, por el
// mismo camino que las tres lecturas.
//
// En un alta ese estado es el cero -`incompleta`, suma 0 y `version_vigente`
// nil-, y no por darlo por supuesto: una declaracion necesita la fila de
// `obras` -su clave foranea- y esta operacion es justo la que la crea, asi que
// no puede haber ninguna. Se lee igual porque la respuesta tiene que tener la
// MISMA forma que las otras tres: un schema que promete tres campos y una
// respuesta que no los trae es el fallo caro de este cambio.
func (c Catalogo) RegistrarObra(ctx context.Context, id string, m repertorio.Metadatos) (ObraDelCatalogo, error) {
	obra, err := repertorio.NuevaObra(id, m)
	if err != nil {
		return ObraDelCatalogo{}, err
	}
	if err := c.Obras.Registrar(ctx, obra); err != nil {
		// Sin envolver ErrObraDuplicada en un texto que lo tape: quien llama
		// lo distingue con errors.Is, y el adaptador ya le pone su contexto.
		return ObraDelCatalogo{}, err
	}
	return c.conDeclaracionDeUna(ctx, obra)
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
// Devuelve ErrNoEncontrado si la obra no existe. No la crea: un PATCH que
// inserta convierte un id mal escrito en una obra fantasma del catalogo, y
// contra el catalogo resuelve todo el matching.
//
// Corregir los metadatos no toca la declaracion -son dos cosas distintas, y
// `R-03` deja el porcentaje fuera del catalogo-, pero la respuesta la lleva
// igual que la lectura: es la misma obra.
func (c Catalogo) ActualizarMetadatosObra(ctx context.Context, id string, m repertorio.Metadatos) (ObraDelCatalogo, error) {
	// Se construye una obra completa y valida ANTES de tocar la base: es el
	// mismo constructor que el alta, asi que una obra corregida cumple lo
	// mismo que una recien creada.
	obra, err := repertorio.NuevaObra(id, m)
	if err != nil {
		return ObraDelCatalogo{}, err
	}
	if err := c.Obras.Actualizar(ctx, obra); err != nil {
		return ObraDelCatalogo{}, err
	}
	return c.conDeclaracionDeUna(ctx, obra)
}

// ObraPorID devuelve una obra del catalogo, o ErrNoEncontrado.
func (c Catalogo) ObraPorID(ctx context.Context, id string) (ObraDelCatalogo, error) {
	obra, err := c.Obras.PorID(ctx, id)
	if err != nil {
		return ObraDelCatalogo{}, fmt.Errorf("obra %q: %w", id, err)
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
