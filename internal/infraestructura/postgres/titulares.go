package postgres

import (
	"context"
	"fmt"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

var _ aplicacion.PadronTitulares = (*Store)(nil)

// columnasTitular es la fila del padron tal como sale de la base, antes de ser
// una entidad.
//
// Sin `email`, y no es un olvido: no es un campo de [afiliacion.Titular] y
// ninguna regla del padron lo lee. Leerlo para tirarlo sacaria de la base la
// direccion de contacto de cada titular en cada listado, para nada -la misma
// cuenta que llevo a sacar `password_hash` de columnasUsuario-.
const columnasTitular = `id, nombre, ipi, persona_natural, clase`

// BuscarTitulares resuelve los cuatro filtros y la paginacion en una consulta.
//
// # Los filtros
//
// Nombre, IPI, persona natural e IDs se neutralizan dentro del WHERE con su
// valor cero -cadena vacia, y NULL para el booleano-, que es la misma forma del
// catalogo de obras ([Store.Buscar]) y evita concatenar SQL. La diferencia con
// `obras` es que aqui el IPI no cruza contra ninguna otra tabla: en el padron
// el IPI es una columna propia, no la de un coautor.
//
// El ILIKE del nombre pasa por [patronContiene]: el comodin que escriba quien
// busca es texto, no sintaxis. Sin eso, un `%` pediria el padron entero.
//
// El CAST de `persona_natural` es obligatorio y no cosmetico: el parametro
// llega como NULL cuando nadie filtra, y un parametro que solo aparece
// comparado con NULL no tiene de donde deducir su tipo.
//
// El filtro de IDs es el unico de los cuatro con indice detras: `id` es la
// clave primaria, y `nombre` ni `persona_natural` tienen ninguno (ver abajo).
// Es lo que hace que la consulta de `R-01`, que es quien lo usa, cueste lo que
// mida la lista de ids que le llega y no lo que mida el padron.
//
// # Lo que no hay aqui
//
// Un indice. El esquema no tiene ninguno sobre `nombre` ni sobre
// `persona_natural` -solo la clave primaria de `id`-, asi que el ILIKE es un
// seq scan sobre el padron. Anadir uno es una migracion, y esta #30 no trae
// ninguna: cuando el padron real este cargado, el indice se decide con el
// EXPLAIN delante, no antes. Es el mismo criterio que el OFFSET del catalogo.
//
// ORDER BY id nombra la clave: el ADR 0005 exige que una corrida se reproduzca
// bit a bit, y una pagina sin orden explicito no lo garantiza.
func (s *Store) BuscarTitulares(ctx context.Context, f aplicacion.FiltroTitulares) ([]afiliacion.Titular, error) {
	p := f.ConDefecto()

	// Un IDs vacio y uno nil son la misma pregunta -"sin filtro"- y en SQL no
	// lo son: el slice nil viaja como NULL, y `cardinality(NULL) = 0` no es
	// cierto, es NULL. Se igualan aqui para que la neutralizacion del WHERE sea
	// una condicion y no tres.
	ids := f.IDs
	if ids == nil {
		ids = []string{}
	}

	sql := `SELECT ` + columnasTitular + `
	         FROM titulares
	        WHERE ($1 = '' OR nombre ILIKE $2)
	          AND ($3 = '' OR ipi = $3)
	          AND ($4::boolean IS NULL OR persona_natural = $4)
	          AND (cardinality($5::text[]) = 0 OR id = ANY($5))
	        ORDER BY id`
	args := []any{f.Nombre, patronContiene(f.Nombre), f.IPI, f.PersonaNatural, ids}
	if p.Limite != aplicacion.LimiteSinTope {
		sql += `
	        LIMIT $6 OFFSET $7`
		args = append(args, p.Limite, p.Desplazamiento)
	}

	filas, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, traducirError(err, "buscar titulares")
	}
	defer filas.Close()

	var titulares []afiliacion.Titular
	for filas.Next() {
		var (
			id, nombre, ipi string
			personaNatural  bool
			clase           string
		)
		if err := filas.Scan(&id, &nombre, &ipi, &personaNatural, &clase); err != nil {
			return nil, traducirError(err, "escanear titular del padron")
		}
		// La fila se reconstruye por el MISMO constructor que valida el alta,
		// igual que hace el catalogo con las obras: una fila que no forma un
		// titular -insertada por SQL crudo, saltandose el dominio- falla la
		// lectura nombrandola, en vez de circular como una entrada valida del
		// padron.
		titular, err := afiliacion.NuevoTitular(id, nombre, ipi, personaNatural, afiliacion.Clase(clase))
		if err != nil {
			return nil, fmt.Errorf("el titular %q del padron no forma un titular valido: %w", id, err)
		}
		titulares = append(titulares, titular)
	}
	// No es opcional: un fallo a mitad de stream sale solo por aqui, y sin esta
	// comprobacion una lista TRUNCADA se devuelve como lista completa.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "buscar titulares")
	}
	return titulares, nil
}
