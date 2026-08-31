package semilla

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
)

// ErrBitacoraNoVacia: SEED_RESET se nego porque hay asientos o notificaciones.
//
// asientos y notificaciones tienen triggers BEFORE TRUNCATE (ADR 0006). El
// seed corre antes de que exista ningun asiento; si alguien ya asento, reset
// no es una opcion, es borrar el libro.
var ErrBitacoraNoVacia = errors.New("SEED_RESET rechazado: la bitacora no esta vacia")

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
// esta a medias, pide SEED_RESET. Con reset, vacia las tablas mutables y
// vuelve a escribir. No toca asientos ni notificaciones.
func Cargar(ctx context.Context, store *postgres.Store, almacen aplicacion.AlmacenObjetos, hasher aplicacion.Hasher, claves Claves, reset bool, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	pool := store.Pool()

	nObras, nReportes, err := recuento(ctx, pool)
	if err != nil {
		return err
	}

	if reset {
		if err := vaciar(ctx, pool); err != nil {
			return err
		}
	} else if nObras > 0 && nReportes > 0 {
		log.Info("dataset ya sembrado; pase SEED_RESET=true para recargar")
		return nil
	} else if nObras > 0 || nReportes > 0 {
		return fmt.Errorf("semilla a medias (%d obras, %d reportes): pase SEED_RESET=true", nObras, nReportes)
	}

	d := Construir()
	hashes, err := hashear(hasher, claves)
	if err != nil {
		return err
	}

	if err := insertarPadron(ctx, pool, d, hashes); err != nil {
		return err
	}

	ingesta := aplicacion.Ingesta{Reportes: store, Almacen: almacen}
	for _, r := range d.Reportes {
		rep, err := ingesta.GuardarReporte(ctx, r.Fuente, r.Periodo, r.Bytes)
		if err != nil {
			return fmt.Errorf("guardar reporte %q: %w", r.Fuente, err)
		}
		if _, err := ingesta.GuardarUsos(ctx, rep.ID, r.Usos); err != nil {
			return fmt.Errorf("guardar usos de %q: %w", r.Fuente, err)
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

func vaciar(ctx context.Context, pool *pgxpool.Pool) error {
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

func insertarPadron(ctx context.Context, pool *pgxpool.Pool, d Dataset, hashes map[string]string) error {
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

	for _, o := range d.Obras {
		if _, err := tx.Exec(ctx, `
			INSERT INTO obras (id, titulo, ida, eidr, imdb, tipo)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			o.ID, o.Titulo, o.IDA, o.EIDR, o.IMDB, o.Tipo); err != nil {
			return fmt.Errorf("insertar obra %s: %w", o.ID, err)
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
