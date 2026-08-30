package postgres

import (
	"context"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

var _ aplicacion.RepositorioRepertorio = (*Store)(nil)

const columnasObra = `id, titulo, ida, eidr, imdb, tipo`

// declaraciones.version existe pero NO esta en la clave primaria
// (PRIMARY KEY (obra_id, titular_id)), asi que hoy hay exactamente una fila
// por par y la lectura no filtra por version. Es deliberado: elegir que
// version vigente uso una corrida es el trabajo de #23, y ese PR cambia la
// clave. Filtrar aqui por MAX(version) fingiria un versionado que el esquema
// todavia no soporta.
const columnasParte = `titular_id, ipi, porcentaje`

// ListarObras devuelve el catalogo con el estado de cada declaracion.
//
// Dos consultas y no una por obra: con N obras, preguntar las partes de cada
// una es N+1 viajes contra la base. Con dos, el coste no depende de N.
func (s *Store) ListarObras(ctx context.Context) ([]aplicacion.Obra, error) {
	// ORDER BY id: el ADR 0005 exige que una corrida se reproduzca bit a bit,
	// y una lista sin orden explicito no lo es -PostgreSQL no promete ninguno.
	filas, err := s.pool.Query(ctx, `SELECT `+columnasObra+` FROM obras ORDER BY id`)
	if err != nil {
		return nil, traducirError(err, "listar obras")
	}
	defer filas.Close()

	var obras []aplicacion.Obra
	for filas.Next() {
		var o aplicacion.Obra
		if err := filas.Scan(&o.ID, &o.Titulo, &o.IDA, &o.EIDR, &o.IMDB, &o.Tipo); err != nil {
			return nil, traducirError(err, "escanear obra")
		}
		obras = append(obras, o)
	}
	// No es opcional: un fallo a mitad de stream sale por aqui, y sin esta
	// comprobacion una lista TRUNCADA se devuelve como lista completa.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "listar obras")
	}

	partes, err := s.todasLasPartes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range obras {
		// Una obra sin partes da la Declaracion cero, cuyo Estado() es
		// "incompleta". Es lo correcto bajo R-04: sin declaracion no se
		// reparte nada, se retiene.
		d := repertorio.Declaracion{ObraID: obras[i].ID, Partes: partes[obras[i].ID]}
		obras[i].EstadoDecl = d.Estado()
	}
	return obras, nil
}

func (s *Store) ObraPorID(ctx context.Context, id string) (aplicacion.Obra, error) {
	var o aplicacion.Obra
	err := s.pool.QueryRow(ctx,
		`SELECT `+columnasObra+` FROM obras WHERE id = $1`, id).
		Scan(&o.ID, &o.Titulo, &o.IDA, &o.EIDR, &o.IMDB, &o.Tipo)
	if err != nil {
		return aplicacion.Obra{}, traducirError(err, "obra por id %q", id)
	}

	partes, err := s.partesDeObra(ctx, id)
	if err != nil {
		return aplicacion.Obra{}, err
	}
	o.EstadoDecl = repertorio.Declaracion{ObraID: id, Partes: partes}.Estado()
	return o, nil
}

// Declaraciones devuelve una entrada por obra QUE TIENE al menos una parte.
//
// Una obra sin partes no aparece en el mapa, y no hace falta que aparezca:
// pedir una clave ausente devuelve la Declaracion cero, cuyo Estado() ya es
// "incompleta". Si algun caso de uso necesita enumerar obras, que las pida a
// ListarObras; este mapa no es el censo del catalogo.
func (s *Store) Declaraciones(ctx context.Context) (map[string]repertorio.Declaracion, error) {
	partes, err := s.todasLasPartes(ctx)
	if err != nil {
		return nil, err
	}

	decls := make(map[string]repertorio.Declaracion, len(partes))
	for obraID, ps := range partes {
		decls[obraID] = repertorio.Declaracion{ObraID: obraID, Partes: ps}
	}
	return decls, nil
}

// DeclaracionDeObra distingue dos cosas que se parecen y no son iguales.
//
// Una obra que EXISTE y no tiene partes es declaracion_incompleta: un estado
// valido del modelo (R-04, RD 13.1.3), no un error. Una obra que no existe si
// es ErrNoEncontrado. Por eso la existencia se comprueba aparte y no se
// deduce de "no vinieron filas de declaraciones": deducirlo convertiria un
// estado normal del negocio en un 404.
func (s *Store) DeclaracionDeObra(ctx context.Context, obraID string) (repertorio.Declaracion, error) {
	var existe string
	if err := s.pool.QueryRow(ctx,
		`SELECT id FROM obras WHERE id = $1`, obraID).Scan(&existe); err != nil {
		return repertorio.Declaracion{}, traducirError(err, "declaracion de obra %q", obraID)
	}

	partes, err := s.partesDeObra(ctx, obraID)
	if err != nil {
		return repertorio.Declaracion{}, err
	}
	return repertorio.Declaracion{ObraID: obraID, Partes: partes}, nil
}

// partesDeObra lee las partes de una obra. ORDER BY por la misma razon que
// ListarObras: reproducibilidad (ADR 0005).
func (s *Store) partesDeObra(ctx context.Context, obraID string) ([]repertorio.Parte, error) {
	filas, err := s.pool.Query(ctx,
		`SELECT `+columnasParte+` FROM declaraciones WHERE obra_id = $1 ORDER BY titular_id`,
		obraID)
	if err != nil {
		return nil, traducirError(err, "partes de obra %q", obraID)
	}
	defer filas.Close()

	var partes []repertorio.Parte
	for filas.Next() {
		var p repertorio.Parte
		// porcentaje es NUMERIC(8,4) y entra directo a decimal.Decimal: pgx
		// lo entrega como cadena y decimal.Scan la parsea. Ningun float toca
		// un porcentaje de reparto (ADR 0010).
		if err := filas.Scan(&p.TitularID, &p.IPI, &p.Porcentaje); err != nil {
			return nil, traducirError(err, "escanear parte de obra %q", obraID)
		}
		partes = append(partes, p)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "partes de obra %q", obraID)
	}
	return partes, nil
}

// todasLasPartes trae las partes de todas las obras de una vez, agrupadas por
// obra. Es la mitad que evita el N+1 de ListarObras y Declaraciones.
func (s *Store) todasLasPartes(ctx context.Context) (map[string][]repertorio.Parte, error) {
	filas, err := s.pool.Query(ctx,
		`SELECT obra_id, `+columnasParte+` FROM declaraciones ORDER BY obra_id, titular_id`)
	if err != nil {
		return nil, traducirError(err, "listar declaraciones")
	}
	defer filas.Close()

	partes := map[string][]repertorio.Parte{}
	for filas.Next() {
		var (
			obraID string
			p      repertorio.Parte
		)
		if err := filas.Scan(&obraID, &p.TitularID, &p.IPI, &p.Porcentaje); err != nil {
			return nil, traducirError(err, "escanear declaracion")
		}
		partes[obraID] = append(partes[obraID], p)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "listar declaraciones")
	}
	return partes, nil
}
