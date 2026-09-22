package postgres

import (
	"context"
	"fmt"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioProcesos = (*Store)(nil)

// GuardarProceso crea o actualiza la fila de `procesos`. circuito, bolsa_id,
// snapshot_id y reglamento se fijan al abrir y no cambian despues -- el
// UPDATE solo toca lo que una transicion de RD 13.5 mueve: etapa, revision
// y rechazo.
func (s *Store) GuardarProceso(ctx context.Context, p aplicacion.ProcesoVista) error {
	_, err := s.ejecutorDe(ctx).Exec(ctx,
		`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento, revision, rechazo)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (id) DO UPDATE SET etapa = EXCLUDED.etapa, revision = EXCLUDED.revision, rechazo = EXCLUDED.rechazo`,
		p.ID, string(p.Circuito), string(p.Etapa), p.Periodo, p.BolsaID, p.SnapshotID, p.Reglamento, p.Revision, p.RechazoMotivo,
	)
	return traducirError(err, "guardar proceso %q", p.ID)
}

// ProcesoPorID relee un proceso con TODAS sus firmas, de cualquier revision:
// [reparto.ProcesoDeReparto.Firmar] y [reparto.ProcesoDeReparto.AvanzarEtapa]
// ya filtran por SobreRev == Revision, asi que traer el historial completo
// no cambia el comportamiento y deja la fila explicable ante una auditoria
// (RD 16) sin una segunda consulta.
func (s *Store) ProcesoPorID(ctx context.Context, id string) (aplicacion.ProcesoVista, error) {
	var v aplicacion.ProcesoVista
	var circuito, etapa string
	err := s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento, revision, rechazo
		   FROM procesos WHERE id = $1`,
		id,
	).Scan(&v.ID, &circuito, &etapa, &v.Periodo, &v.BolsaID, &v.SnapshotID, &v.Reglamento, &v.Revision, &v.RechazoMotivo)
	if err != nil {
		return aplicacion.ProcesoVista{}, traducirError(err, "leer proceso %q", id)
	}
	v.Circuito = reparto.Circuito(circuito)
	v.Etapa = reparto.Etapa(etapa)

	firmas, err := firmasDe(ctx, s.ejecutorDe(ctx), id)
	if err != nil {
		return aplicacion.ProcesoVista{}, err
	}
	v.Firmas = firmas
	return v, nil
}

// ListarProcesos trae todos los procesos y sus firmas en dos consultas, no
// N+1: una por proceso y otra para las firmas de todos, agrupadas en
// memoria.
func (s *Store) ListarProcesos(ctx context.Context) ([]aplicacion.ProcesoVista, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento, revision, rechazo
		   FROM procesos ORDER BY periodo, id`)
	if err != nil {
		return nil, traducirError(err, "listar procesos")
	}
	defer filas.Close()

	var lista []aplicacion.ProcesoVista
	for filas.Next() {
		var v aplicacion.ProcesoVista
		var circuito, etapa string
		if err := filas.Scan(&v.ID, &circuito, &etapa, &v.Periodo, &v.BolsaID, &v.SnapshotID, &v.Reglamento, &v.Revision, &v.RechazoMotivo); err != nil {
			return nil, traducirError(err, "escanear procesos")
		}
		v.Circuito = reparto.Circuito(circuito)
		v.Etapa = reparto.Etapa(etapa)
		lista = append(lista, v)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "listar procesos")
	}

	firmasPorProceso, err := todasLasFirmas(ctx, s.ejecutorDe(ctx))
	if err != nil {
		return nil, err
	}
	for i := range lista {
		lista[i].Firmas = firmasPorProceso[lista[i].ID]
	}
	return lista, nil
}

// GuardarFirma inserta una firma. La clave (proceso_id, rol, revision) y
// `firma_actor_unico_por_revision` son la misma barrera que ya aplica
// [reparto.ProcesoDeReparto.Firmar] en el dominio; aqui es el respaldo de la
// base, no la primera linea de defensa.
func (s *Store) GuardarFirma(ctx context.Context, procesoID string, f reparto.Firma) error {
	_, err := s.ejecutorDe(ctx).Exec(ctx,
		`INSERT INTO firmas (proceso_id, rol, revision, actor_id) VALUES ($1,$2,$3,$4)`,
		procesoID, f.Rol, f.SobreRev, f.ActorID,
	)
	if esClaveForanea(err) {
		return fmt.Errorf("guardar firma de %q: %w", procesoID, aplicacion.ErrNoEncontrado)
	}
	return traducirError(err, "guardar firma de %q", procesoID)
}

// firmasDe lee las firmas de UN proceso, ordenadas para ser reproducibles.
func firmasDe(ctx context.Context, q consultor, procesoID string) ([]reparto.Firma, error) {
	filas, err := q.Query(ctx,
		`SELECT rol, revision, actor_id FROM firmas WHERE proceso_id = $1 ORDER BY revision, rol`, procesoID)
	if err != nil {
		return nil, traducirError(err, "leer firmas de %q", procesoID)
	}
	defer filas.Close()

	var firmas []reparto.Firma
	for filas.Next() {
		var f reparto.Firma
		if err := filas.Scan(&f.Rol, &f.SobreRev, &f.ActorID); err != nil {
			return nil, traducirError(err, "escanear firmas de %q", procesoID)
		}
		firmas = append(firmas, f)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "leer firmas de %q", procesoID)
	}
	return firmas, nil
}

// todasLasFirmas lee las firmas de TODOS los procesos en una sola consulta,
// para que [Store.ListarProcesos] no dispare una por fila.
func todasLasFirmas(ctx context.Context, q consultor) (map[string][]reparto.Firma, error) {
	filas, err := q.Query(ctx, `SELECT proceso_id, rol, revision, actor_id FROM firmas ORDER BY proceso_id, revision, rol`)
	if err != nil {
		return nil, traducirError(err, "leer firmas")
	}
	defer filas.Close()

	porProceso := map[string][]reparto.Firma{}
	for filas.Next() {
		var procesoID string
		var f reparto.Firma
		if err := filas.Scan(&procesoID, &f.Rol, &f.SobreRev, &f.ActorID); err != nil {
			return nil, traducirError(err, "escanear firmas")
		}
		porProceso[procesoID] = append(porProceso[procesoID], f)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "leer firmas")
	}
	return porProceso, nil
}
