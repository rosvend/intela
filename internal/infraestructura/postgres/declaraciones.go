package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

var _ aplicacion.GestionDeclaraciones = (*Store)(nil)

const columnasParteEscritura = `titular_id, ipi, porcentaje`

// Guardar cierra la version abierta de la obra -si la hay- y abre una nueva
// con las partes que llegan, todo en una sola transaccion.
//
// "Declarar por primera vez" y "editar" son la misma operacion aqui: la unica
// diferencia es si existia una fila en declaracion_versiones con
// vigente_hasta IS NULL para cerrar antes. El EXCLUDE de la migracion
// 00007 es la ultima linea de defensa contra un solape; esta funcion nunca
// deja dos versiones abiertas por su cuenta.
func (s *Store) Guardar(ctx context.Context, d repertorio.Declaracion, ahora time.Time) (int, error) {
	var version int
	err := s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		var versionAnterior int
		err := tx.QueryRow(ctx,
			`SELECT version FROM declaracion_versiones WHERE obra_id = $1 AND vigente_hasta IS NULL`,
			d.ObraID).Scan(&versionAnterior)
		switch {
		case err == nil:
			version = versionAnterior + 1
			if _, err := tx.Exec(ctx,
				`UPDATE declaracion_versiones SET vigente_hasta = $3 WHERE obra_id = $1 AND version = $2`,
				d.ObraID, versionAnterior, ahora); err != nil {
				return traducirError(err, "cerrar la version %d de la obra %q", versionAnterior, d.ObraID)
			}
		case errors.Is(err, pgx.ErrNoRows):
			// No habia ninguna version abierta: esta es la primera declaracion
			// de la obra.
			version = 1
		default:
			return traducirError(err, "buscar la version abierta de la obra %q", d.ObraID)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO declaracion_versiones (obra_id, version, vigente_desde) VALUES ($1, $2, $3)`,
			d.ObraID, version, ahora); err != nil {
			// La unica FK de esta tabla es obra_id -> obras: una violacion aqui
			// solo puede ser esa.
			if esClaveForanea(err) {
				return fmt.Errorf("abrir version %d de la obra %q: %w", version, d.ObraID, aplicacion.ErrNoEncontrado)
			}
			return traducirError(err, "abrir version %d de la obra %q", version, d.ObraID)
		}

		titulares := make([]string, len(d.Partes))
		ipis := make([]string, len(d.Partes))
		porcentajes := make([]string, len(d.Partes))
		for i, p := range d.Partes {
			titulares[i] = p.TitularID
			ipis[i] = p.IPI
			porcentajes[i] = p.Porcentaje.String()
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO declaraciones (obra_id, version, `+columnasParteEscritura+`)
			 SELECT $1, $2, * FROM unnest($3::text[], $4::text[], $5::text[]::numeric[])`,
			d.ObraID, version, titulares, ipis, porcentajes); err != nil {
			return traducirError(err, "escribir las partes de la version %d de la obra %q", version, d.ObraID)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return version, nil
}

// Historial devuelve todas las versiones de la declaracion de una obra, en
// orden. ORDER BY explicito por lo mismo que en repertorio.go: reproducible
// (ADR 0005).
func (s *Store) Historial(ctx context.Context, obraID string) ([]aplicacion.VersionDeclaracion, error) {
	filas, err := s.pool.Query(ctx,
		`SELECT version, vigente_desde, vigente_hasta FROM declaracion_versiones
		  WHERE obra_id = $1 ORDER BY version`, obraID)
	if err != nil {
		return nil, traducirError(err, "historial de declaraciones de la obra %q", obraID)
	}
	defer filas.Close()

	var versiones []aplicacion.VersionDeclaracion
	for filas.Next() {
		var vd aplicacion.VersionDeclaracion
		if err := filas.Scan(&vd.Version, &vd.VigenteDesde, &vd.VigenteHasta); err != nil {
			return nil, traducirError(err, "escanear version de la obra %q", obraID)
		}
		versiones = append(versiones, vd)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "historial de declaraciones de la obra %q", obraID)
	}

	for i := range versiones {
		partes, err := s.partesDeVersion(ctx, obraID, versiones[i].Version)
		if err != nil {
			return nil, err
		}
		versiones[i].Declaracion = repertorio.Declaracion{ObraID: obraID, Partes: partes}
	}
	return versiones, nil
}

// VigenteEn resuelve la version que regia en un instante dado: la unica cuya
// ventana [vigente_desde, vigente_hasta) contiene el momento pedido. El
// EXCLUDE de la migracion garantiza que esa fila, si existe, es unica.
func (s *Store) VigenteEn(ctx context.Context, obraID string, momento time.Time) (aplicacion.VersionDeclaracion, error) {
	var vd aplicacion.VersionDeclaracion
	err := s.pool.QueryRow(ctx,
		`SELECT version, vigente_desde, vigente_hasta FROM declaracion_versiones
		  WHERE obra_id = $1 AND vigente_desde <= $2 AND (vigente_hasta IS NULL OR vigente_hasta > $2)`,
		obraID, momento).Scan(&vd.Version, &vd.VigenteDesde, &vd.VigenteHasta)
	if err != nil {
		return aplicacion.VersionDeclaracion{}, traducirError(err, "declaracion vigente de la obra %q en %s", obraID, momento)
	}

	partes, err := s.partesDeVersion(ctx, obraID, vd.Version)
	if err != nil {
		return aplicacion.VersionDeclaracion{}, err
	}
	vd.Declaracion = repertorio.Declaracion{ObraID: obraID, Partes: partes}
	return vd, nil
}

// partesDeVersion lee las partes de UNA version concreta. Distinta de
// partesDeObra (repertorio.go), que solo lee la version vigente para el
// motor de reparto y el estado del catalogo.
func (s *Store) partesDeVersion(ctx context.Context, obraID string, version int) ([]repertorio.Parte, error) {
	filas, err := s.pool.Query(ctx,
		`SELECT `+columnasParteEscritura+` FROM declaraciones
		  WHERE obra_id = $1 AND version = $2 ORDER BY titular_id`,
		obraID, version)
	if err != nil {
		return nil, traducirError(err, "partes de la version %d de la obra %q", version, obraID)
	}
	defer filas.Close()

	var partes []repertorio.Parte
	for filas.Next() {
		var p repertorio.Parte
		if err := filas.Scan(&p.TitularID, &p.IPI, &p.Porcentaje); err != nil {
			return nil, traducirError(err, "escanear parte de la version %d de la obra %q", version, obraID)
		}
		partes = append(partes, p)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "partes de la version %d de la obra %q", version, obraID)
	}
	return partes, nil
}
