package semilla

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
)

// ErrBitacoraNoVacia: SEED_RESET se nego porque hay asientos o notificaciones.
//
// asientos y notificaciones tienen triggers BEFORE TRUNCATE (ADR 0006). El
// seed corre antes de que exista ningun asiento; si alguien ya asento, reset
// no es una opcion, es borrar el libro.
var ErrBitacoraNoVacia = errors.New("SEED_RESET rechazado: la bitacora no esta vacia")

// ErrDatosNoSinteticos: SEED_RESET se nego porque en la base hay datos que no
// son del dataset.
var ErrDatosNoSinteticos = errors.New("SEED_RESET rechazado: hay datos que no son del dataset sintetico")

// Claves de las cuentas de desarrollo. Cada rol la suya: una sola clave
// compartida entre distribucion y contabilidad anula el control de doble
// firma (docs/ARRANQUE.md).
type Claves struct {
	Admin        string
	Distribucion string
	Contabilidad string
	Auditor      string
	Titular      string
}

// Cargar persiste el dataset contra una base ya migrada.
//
// Sin reset es idempotente: si el juego completo ya esta, no hace nada; si
// esta a medias, pide SEED_RESET.
//
// Con reset vacia las tablas mutables y vuelve a escribir, pero solo si lo que
// hay es reconocible como semilla: ver [vaciar]. No toca asientos ni
// notificaciones.
func Cargar(ctx context.Context, store *postgres.Store, almacen aplicacion.AlmacenObjetos, hasher aplicacion.Hasher, claves Claves, reset bool, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	pool := store.Pool()
	d := Construir()

	nObras, nReportes, err := recuento(ctx, pool)
	if err != nil {
		return err
	}

	if reset {
		if err := vaciar(ctx, pool, d); err != nil {
			return err
		}
	} else if nObras > 0 && nReportes > 0 {
		log.Info("dataset ya sembrado; pase SEED_RESET=true para recargar")
		return nil
	} else if nObras > 0 || nReportes > 0 {
		return fmt.Errorf("semilla a medias (%d obras, %d reportes): pase SEED_RESET=true", nObras, nReportes)
	}

	hashes, err := hashear(hasher, claves)
	if err != nil {
		return err
	}

	if err := insertarPadron(ctx, store, d, hashes); err != nil {
		return err
	}

	ingesta := aplicacion.Ingesta{Reportes: store, Almacen: almacen}
	for _, r := range d.Reportes {
		rep, err := ingesta.GuardarReporte(ctx, r.Fuente, r.Periodo, r.Bytes)
		if err != nil {
			return fmt.Errorf("guardar reporte %q: %w", r.Fuente, err)
		}
		// H5: GuardarUsos rechaza cualquier obra_id. La identificacion
		// sintetica va despues, en SQL, para que las demos no queden en ONI.
		rechazados, err := ingesta.GuardarUsos(ctx, rep, usosCrudos(r.Usos))
		if err != nil {
			return fmt.Errorf("guardar usos de %q: %w", r.Fuente, err)
		}
		if len(rechazados) > 0 {
			return fmt.Errorf("guardar usos de %q: %d filas rechazadas (%s)",
				r.Fuente, len(rechazados), rechazados[0].RechazoMotivo)
		}
		if err := identificar(ctx, pool, rep.ID, r.Usos); err != nil {
			return fmt.Errorf("identificar usos de %q: %w", r.Fuente, err)
		}
		log.Info("reporte sembrado",
			slog.String("fuente", r.Fuente),
			slog.String("id", rep.ID),
			slog.Int("usos", len(r.Usos)))
	}

	log.Info("dataset sintetico cargado",
		slog.Int("titulares", len(d.Titulares)),
		slog.Int("obras", len(d.Obras)),
		slog.Int("bolsas", len(d.Bolsas)),
		slog.Int("parametros", len(d.Parametros)))
	return nil
}

func recuento(ctx context.Context, pool *pgxpool.Pool) (obras, reportes int, err error) {
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM obras`).Scan(&obras); err != nil {
		return 0, 0, fmt.Errorf("contar obras: %w", err)
	}
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM reportes`).Scan(&reportes); err != nil {
		return 0, 0, fmt.Errorf("contar reportes: %w", err)
	}
	return obras, reportes, nil
}

// vaciar borra las tablas mutables para volver a escribirlas. Solo si lo que
// hay es reconocible como semilla.
//
// # Por que la bitacora no basta como guarda
//
// La comprobacion de asientos y notificaciones mira el LIBRO, no la
// PROCEDENCIA de los datos. El estado real de REDES hoy -catalogo y padron IPI
// cargados, ningun reparto asentado todavia- la pasa entera. Y este binario
// llega a produccion tan facil como con un DSN copiado de staging: con
// SEED_RESET=true borraba titulares y obras de verdad y reescribia `usuarios`
// con admin@redes.co y una clave publicada en docs/ARRANQUE.md.
//
// # La comprobacion mas barata que es honesta
//
// Que todos los ids presentes en `obras` y en `titulares` esten en el propio
// dataset. Son las dos tablas de las que cuelga lo demas -`declaraciones`,
// `alias_obra` y `usos` referencian obras; `declaraciones` y `notificaciones`
// referencian titulares- y son exactamente las dos que REDES ya tiene
// cargadas, asi que una base con un solo dato real dice NO. No hace falta una
// columna de procedencia por fila: el dataset es un juego cerrado de ids
// (ADR 0005), y "no reconozco este id" es todo lo que hay que saber.
func vaciar(ctx context.Context, pool *pgxpool.Pool, d Dataset) error {
	idsObras := make([]string, len(d.Obras))
	for i, o := range d.Obras {
		idsObras[i] = o.ID
	}
	idsTitulares := make([]string, len(d.Titulares))
	for i, t := range d.Titulares {
		idsTitulares[i] = t.ID
	}
	if err := soloDelDataset(ctx, pool, "obras", idsObras); err != nil {
		return err
	}
	if err := soloDelDataset(ctx, pool, "titulares", idsTitulares); err != nil {
		return err
	}

	var asientos, avisos int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM asientos`).Scan(&asientos); err != nil {
		return fmt.Errorf("contar asientos: %w", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notificaciones`).Scan(&avisos); err != nil {
		return fmt.Errorf("contar notificaciones: %w", err)
	}
	if asientos > 0 || avisos > 0 {
		return fmt.Errorf("%w (%d asientos, %d notificaciones)",
			ErrBitacoraNoVacia, asientos, avisos)
	}

	// DELETE y no TRUNCATE. asientos.actor_id referencia usuarios, y
	// notificaciones referencia titulares y procesos: TRUNCATE de esas
	// tablas falla aunque la bitacora este vacia (el FK existe). CASCADE
	// arrastraria asientos y dispararia el trigger BEFORE TRUNCATE.
	// DELETE solo mira filas: con la bitacora vacia, pasa.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrir transaccion para vaciar: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, tabla := range []string{
		"reclamaciones",
		"anticipos",
		"calendario",
		"cola_trabajos",
		"resultados_titular",
		"resultados_obra",
		"resultados_proceso",
		"firmas",
		"procesos",
		"usos_rechazados",
		"usos",
		"reportes",
		"alias_obra",
		"obra_coautores",
		"declaraciones",
		"bolsas",
		"parametros",
		"sesiones",
		"usuarios",
		"obras",
		"titulares",
	} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+tabla); err != nil {
			return fmt.Errorf("vaciar %s: %w", tabla, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar vaciado: %w", err)
	}
	return nil
}

// soloDelDataset falla si en la tabla hay algun id que el dataset no conoce.
//
// El nombre de la tabla se concatena porque un identificador no puede viajar
// como parametro; los dos valores posibles son literales de este fichero.
func soloDelDataset(ctx context.Context, pool *pgxpool.Pool, tabla string, ids []string) error {
	var (
		ajenas  int
		ejemplo string
	)
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(MIN(id), '') FROM `+tabla+` WHERE id <> ALL($1)`, ids,
	).Scan(&ajenas, &ejemplo); err != nil {
		return fmt.Errorf("comprobar la procedencia de %s: %w", tabla, err)
	}
	if ajenas > 0 {
		return fmt.Errorf("%w: %d filas de %s no son del dataset (por ejemplo %q)",
			ErrDatosNoSinteticos, ajenas, tabla, ejemplo)
	}
	return nil
}

func hashear(hasher aplicacion.Hasher, c Claves) (map[string]string, error) {
	pares := []struct{ email, clave string }{
		{EmailAdmin, c.Admin},
		{EmailDistribucion, c.Distribucion},
		{EmailContabilidad, c.Contabilidad},
		{EmailAuditor, c.Auditor},
		{EmailTitular, c.Titular},
	}
	out := make(map[string]string, len(pares))
	for _, p := range pares {
		if p.clave == "" {
			return nil, fmt.Errorf("falta la clave de %s", p.email)
		}
		h, err := hasher.Hash(p.clave)
		if err != nil {
			return nil, fmt.Errorf("hashear %s: %w", p.email, err)
		}
		out[p.email] = h
	}
	return out, nil
}

func insertarPadron(ctx context.Context, store *postgres.Store, d Dataset, hashes map[string]string) error {
	// Las obras van PRIMERO y por el adaptador del catalogo, no por el SQL de
	// abajo. Ver registrarObras: es lo que hace que una obra sin coautores no
	// se pueda sembrar. Van antes porque `declaraciones` y `alias_obra`
	// referencian `obras`, y esta transaccion las escribe.
	if err := registrarObras(ctx, store, d.Obras); err != nil {
		return err
	}

	pool := store.Pool()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrir transaccion del padron: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, tit := range d.Titulares {
		if _, err := tx.Exec(ctx, `
			INSERT INTO titulares (id, nombre, ipi, persona_natural, clase, email)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			tit.ID, tit.Nombre, tit.IPI, tit.PersonaNatural, tit.Clase, tit.Email); err != nil {
			return fmt.Errorf("insertar titular %s: %w", tit.ID, err)
		}
	}

	for _, u := range d.Usuarios {
		hash, ok := hashes[u.Email]
		if !ok {
			return fmt.Errorf("no hay hash para %s", u.Email)
		}
		var titular any
		if u.TitularID != "" {
			titular = u.TitularID
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO usuarios (id, email, nombre, rol, titular_id, password_hash)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			u.ID, u.Email, u.Nombre, string(u.Rol), titular, hash); err != nil {
			return fmt.Errorf("insertar usuario %s: %w", u.ID, err)
		}
	}

	for _, decl := range d.Declaraciones {
		for _, p := range decl.Partes {
			if _, err := tx.Exec(ctx, `
				INSERT INTO declaraciones (obra_id, titular_id, ipi, porcentaje)
				VALUES ($1, $2, $3, $4)`,
				decl.ObraID, p.TitularID, p.IPI, p.Porcentaje); err != nil {
				return fmt.Errorf("insertar declaracion de %s/%s: %w", decl.ObraID, p.TitularID, err)
			}
		}
	}

	for _, b := range d.Bolsas {
		if _, err := tx.Exec(ctx, `
			INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto, convenio, tarifa, factura)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			b.ID, b.UsuarioID, b.Periodo, string(b.Circuito), b.Bruto,
			"convenio-sintetico", "tarifa-sintetica", "factura-sintetica"); err != nil {
			return fmt.Errorf("insertar bolsa %s: %w", b.ID, err)
		}
	}

	for _, p := range d.Parametros {
		if _, err := tx.Exec(ctx, `
			INSERT INTO parametros (clave, valor, vigente_desde, organo, reglamento)
			VALUES ($1, $2, $3, $4, $5)`,
			p.Clave, p.Valor, p.VigenteDesde, p.Organo, p.Reglamento); err != nil {
			return fmt.Errorf("insertar parametro %s: %w", p.Clave, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar padron: %w", err)
	}
	return nil
}

// registrarObras da de alta el catalogo por el MISMO camino que la API.
//
// Cada obra se construye con [repertorio.NuevaObra] y se escribe con
// Store.Registrar, que mete `obras` y `obra_coautores` en una transaccion. Es
// lo que impide de raiz el fallo que tenia el seed: escribia `obras` con un
// INSERT propio y no tocaba `obra_coautores`, asi que las cuatro obras
// quedaban escritas y NINGUNA se podia leer -la lectura reconstruye la entidad
// por el mismo constructor, que exige al menos un coautor con IPI-. GET /obras
// devolvia 500 en todas.
//
// Con el adaptador, un dataset sin coautores falla AL SEMBRAR y nombrando la
// obra, en vez de mas tarde y en la cara de quien lee el catalogo. Es la misma
// razon por la que la lectura falla en vez de servir la fila: contra este
// catalogo resuelve todo el matching.
//
// El resto del padron sigue por Store.Pool(): `titulares`, `usuarios`,
// `declaraciones`, `bolsas` y `parametros` no tienen todavia adaptador de
// escritura, y inventarle un puerto al sembrador para taparlo seria
// indireccion sin requisito.
func registrarObras(ctx context.Context, store *postgres.Store, obras []Obra) error {
	for _, o := range obras {
		obra, err := repertorio.NuevaObra(o.ID, repertorio.Metadatos{
			Titulo:    o.Titulo,
			Genero:    o.Genero,
			Anio:      o.Anio,
			Tipo:      o.Tipo,
			IDA:       o.IDA,
			EIDR:      o.EIDR,
			IMDB:      o.IMDB,
			Coautores: o.Coautores,
		})
		if err != nil {
			return fmt.Errorf("la obra %s del dataset no es una obra valida: %w", o.ID, err)
		}
		if err := store.Registrar(ctx, obra); err != nil {
			return fmt.Errorf("registrar obra %s: %w", o.ID, err)
		}
	}
	return nil
}

// usosCrudos quita la identificacion del dataset para que pase H5: la
// cascada (ADR 0007) es el unico camino a obra_id por GuardarUsos.
func usosCrudos(usos []aplicacion.UsoPersistido) []aplicacion.UsoPersistido {
	out := make([]aplicacion.UsoPersistido, len(usos))
	for i, u := range usos {
		u.ObraID = ""
		u.Escalon = ""
		u.Evidencia = ""
		u.ONI = false
		out[i] = u
	}
	return out
}

// identificar aplica el match sintetico DESPUES de la ingesta. El id de
// cada fila es el que GuardarUsos deriva (reporte + posicion en el lote).
func identificar(ctx context.Context, pool *pgxpool.Pool, reporteID string, usos []aplicacion.UsoPersistido) error {
	for n, u := range usos {
		id := reporteID + "-" + strconv.Itoa(n)
		tag, err := pool.Exec(ctx, `
			UPDATE usos
			   SET obra_id = $2,
			       oni = false,
			       escalon = 'alias',
			       evidencia = $3,
			       puntaje = 1
			 WHERE id = $1`,
			id, u.ObraID, u.Evidencia)
		if err != nil {
			return fmt.Errorf("uso %s: %w", id, err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("uso %s: no estaba en usos", id)
		}
	}
	return nil
}
