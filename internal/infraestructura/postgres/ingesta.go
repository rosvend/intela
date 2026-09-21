package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioIngesta = (*Store)(nil)

// columnasUso es la proyeccion canonica de una fila de uso.
//
// COALESCE sobre obra_id por la misma razon que en columnasUsuario: la columna
// es NULL mientras la obra no este identificada, y como referencia a obras(id)
// nunca puede ser la cadena vacia. Asi el "" de UsoPersistido.ObraID significa
// exactamente NULL sin meter un tipo nullable en el adaptador.
//
// No hay columna de dinero que proyectar, y no la va a haber: un reporte de uso
// PONDERA la bolsa, no la aporta.
const columnasUso = `id, reporte_id, fuente, titulo, titulo_original, ids_fuente, COALESCE(obra_id, ''),
	escalon, evidencia, oni, modalidad, tipo_obra, canal_id, fecha, hora,
	duracion_min, emisiones, rating, taquilla, espectadores, exhibiciones,
	vistas, minutos_vistos, pb`

// escanearUso lee columnasUso. Una sola funcion para las tres consultas que la
// comparten: con una por consulta, una columna nueva hay que anadirla en tres
// sitios, y el dia que solo se anada en dos el desajuste no se ve hasta que
// una fila vuelve con los campos corridos.
func escanearUso(fila pgx.Row) (aplicacion.UsoPersistido, error) {
	var (
		u         aplicacion.UsoPersistido
		modalidad string
	)
	// modalidad se escanea a string y se convierte, igual que el rol en
	// afiliacion.go: pgx sabe desenvolver un `type Modalidad string`, pero de
	// este campo depende con que formula se valoriza el uso (RD 8) y no
	// conviene que dependa de que plan de escaneo elija la libreria.
	err := fila.Scan(
		&u.ID, &u.ReporteID, &u.Fuente, &u.Titulo, &u.TituloOrig, &u.IDsFuente, &u.ObraID,
		&u.Escalon, &u.Evidencia, &u.ONI, &modalidad, &u.TipoObra, &u.CanalID, &u.Fecha, &u.Hora,
		&u.DuracionMin, &u.Emisiones, &u.Rating, &u.Taquilla, &u.Espectadores,
		&u.Exhibiciones, &u.Vistas, &u.MinutosVistos, &u.PB,
	)
	u.Modalidad = reparto.Modalidad(modalidad)
	return u, err
}

// GuardarReporte registra el acuse de una entrega ya congelada en la boveda.
//
// El unico error que se traduce a vocabulario del negocio es el duplicado. Lo
// decide el UNIQUE (sha256, fuente), que es la fuente de verdad de "esta
// entrega ya llego": el nombre del archivo no sirve -los contadores de fila
// del estilo Id_Ntx se renumeran en cada entrega- y comprobarlo antes con un
// SELECT dejaria una ventana entre la consulta y el INSERT.
//
// La clave primaria puede chocar tambien, pero solo si coinciden fuente y
// huella -el id se deriva de ese par-, o sea exactamente en el mismo caso. Por
// eso basta con mirar el codigo de unicidad y no hace falta distinguir que
// restriccion salto.
func (s *Store) GuardarReporte(ctx context.Context, id, fuente, periodo, sha, claveObjeto string, nbytes int) error {
	_, err := s.ejecutorDe(ctx).Exec(ctx, sqlInsertarReporte, id, fuente, periodo, sha, claveObjeto, nbytes)
	return traducirErrorDeReporte(err, fuente, periodo)
}

// sqlInsertarReporte lo comparten [Store.GuardarReporte] y
// [Store.GuardarEntrega]: la misma fila, escrita fuera o dentro de una
// transaccion. Una constante y no dos literales para que no puedan divergir --
// una columna anadida en un sitio y olvidada en el otro no falla, escribe una
// entrega incompleta por uno de los dos caminos.
const sqlInsertarReporte = `INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes)
	 VALUES ($1, $2, $3, $4, $5, $6)`

// traducirErrorDeReporte pone el duplicado por huella en vocabulario del
// negocio, y lo demas en el traductor general.
func traducirErrorDeReporte(err error, fuente, periodo string) error {
	if esClaveDuplicada(err) {
		return fmt.Errorf("guardar reporte de %q, periodo %q: %w",
			fuente, periodo, aplicacion.ErrReporteDuplicado)
	}
	return traducirError(err, "guardar reporte de %q, periodo %q", fuente, periodo)
}

// GuardarEntrega escribe el acuse de una entrega y sus filas en UNA transaccion.
//
// # Por que existe, teniendo GuardarReporte y GuardarUsos
//
// Porque llamarlos en fila no es lo mismo. El acuse QUEMA la huella: el
// duplicado lo decide el UNIQUE (sha256, fuente), asi que un acuse escrito y un
// lote que falla despues dejan la entrega registrada con CERO usos y la huella
// gastada. El cliente reenvia el mismo archivo -- que es justo lo que hace
// cuando le dicen que su carga fallo -- y se lleva un ErrReporteDuplicado para
// siempre; de la boveda no se borra (ADR 0006) y la fila de `reportes` no la
// quita nadie. Dentro de una transaccion, un lote que falla no deja acuse, y el
// reenvio entra.
//
// # Que NO entra en la transaccion
//
// La boveda. De un fichero escrito no se hace rollback, y meterlo aqui daria la
// ilusion de atomicidad y no la propiedad. El resto que eso deja -- un objeto sin
// acuse -- es inerte y se recupera solo, porque la clave del objeto es su
// contenido. Lo explica [aplicacion.Ingesta.GuardarReporte].
//
// El limite lo elige el caso de uso llamando a este metodo en vez de a los otros
// dos; la transaccion la abre el adaptador porque el nucleo no puede tocar pgx.
// Es la misma forma que [Store.Registrar] con la obra y sus coautores.
func (s *Store) GuardarEntrega(ctx context.Context, rep aplicacion.Reporte, usos []aplicacion.UsoPersistido) error {
	return s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sqlInsertarReporte,
			rep.ID, rep.Fuente, rep.Periodo, rep.SHA256, rep.ClaveObjeto, rep.NBytes)
		if err != nil {
			return traducirErrorDeReporte(err, rep.Fuente, rep.Periodo)
		}
		return escribirLote(ctx, tx, usos)
	})
}

// ListarCargas devuelve las entregas recibidas con sus dos recuentos.
//
// # Por que subconsultas escalares y no dos LEFT JOIN
//
// `usos` y `usos_rechazados` son dos tablas distintas colgando del mismo
// reporte. Unirlas las dos a la vez con JOIN multiplica las filas -- 3 usos y
// 2 rechazos dan 6 combinaciones -- y a partir de ahi los dos COUNT salen
// inflados sin que nada falle. Se puede arreglar con COUNT(DISTINCT ...), pero
// entonces la correccion del recuento depende de que nadie quite ese DISTINCT
// mas adelante. Con una subconsulta por tabla cada COUNT cuenta lo suyo y no
// hay forma de que se contaminen; las dos van por `usos_reporte` y
// `usos_rechazados_reporte`, que son indices que ya existen.
//
// El periodo se filtra con un parametro que puede ser vacio, no concatenando
// otro WHERE: una sola sentencia, un solo plan, y ningun camino en el que el
// texto del SQL dependa de la entrada.
//
// El orden es por `creado` descendente y desempata por id. Sin el desempate,
// dos cargas del mismo instante -- que es lo normal en una prueba, y posible en
// produccion -- salen en orden arbitrario y el listado cambia entre lecturas.
func (s *Store) ListarCargas(ctx context.Context, periodo string) ([]aplicacion.CargaReporte, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx, `
		SELECT r.id, r.fuente, r.periodo, r.sha256, r.clave_objeto, r.nbytes, r.creado,
		       (SELECT COUNT(*) FROM usos            u WHERE u.reporte_id = r.id),
		       (SELECT COUNT(*) FROM usos_rechazados x WHERE x.reporte_id = r.id)
		  FROM reportes r
		 WHERE $1 = '' OR r.periodo = $1
		 ORDER BY r.creado DESC, r.id`, periodo)
	if err != nil {
		return nil, traducirError(err, "listar cargas del periodo %q", periodo)
	}
	defer filas.Close()

	var cargas []aplicacion.CargaReporte
	for filas.Next() {
		var c aplicacion.CargaReporte
		if err := filas.Scan(
			&c.ID, &c.Fuente, &c.Periodo, &c.SHA256, &c.ClaveObjeto, &c.NBytes, &c.Recibido,
			&c.Aceptados, &c.Rechazados,
		); err != nil {
			return nil, traducirError(err, "escanear carga")
		}
		cargas = append(cargas, c)
	}
	// Igual que en consultarUsos: sin esto una lista TRUNCADA por un fallo a
	// mitad de stream se devuelve como lista completa.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "listar cargas del periodo %q", periodo)
	}
	return cargas, nil
}

// GuardarUsos escribe un lote de filas, canonicas y rechazadas.
//
// # Es transaccional por contrato
//
// Igual que RepositorioResultados.Guardar, y por un motivo del mismo orden: un
// lote guardado a medias deja una entrega cuyo recuento no cuadra con el
// archivo, y nadie sabe cual de las dos mitades falta. La transaccion no la
// abre el caso de uso porque no hay ningun limite que este pueda elegir: el
// lote es UNA llamada, y su atomicidad es parte de lo que la llamada significa.
//
// # El encaminamiento vive aqui, no en el caso de uso
//
// Una fila con RechazoMotivo va a usos_rechazados; el resto, a usos. Que el
// nucleo decida QUE fila es invalida y el adaptador DONDE acaba cada clase es
// la misma division que en sesiones.go, donde el caso de uso maneja tokens en
// claro y el adaptador decide guardar un resumen: en que tabla vive algo es una
// propiedad del almacenamiento. El ADR 0016 explica por que son dos tablas.
//
// Con esto, la exclusion de los rechazos de las lecturas canonicas es
// estructural: no depende de que ninguna consulta futura se acuerde de un
// filtro.
func (s *Store) GuardarUsos(ctx context.Context, usos []aplicacion.UsoPersistido) error {
	if len(usos) == 0 {
		return nil
	}

	return s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		return escribirLote(ctx, tx, usos)
	})
}

// escribirLote encamina cada fila a su tabla, DENTRO de la transaccion que le
// den.
//
// Extraido de GuardarUsos para que [Store.GuardarEntrega] escriba las filas
// exactamente igual: con dos copias del bucle, el encaminamiento del ADR 0016
// -- que es lo que mantiene los rechazos fuera de las lecturas canonicas --
// tendria que acordarse de cambiar en dos sitios.
func escribirLote(ctx context.Context, tx pgx.Tx, usos []aplicacion.UsoPersistido) error {
	for _, u := range usos {
		var err error
		if u.RechazoMotivo != "" {
			err = insertarRechazo(ctx, tx, u)
		} else {
			err = insertarUso(ctx, tx, u)
		}
		if err != nil {
			return traducirError(err, "guardar la fila %q del reporte %q", u.ID, u.ReporteID)
		}
	}
	return nil
}

// insertarUso escribe una fila canonica.
//
// obra_id entra con NULLIF: la cadena vacia de UsoPersistido significa "sin
// obra todavia", y el CHECK uso_resuelto_tiene_obra la quiere como NULL. Sin
// esto, una fila en ONI intentaria guardar la cadena vacia como referencia a
// obras(id) y fallaria por clave foranea con un mensaje que no dice nada de lo
// que pasa.
//
// NULLIF compara con la cadena vacia LITERAL, y no hay forma de que sea mas
// indulgente sin meter aqui una regla de negocio: envolver el parametro en un
// btrim() haria que la base decidiera por su cuenta que cuenta como "sin obra",
// que es justo la clase de criterio que no puede vivir en dos sitios.
//
// Por eso el valor llega ya recortado: Ingesta.GuardarUsos hace el TrimSpace una
// sola vez, arriba del todo, y esta comparacion es la MISMA que la de Go, no una
// segunda opinion.
//
// Si algun dia otro camino escribiera usos sin pasar por ese caso de uso, tiene
// que traer esa misma garantia. Sin ella, un obra_id de solo blancos esquiva el
// NULLIF, viola el CHECK y aborta la transaccion del lote ENTERO.
func insertarUso(ctx context.Context, tx pgx.Tx, u aplicacion.UsoPersistido) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO usos (
		   id, reporte_id, fuente, titulo, titulo_original, ids_fuente, obra_id, escalon, evidencia,
		   oni, modalidad, tipo_obra, canal_id, fecha, hora,
		   duracion_min, emisiones, rating, taquilla, espectadores, exhibiciones,
		   vistas, minutos_vistos, pb)
		 VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9,
		         $10, $11, $12, $13, $14, $15,
		         $16, $17, $18, $19, $20, $21,
		         $22, $23, $24)`,
		u.ID, u.ReporteID, u.Fuente, u.Titulo, u.TituloOrig, u.IDsFuente, u.ObraID, u.Escalon, u.Evidencia,
		u.ONI, string(u.Modalidad), u.TipoObra, u.CanalID, u.Fecha, u.Hora,
		u.DuracionMin, u.Emisiones, u.Rating, u.Taquilla, u.Espectadores, u.Exhibiciones,
		u.Vistas, u.MinutosVistos, u.PB)
	return err
}

// insertarRechazo escribe una fila en el log de rechazos.
//
// Guarda lo identificatorio y el motivo, y NINGUNA columna de medida: una fila
// rechazada no pondera, y sin las medidas aqui no hay forma de que una consulta
// futura la sume "solo para ver" (ADR 0016).
func insertarRechazo(ctx context.Context, tx pgx.Tx, u aplicacion.UsoPersistido) error {
	tipo := u.RechazoTipo
	if tipo == "" {
		tipo = aplicacion.TipoRevisionAdaptador
	}
	codigo := u.RechazoCodigo
	if codigo == "" {
		codigo = aplicacion.CodigoRechazoFormato
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO usos_rechazados (id, reporte_id, fuente, titulo, ids_fuente, modalidad, motivo, tipo, codigo)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		u.ID, u.ReporteID, u.Fuente, u.Titulo, u.IDsFuente, string(u.Modalidad), u.RechazoMotivo, tipo, codigo)
	return err
}

// UsosSinResolver devuelve las filas que la cascada de identificacion todavia
// no ha mirado.
//
// El filtro es escalon = 'pendiente' y no `oni`: son dos preguntas distintas.
// Una fila en ONI ya paso por la cascada y no la reconocio nadie -esa cola la
// sirve RepositorioONI-, mientras que una pendiente ni siquiera se ha
// intentado. Ademas hay un indice parcial hecho para este WHERE.
func (s *Store) UsosSinResolver(ctx context.Context) ([]aplicacion.UsoPersistido, error) {
	return s.consultarUsos(ctx,
		`SELECT `+columnasUso+` FROM usos WHERE escalon = 'pendiente' ORDER BY id`,
		"listar usos sin resolver")
}

// UsosDePeriodo devuelve las filas de los reportes de un periodo.
//
// El periodo no esta en `usos`: vive en el reporte del que salio la fila. Va
// como subconsulta y no como JOIN para poder reutilizar columnasUso tal cual;
// con un JOIN habria que calificar las dieciocho columnas con el alias de la
// tabla, y esa proyeccion se comparte justamente para que no diverja.
func (s *Store) UsosDePeriodo(ctx context.Context, periodo string) ([]aplicacion.UsoPersistido, error) {
	return s.consultarUsos(ctx,
		`SELECT `+columnasUso+` FROM usos
		  WHERE reporte_id IN (SELECT id FROM reportes WHERE periodo = $1)
		  ORDER BY id`,
		"listar usos del periodo %q", periodo)
}

// UsoPorID resuelve una fila canonica.
//
// Un id que solo existe en el log de rechazos da ErrNoEncontrado, y es lo
// correcto: esa fila no es un uso. Que no se pueda leer por aqui es la misma
// propiedad que la hace invisible para el reparto.
func (s *Store) UsoPorID(ctx context.Context, id string) (aplicacion.UsoPersistido, error) {
	fila := s.ejecutorDe(ctx).QueryRow(ctx, `SELECT `+columnasUso+` FROM usos WHERE id = $1`, id)

	u, err := escanearUso(fila)
	if err != nil {
		return aplicacion.UsoPersistido{}, traducirError(err, "uso por id %q", id)
	}
	return u, nil
}

// ListarRechazos es la cola de revision de OE-1. Devuelve lo que no se pudo
// normalizar, cada fila con su motivo y discriminante tipado. Un log vacio es
// una lista vacia, no un error: la cola encoge cuando el cliente manda el
// archivo bien.
//
// LIMIT 1000: sin cota, /admin/cola-revision devolveria el log entero (S5).
func (s *Store) ListarRechazos(ctx context.Context) ([]aplicacion.UsoPersistido, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT id, reporte_id, fuente, titulo, ids_fuente, modalidad, motivo, tipo, codigo
		   FROM usos_rechazados
		  ORDER BY id
		  LIMIT 1000`)
	if err != nil {
		return nil, traducirError(err, "listar rechazos")
	}
	defer filas.Close()

	usos := make([]aplicacion.UsoPersistido, 0)
	for filas.Next() {
		var (
			u         aplicacion.UsoPersistido
			modalidad string
		)
		if err := filas.Scan(
			&u.ID, &u.ReporteID, &u.Fuente, &u.Titulo, &u.IDsFuente, &modalidad,
			&u.RechazoMotivo, &u.RechazoTipo, &u.RechazoCodigo,
		); err != nil {
			return nil, traducirError(err, "escanear rechazo")
		}
		u.Modalidad = reparto.Modalidad(modalidad)
		usos = append(usos, u)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "listar rechazos")
	}
	return usos, nil
}

// RechazosDeReporte devuelve una pagina del log de rechazos de una entrega, en
// orden de fila del archivo.
//
// # Una sola sentencia, y por eso un LEFT JOIN desde `reportes`
//
// "La entrega no existe" y "existe sin rechazos" tienen que salir de la MISMA
// lectura. Con un SELECT de existencia y despues otro de filas, la entrega
// puede borrarse entre los dos -el log cuelga de ella con ON DELETE CASCADE- y
// la respuesta mezclaria dos estados de la base. Asi: cero filas es que la
// entrega no existe; una sola fila con x.id NULL es la fila nula del LEFT JOIN,
// o sea una entrega sin rechazos.
//
// # Por que la pagina va en un CTE y la existencia fuera
//
// La cota se aplica a la PAGINA y no a la lectura. Con el LIMIT en la sentencia
// de arriba, una pagina vacia -`desplazamiento` mas alla del final, o una
// entrega sin rechazos consultada desde la pagina 2- devolvia cero filas y el
// adaptador la leia como "esa entrega no existe": un 404 sobre una entrega que
// si existe. La existencia se resuelve fuera de la pagina, asi que una pagina
// vacia sale como lista vacia y el 404 queda para lo que es.
//
// x.id se escanea como *string porque es el unico NULL que significa algo: lo
// distingue de un rechazo de verdad, cuyo id es clave primaria. El resto va con
// COALESCE, por la misma razon que obra_id en columnasUso: sin tipos nullable
// en el adaptador para columnas que son NOT NULL en su tabla y solo salen NULL
// en esa fila.
//
// # El orden: longitud y despues texto
//
// Los ids de fila los deriva la ingesta como `<reporte>-<n>` sin ceros a la
// izquierda, y el orden lexico pondria `-10` antes que `-2`. Dentro de UNA
// entrega todos comparten el prefijo, asi que ordenar por longitud y despues
// por texto es ordenar por n, que es la fila del archivo. Se repite en la
// sentencia de fuera: el orden de un CTE no es una promesa del resultado.
//
// Ojo con el indice: `usos_rechazados_reporte` sirve al JOIN -el WHERE por
// `reporte_id`-, pero `ORDER BY length(x.id), x.id` NO lo puede usar, porque
// Postgres ordena. El indice acota que filas entran; el orden se paga aparte.
func (s *Store) RechazosDeReporte(ctx context.Context, reporteID string, pag aplicacion.Paginacion) ([]aplicacion.UsoPersistido, error) {
	// La cota la aplica la base y no el adaptador: traer el log entero para
	// recortarlo aqui deja en pie el pico de memoria que la cota existe para
	// evitar. `Paginacion{}` aplica [aplicacion.LimiteObrasPorDefecto], igual que
	// las lecturas del catalogo; los valores ilegales los rechaza el adaptador
	// HTTP con 400 antes de llegar aqui.
	pag = pag.ConDefecto()

	filas, err := s.ejecutorDe(ctx).Query(ctx, `
		WITH carga AS (
			SELECT id FROM reportes WHERE id = $1
		), pagina AS (
			SELECT x.id, x.reporte_id, x.fuente, x.titulo, x.ids_fuente,
			       x.modalidad, x.motivo, x.tipo, x.codigo
			  FROM usos_rechazados x
			 WHERE x.reporte_id = $1
			 ORDER BY length(x.id), x.id
			 LIMIT $2 OFFSET $3
		)
		SELECT p.id,
		       COALESCE(p.reporte_id, ''), COALESCE(p.fuente, ''), COALESCE(p.titulo, ''),
		       COALESCE(p.ids_fuente, ''), COALESCE(p.modalidad, ''), COALESCE(p.motivo, ''),
		       COALESCE(p.tipo, ''), COALESCE(p.codigo, ''),
		       c.id AS reporte
		  FROM carga c
		  LEFT JOIN pagina p ON true
		 ORDER BY length(p.id), p.id`,
		reporteID, pag.Limite, pag.Desplazamiento)
	if err != nil {
		return nil, traducirError(err, "rechazos del reporte %q", reporteID)
	}
	defer filas.Close()

	existe := false
	usos := make([]aplicacion.UsoPersistido, 0)
	for filas.Next() {
		existe = true
		var (
			u         aplicacion.UsoPersistido
			id        *string
			modalidad string
			reporte   string
		)
		if err := filas.Scan(
			&id, &u.ReporteID, &u.Fuente, &u.Titulo, &u.IDsFuente, &modalidad,
			&u.RechazoMotivo, &u.RechazoTipo, &u.RechazoCodigo, &reporte,
		); err != nil {
			return nil, traducirError(err, "escanear rechazo")
		}
		if id == nil {
			continue
		}
		u.ID = *id
		// Igual que en ListarRechazos: a string y despues al tipo, sin depender
		// del plan de escaneo de la libreria.
		u.Modalidad = reparto.Modalidad(modalidad)
		usos = append(usos, u)
	}
	// Igual que en ListarCargas: sin esto un log TRUNCADO por un fallo a mitad
	// de stream se devuelve como log completo.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "rechazos del reporte %q", reporteID)
	}
	if !existe {
		return nil, fmt.Errorf("rechazos del reporte %q: %w", reporteID, aplicacion.ErrNoEncontrado)
	}
	return usos, nil
}

// UsosPorIDs resuelve un lote de ids en un solo viaje (S5). Los que no
// existen simplemente no aparecen en el mapa.
func (s *Store) UsosPorIDs(ctx context.Context, ids []string) (map[string]aplicacion.UsoPersistido, error) {
	out := make(map[string]aplicacion.UsoPersistido, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT `+columnasUso+` FROM usos WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, traducirError(err, "usos por ids")
	}
	defer filas.Close()
	for filas.Next() {
		u, err := escanearUso(filas)
		if err != nil {
			return nil, traducirError(err, "escanear uso")
		}
		out[u.ID] = u
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "usos por ids")
	}
	return out, nil
}

// SnapshotNormalizacion lee de `parametros` los coeficientes que la ingesta
// necesita para aplicar RD 9.1.1 y las tasas de cambio. No es el
// SnapshotEnFecha completo del proceso de reparto: solo lo que #26 cablea.
//
// MonedaBase es COP porque las claves cambio.* se siembran como factor a
// pesos. La lista de monedas NO vive en Go: solo se convierten las que
// tengan fila cambio.<ISO>.
func (s *Store) SnapshotNormalizacion(ctx context.Context) (reparto.Snapshot, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx, `
		SELECT clave, valor FROM parametros
		 WHERE vigente_hasta IS NULL
		    OR vigente_hasta > CURRENT_DATE
		 ORDER BY clave, vigente_desde DESC`)
	if err != nil {
		return reparto.Snapshot{}, traducirError(err, "leer parametros de normalizacion")
	}
	defer filas.Close()

	snap := reparto.Snapshot{
		MonedaBase: "COP",
		Tasas:      map[string]decimal.Decimal{},
	}
	vistos := map[string]bool{}
	for filas.Next() {
		var clave string
		var valor decimal.Decimal
		if err := filas.Scan(&clave, &valor); err != nil {
			return reparto.Snapshot{}, traducirError(err, "escanear parametro")
		}
		if vistos[clave] {
			continue
		}
		vistos[clave] = true
		switch clave {
		case "duracion.artistica_pct":
			snap.DuracionArtisticaPct = valor
		case "duracion.minutos_hora_tv":
			snap.MinutosHoraTV = valor
		default:
			if codigo, ok := strings.CutPrefix(clave, "cambio."); ok && codigo != "" {
				snap.Tasas[strings.ToUpper(codigo)] = valor
			}
		}
	}
	if err := filas.Err(); err != nil {
		return reparto.Snapshot{}, traducirError(err, "leer parametros de normalizacion")
	}
	return snap, nil
}

// consultarUsos comparte el recorrido de las dos lecturas de lista.
//
// Los argumentos del SQL son los mismos que los del contexto del error a
// proposito: las dos consultas que la usan filtran, como mucho, por un valor.
// Si algun dia hicieran falta dos parametros distintos, se parten; hoy
// unificarlos evita repetir el bucle y su filas.Err().
func (s *Store) consultarUsos(ctx context.Context, sql, contexto string, args ...any) ([]aplicacion.UsoPersistido, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, traducirError(err, contexto, args...)
	}
	defer filas.Close()

	var usos []aplicacion.UsoPersistido
	for filas.Next() {
		u, err := escanearUso(filas)
		if err != nil {
			return nil, traducirError(err, "escanear uso")
		}
		usos = append(usos, u)
	}
	// No es opcional: un fallo a mitad de stream sale solo por aqui, y sin
	// esta comprobacion una lista TRUNCADA se devuelve como lista completa.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, contexto, args...)
	}
	return usos, nil
}
