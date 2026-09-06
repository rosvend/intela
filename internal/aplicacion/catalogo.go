package aplicacion

import (
	"context"
	"fmt"

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
// No reparte, no lee `declaraciones` y no toca dinero. Registrar una obra no
// crea derecho a cobrar: el derecho sale de la Declaracion de Obra (`R-03`),
// que entra por otro camino. Que los coautores del catalogo no lleven
// porcentaje es lo que impide construir aqui el segundo camino hasta un pago
// que `R-02` cierra.
type Catalogo struct {
	Obras CatalogoObras
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
func (c Catalogo) RegistrarObra(ctx context.Context, id string, m repertorio.Metadatos) (repertorio.Obra, error) {
	obra, err := repertorio.NuevaObra(id, m)
	if err != nil {
		return repertorio.Obra{}, err
	}
	if err := c.Obras.Registrar(ctx, obra); err != nil {
		// Sin envolver ErrObraDuplicada en un texto que lo tape: quien llama
		// lo distingue con errors.Is, y el adaptador ya le pone su contexto.
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
// Devuelve ErrNoEncontrado si la obra no existe. No la crea: un PATCH que
// inserta convierte un id mal escrito en una obra fantasma del catalogo, y
// contra el catalogo resuelve todo el matching.
func (c Catalogo) ActualizarMetadatosObra(ctx context.Context, id string, m repertorio.Metadatos) (repertorio.Obra, error) {
	// Se construye una obra completa y valida ANTES de tocar la base: es el
	// mismo constructor que el alta, asi que una obra corregida cumple lo
	// mismo que una recien creada.
	obra, err := repertorio.NuevaObra(id, m)
	if err != nil {
		return repertorio.Obra{}, err
	}
	if err := c.Obras.Actualizar(ctx, obra); err != nil {
		return repertorio.Obra{}, err
	}
	return obra, nil
}

// ObraPorID devuelve una obra del catalogo, o ErrNoEncontrado.
func (c Catalogo) ObraPorID(ctx context.Context, id string) (repertorio.Obra, error) {
	obra, err := c.Obras.PorID(ctx, id)
	if err != nil {
		return repertorio.Obra{}, fmt.Errorf("obra %q: %w", id, err)
	}
	return obra, nil
}

// BuscarObras resuelve una consulta del catalogo.
//
// Un filtro vacio devuelve el catalogo entero, que es el listado. No es un
// caso aparte: "sin recorte" es un recorte mas, y tener dos operaciones para
// eso duplicaria el orden, la traduccion y la autorizacion.
func (c Catalogo) BuscarObras(ctx context.Context, f FiltroObras) ([]repertorio.Obra, error) {
	obras, err := c.Obras.Buscar(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("buscar obras: %w", err)
	}
	return obras, nil
}
