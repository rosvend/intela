package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

var _ aplicacion.CatalogoObras = (*Store)(nil)

// columnasCatalogo es la obra ENTERA, la que reconstruye la entidad.
//
// Distinta de columnasObra, que sirve a la proyeccion que consume el motor de
// reparto y no necesita ni genero ni anio. Dos lecturas, dos listas: compartir
// una sola obligaria a la proyeccion a arrastrar campos que no usa.
const columnasCatalogo = `id, titulo, genero, anio, tipo, ida, eidr, imdb`

const columnasCoautor = `ipi, nombre, rol`

// Registrar inserta la obra y sus coautores en una sola transaccion.
//
// El limite lo fija el adaptador y no el caso de uso, al contrario de lo que
// dice la regla general de doc.go, porque aqui no hay dos repositorios que
// coordinar: es UNA operacion del puerto, y su contrato dice que es atomica.
// Una obra a medias -en `obras` pero sin coautores- no la puede reconstruir
// [repertorio.NuevaObra], asi que quedaria escrita y no se podria leer.
//
// El duplicado lo decide la clave primaria. Un SELECT previo dejaria una
// ventana entre la consulta y el INSERT por la que cabe otra peticion.
func (s *Store) Registrar(ctx context.Context, o repertorio.Obra) error {
	m := o.Metadatos()
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO obras (`+columnasCatalogo+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			o.ID(), m.Titulo, m.Genero, m.Anio, string(m.Tipo), m.IDA, m.EIDR, m.IMDB)
		if err != nil {
			if esClaveDuplicada(err) {
				return fmt.Errorf("registrar obra %q: %w", o.ID(), aplicacion.ErrObraDuplicada)
			}
			return traducirError(err, "registrar obra %q", o.ID())
		}
		return s.escribirCoautores(ctx, tx, o)
	})
}

// Actualizar reemplaza los metadatos de una obra existente. El id no entra en
// el SET: es la clave del WHERE y nada mas.
//
// Los coautores se borran y se vuelven a escribir en vez de reconciliarse fila
// a fila. Es lo mismo que hace el caso de uso conceptualmente -el bloque de
// metadatos llega completo- y ahorra tener que decidir que hacer con un
// coautor que estaba y ya no viene. El ON DELETE CASCADE no interviene: la
// obra no se borra.
func (s *Store) Actualizar(ctx context.Context, o repertorio.Obra) error {
	m := o.Metadatos()
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		etiqueta, err := tx.Exec(ctx,
			`UPDATE obras
			    SET titulo = $2, genero = $3, anio = $4, tipo = $5,
			        ida = $6, eidr = $7, imdb = $8
			  WHERE id = $1`,
			o.ID(), m.Titulo, m.Genero, m.Anio, string(m.Tipo), m.IDA, m.EIDR, m.IMDB)
		if err != nil {
			return traducirError(err, "actualizar obra %q", o.ID())
		}
		// Cero filas es "esa obra no existe", y un UPDATE no lo dice por
		// error: dice que fue bien y no toco nada. Sin esta comprobacion, un
		// PATCH contra un id mal escrito responderia 200 y no habria cambiado
		// nada.
		if etiqueta.RowsAffected() == 0 {
			return fmt.Errorf("actualizar obra %q: %w", o.ID(), aplicacion.ErrNoEncontrado)
		}

		if _, err := tx.Exec(ctx,
			`DELETE FROM obra_coautores WHERE obra_id = $1`, o.ID()); err != nil {
			return traducirError(err, "limpiar coautores de la obra %q", o.ID())
		}
		return s.escribirCoautores(ctx, tx, o)
	})
}

// escribirCoautores inserta la lista entera en una sentencia.
//
// Con unnest y no con un Exec por coautor: son N viajes contra la base dentro
// de la transaccion, y el numero de coautores lo elige quien llama.
func (s *Store) escribirCoautores(ctx context.Context, tx pgx.Tx, o repertorio.Obra) error {
	coautores := o.Coautores()
	ipis := make([]string, len(coautores))
	nombres := make([]string, len(coautores))
	roles := make([]string, len(coautores))
	for i, c := range coautores {
		ipis[i], nombres[i], roles[i] = c.IPI, c.Nombre, string(c.Rol)
	}

	_, err := tx.Exec(ctx,
		`INSERT INTO obra_coautores (obra_id, `+columnasCoautor+`)
		 SELECT $1, * FROM unnest($2::text[], $3::text[], $4::text[])`,
		o.ID(), ipis, nombres, roles)
	if err != nil {
		return traducirError(err, "escribir coautores de la obra %q", o.ID())
	}
	return nil
}

// PorID reconstruye una obra del catalogo.
func (s *Store) PorID(ctx context.Context, id string) (repertorio.Obra, error) {
	var (
		fila fila
		tipo string
	)
	err := s.pool.QueryRow(ctx,
		`SELECT `+columnasCatalogo+` FROM obras WHERE id = $1`, id).
		Scan(&fila.id, &fila.titulo, &fila.genero, &fila.anio, &tipo,
			&fila.ida, &fila.eidr, &fila.imdb)
	if err != nil {
		return repertorio.Obra{}, traducirError(err, "obra %q del catalogo", id)
	}
	fila.tipo = repertorio.TipoObra(tipo)

	coautores, err := s.coautoresDeObras(ctx, []string{id})
	if err != nil {
		return repertorio.Obra{}, err
	}
	return fila.entidad(coautores[id])
}

// Buscar resuelve los cuatro filtros en una consulta.
//
// Cada filtro se neutraliza con su propio valor cero dentro del WHERE en vez
// de armar el SQL a trozos segun lo que venga: una sola sentencia, sin
// concatenar, sin contar parametros a mano y sin que el numero de consultas
// distintas crezca con las combinaciones. El titulo va por ILIKE '%...%', que
// es lo que sabe resolver el indice GIN de trigramas de 00001; los otros tres
// son igualdad.
func (s *Store) Buscar(ctx context.Context, f aplicacion.FiltroObras) ([]repertorio.Obra, error) {
	// ORDER BY id: el ADR 0005 exige que una corrida se reproduzca bit a bit,
	// y una lista sin orden explicito no lo es.
	filas, err := s.pool.Query(ctx,
		`SELECT `+columnasCatalogo+`
		   FROM obras o
		  WHERE ($1 = ''  OR o.titulo ILIKE $2)
		    AND ($3 = ''  OR o.genero = $3)
		    AND ($4 = 0   OR o.anio   = $4)
		    AND ($5 = ''  OR EXISTS (SELECT 1 FROM obra_coautores c
		                              WHERE c.obra_id = o.id AND c.ipi = $5))
		  ORDER BY o.id`,
		f.Titulo, patronContiene(f.Titulo), f.Genero, f.Anio, f.IPI)
	if err != nil {
		return nil, traducirError(err, "buscar obras")
	}
	defer filas.Close()

	var (
		encontradas []fila
		ids         []string
	)
	for filas.Next() {
		var (
			fl   fila
			tipo string
		)
		if err := filas.Scan(&fl.id, &fl.titulo, &fl.genero, &fl.anio, &tipo,
			&fl.ida, &fl.eidr, &fl.imdb); err != nil {
			return nil, traducirError(err, "escanear obra del catalogo")
		}
		fl.tipo = repertorio.TipoObra(tipo)
		encontradas = append(encontradas, fl)
		ids = append(ids, fl.id)
	}
	// No es opcional: un fallo a mitad de stream sale solo por aqui, y sin
	// esta comprobacion una lista TRUNCADA se devuelve como lista completa.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "buscar obras")
	}

	// Una consulta para todos los coautores y no una por obra: con N obras,
	// preguntar los coautores de cada una son N+1 viajes.
	coautores, err := s.coautoresDeObras(ctx, ids)
	if err != nil {
		return nil, err
	}

	obras := make([]repertorio.Obra, 0, len(encontradas))
	for _, fl := range encontradas {
		obra, err := fl.entidad(coautores[fl.id])
		if err != nil {
			return nil, err
		}
		obras = append(obras, obra)
	}
	return obras, nil
}

// coautoresDeObras trae los coautores de varias obras de una vez, agrupados
// por obra.
func (s *Store) coautoresDeObras(ctx context.Context, ids []string) (map[string][]repertorio.Coautor, error) {
	porObra := map[string][]repertorio.Coautor{}
	if len(ids) == 0 {
		return porObra, nil
	}

	// ORDER BY por la misma razon que Buscar: reproducibilidad (ADR 0005).
	filas, err := s.pool.Query(ctx,
		`SELECT obra_id, `+columnasCoautor+`
		   FROM obra_coautores
		  WHERE obra_id = ANY($1)
		  ORDER BY obra_id, ipi, rol`, ids)
	if err != nil {
		return nil, traducirError(err, "coautores del catalogo")
	}
	defer filas.Close()

	for filas.Next() {
		var (
			obraID string
			c      repertorio.Coautor
			rol    string
		)
		if err := filas.Scan(&obraID, &c.IPI, &c.Nombre, &rol); err != nil {
			return nil, traducirError(err, "escanear coautor")
		}
		c.Rol = repertorio.RolAutoral(rol)
		porObra[obraID] = append(porObra[obraID], c)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "coautores del catalogo")
	}
	return porObra, nil
}

// patronContiene envuelve el texto en comodines para un ILIKE, escapando los
// que traiga el propio texto.
//
// Sin el escape, buscar "100%" pide todo lo que empiece por "100" y buscar
// "_" pide el catalogo entero: el comodin del usuario se mezcla con el
// nuestro. No es inyeccion -el valor viaja como parametro- pero si es una
// busqueda que devuelve lo que no se pidio, que en un catalogo contra el que
// se resuelve el matching es peor que no encontrar nada.
//
// El caracter de escape es la barra invertida, que es el que LIKE usa por
// defecto en PostgreSQL, y por eso ella misma tambien se escapa.
func patronContiene(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('%')
	for _, r := range s {
		if r == '\\' || r == '%' || r == '_' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('%')
	return b.String()
}

// fila son las columnas de `obras` tal como salen de la base, antes de ser una
// entidad. Existe para que el escaneo y la construccion sean dos pasos y no
// uno con ocho argumentos posicionales.
type fila struct {
	id     string
	titulo string
	genero string
	anio   int
	tipo   repertorio.TipoObra
	ida    string
	eidr   string
	imdb   string
}

// entidad reconstruye la obra por el MISMO constructor que la creo.
//
// No hay puerta trasera que salte la validacion, y eso tiene una consecuencia
// buscada: si una fila guardada no forma una obra valida -tipicamente porque
// alguien inserto en `obras` sin pasar por aqui y la dejo sin coautores-, la
// lectura FALLA nombrando la obra, en vez de servir un registro que el dominio
// habria rechazado.
//
// Es deliberado y es el caso raro: contra este catalogo resuelve todo el
// matching, asi que una obra que la escritura no habria aceptado no puede
// entrar por la lectura. La invariante de "al menos un coautor con IPI" es de
// agregado y no cabe como CHECK de fila, igual que el 100% de una declaracion;
// esta es la mitad que la sostiene del lado de la lectura.
func (f fila) entidad(coautores []repertorio.Coautor) (repertorio.Obra, error) {
	obra, err := repertorio.NuevaObra(f.id, repertorio.Metadatos{
		Titulo:    f.titulo,
		Genero:    f.genero,
		Anio:      f.anio,
		Tipo:      f.tipo,
		IDA:       f.ida,
		EIDR:      f.eidr,
		IMDB:      f.imdb,
		Coautores: coautores,
	})
	if err != nil {
		return repertorio.Obra{}, fmt.Errorf("la obra %q guardada no forma una obra valida: %w", f.id, err)
	}
	return obra, nil
}
