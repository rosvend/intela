package postgres

import (
	"context"
	"encoding/json"
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

// columnasCatalogoDe es lo mismo con alias de tabla, para las lecturas que
// juntan obras y coautores en una sola sentencia.
const columnasCatalogoDe = `o.id, o.titulo, o.genero, o.anio, o.tipo, o.ida, o.eidr, o.imdb`

const columnasCoautor = `ipi, nombre, rol`

// lateralCoautores agrega los coautores de cada obra en la misma sentencia
// que lee `obras`. Una sola instantanea: sin el, PorID y Buscar leian las dos
// tablas en viajes aparte y un Actualizar que confirmara entre medias podia
// devolver metadatos viejos con coautores nuevos (issue #90).
const lateralCoautores = `
	LEFT JOIN LATERAL (
	  SELECT jsonb_agg(
	           jsonb_build_object('ipi', c.ipi, 'nombre', c.nombre, 'rol', c.rol)
	           ORDER BY c.ipi, c.rol
	         ) AS coautores
	    FROM obra_coautores c
	   WHERE c.obra_id = o.id
	) ca ON true`

// Registrar inserta la obra y sus coautores en una sola transaccion.
//
// El limite propio -- las dos tablas de ESTA operacion -- lo fija el adaptador
// y no el caso de uso: es UNA operacion del puerto, y su contrato dice que es
// atomica. Una obra a medias -en `obras` pero sin coautores- no la puede
// reconstruir [repertorio.NuevaObra], asi que quedaria escrita y no se podria
// leer.
//
// enTransaccionDe y no EnTransaccion: si el caso de uso ya abrio una unidad
// -- [Catalogo.RegistrarObra] la abre para que la obra y su asiento sean un
// solo hecho (ADR 0006, issue #91) -- esta escritura entra EN ELLA y la
// confirma quien la abrio. Con EnTransaccion, la obra se confirmaria aqui y un
// asiento que fallara despues ya no tendria nada que revertir.
//
// El duplicado lo decide la clave primaria. Un SELECT previo dejaria una
// ventana entre la consulta y el INSERT por la que cabe otra peticion.
func (s *Store) Registrar(ctx context.Context, o repertorio.Obra) error {
	m := o.Metadatos()
	return s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
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
// Participa en la unidad de quien llame, igual que [Store.Registrar] y por la
// misma razon: [Catalogo.ActualizarMetadatosObra] lee la obra, la reescribe y
// asienta el cambio, y las tres cosas son un solo hecho.
//
// Los coautores se borran y se vuelven a escribir en vez de reconciliarse fila
// a fila. Es lo mismo que hace el caso de uso conceptualmente -el bloque de
// metadatos llega completo- y ahorra tener que decidir que hacer con un
// coautor que estaba y ya no viene. El ON DELETE CASCADE no interviene: la
// obra no se borra.
func (s *Store) Actualizar(ctx context.Context, o repertorio.Obra) error {
	m := o.Metadatos()
	return s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
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

// PorID reconstruye una obra del catalogo en una sola sentencia: metadatos y
// coautores salen de la misma instantanea (issue #90).
//
// Lee por [Store.ejecutorDe] y no por el pool porque esta lectura tambien
// ocurre DENTRO de una unidad: [Catalogo.ActualizarMetadatosObra] la usa para
// saber que habia antes y poder asentar que cambio. Por el pool leeria en otra
// conexion, fuera de la transaccion que esta a punto de reescribir esa misma
// fila.
func (s *Store) PorID(ctx context.Context, id string) (repertorio.Obra, error) {
	var (
		fl            fila
		tipo          string
		coautoresJSON []byte
	)
	err := s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT `+columnasCatalogoDe+`, COALESCE(ca.coautores, '[]'::jsonb)
		   FROM obras o`+lateralCoautores+`
		  WHERE o.id = $1`, id).
		Scan(&fl.id, &fl.titulo, &fl.genero, &fl.anio, &tipo,
			&fl.ida, &fl.eidr, &fl.imdb, &coautoresJSON)
	if err != nil {
		return repertorio.Obra{}, traducirError(err, "obra %q del catalogo", id)
	}
	fl.tipo = repertorio.TipoObra(tipo)
	coautores, err := decodificarCoautores(coautoresJSON)
	if err != nil {
		return repertorio.Obra{}, fmt.Errorf("obra %q del catalogo: %w", id, err)
	}
	return fl.entidad(coautores)
}

// Buscar resuelve los cuatro filtros y la paginacion en una consulta.
//
// Titulo, genero y anio se neutralizan con su valor cero dentro del WHERE: una
// sola sentencia, sin concatenar (decision 8 de #86). El IPI NO entra en esa
// forma: `($5 = ” OR EXISTS (...))` convertia el EXISTS en un subplan
// hasheado sobre cada fila de `obras` y barria el catalogo entero. Aqui el
// camino con IPI y el camino sin IPI van en un UNION ALL mutuamente excluyente
// -`$5 <> ”` frente a `$5 = ”`-, asi el EXISTS suelto deja que
// `obra_coautores_ipi` guie el Nested Loop (issue #90).
//
// La pagina se resuelve PRIMERO (ids + metadatos) y el LATERAL de coautores
// se aplica despues, solo a las filas que sobreviven al LIMIT: si el jsonb_agg
// corriera antes del recorte, la primera pagina costaria mas que servir el
// catalogo entero.
//
// ORDER BY id nombra la clave (no la posicion): el ADR 0005 exige
// reproducibilidad, y el UNION ALL duplica columnasCatalogoDe.
//
// OFFSET degrada linealmente con la profundidad (KISS hoy). Cuando el
// catalogo sea real, el paso a keyset es una decision, no un descubrimiento.
func (s *Store) Buscar(ctx context.Context, f aplicacion.FiltroObras) ([]repertorio.Obra, error) {
	p := f.ConDefecto()
	const filtrosComunes = `
		    AND ($1 = ''  OR o.titulo ILIKE $2)
		    AND ($3 = ''  OR o.genero = $3)
		    AND ($4 = 0   OR o.anio   = $4)`
	// pagina: ids + metadatos sin coautores. El LATERAL va fuera, contra
	// las filas que pasan el LIMIT (o contra todas si LimiteSinTope).
	pagina := `
	     (
	       SELECT ` + columnasCatalogoDe + `
	         FROM obras o
	        WHERE $5 <> ''
	          AND EXISTS (SELECT 1 FROM obra_coautores c
	                       WHERE c.obra_id = o.id AND c.ipi = $5)` + filtrosComunes + `
	     )
	     UNION ALL
	     (
	       SELECT ` + columnasCatalogoDe + `
	         FROM obras o
	        WHERE $5 = ''` + filtrosComunes + `
	     )`
	sql := `SELECT ` + columnasCatalogoDe + `, COALESCE(ca.coautores, '[]'::jsonb)
	   FROM (` + pagina + `
	     ORDER BY id`
	args := []any{f.Titulo, patronContiene(f.Titulo), f.Genero, f.Anio, f.IPI}
	if p.Limite != aplicacion.LimiteSinTope {
		sql += `
	     LIMIT $6 OFFSET $7`
		args = append(args, p.Limite, p.Desplazamiento)
	}
	sql += `
	   ) o` + lateralCoautores + `
	 ORDER BY o.id`

	filas, err := s.ejecutorDe(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, traducirError(err, "buscar obras")
	}
	defer filas.Close()

	var obras []repertorio.Obra
	for filas.Next() {
		var (
			fl            fila
			tipo          string
			coautoresJSON []byte
		)
		if err := filas.Scan(&fl.id, &fl.titulo, &fl.genero, &fl.anio, &tipo,
			&fl.ida, &fl.eidr, &fl.imdb, &coautoresJSON); err != nil {
			return nil, traducirError(err, "escanear obra del catalogo")
		}
		fl.tipo = repertorio.TipoObra(tipo)
		coautores, err := decodificarCoautores(coautoresJSON)
		if err != nil {
			return nil, fmt.Errorf("obra %q del catalogo: %w", fl.id, err)
		}
		obra, err := fl.entidad(coautores)
		if err != nil {
			return nil, err
		}
		obras = append(obras, obra)
	}
	// No es opcional: un fallo a mitad de stream sale solo por aqui, y sin
	// esta comprobacion una lista TRUNCADA se devuelve como lista completa.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "buscar obras")
	}
	return obras, nil
}

// decodificarCoautores traduce el jsonb_agg de lateralCoautores.
func decodificarCoautores(bruto []byte) ([]repertorio.Coautor, error) {
	if len(bruto) == 0 || string(bruto) == "null" {
		return nil, nil
	}
	var filas []struct {
		IPI    string `json:"ipi"`
		Nombre string `json:"nombre"`
		Rol    string `json:"rol"`
	}
	if err := json.Unmarshal(bruto, &filas); err != nil {
		return nil, fmt.Errorf("decodificar coautores: %w", err)
	}
	out := make([]repertorio.Coautor, 0, len(filas))
	for _, f := range filas {
		out = append(out, repertorio.Coautor{
			IPI:    f.IPI,
			Nombre: f.Nombre,
			Rol:    repertorio.RolAutoral(f.Rol),
		})
	}
	return out, nil
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
