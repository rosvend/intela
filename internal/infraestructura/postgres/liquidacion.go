package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioLiquidacion = (*Store)(nil)

const (
	columnasOrden = `id, proceso_id, titular_id, periodo, bruto, neto, estado, enviada, arrastres`
	claveSMMLV    = "smmlv"
)

func (s *Store) DeTitular(ctx context.Context, titularID string) ([]liquidacion.OrdenDePago, error) {
	filas, err := s.pool.Query(ctx,
		`SELECT `+columnasOrden+` FROM ordenes_pago
		 WHERE titular_id = $1
		 ORDER BY periodo, id`, titularID)
	if err != nil {
		return nil, traducirError(err, "liquidaciones de titular %q", titularID)
	}
	return escanearOrdenes(ctx, s, filas, "liquidaciones de titular %q", titularID)
}

func (s *Store) Listar(ctx context.Context) ([]liquidacion.OrdenDePago, error) {
	filas, err := s.pool.Query(ctx,
		`SELECT `+columnasOrden+` FROM ordenes_pago
		 ORDER BY periodo, titular_id, id`)
	if err != nil {
		return nil, traducirError(err, "listar liquidaciones")
	}
	return escanearOrdenes(ctx, s, filas, "listar liquidaciones")
}

func (s *Store) DeProceso(ctx context.Context, procesoID string) ([]liquidacion.OrdenDePago, error) {
	filas, err := s.pool.Query(ctx,
		`SELECT `+columnasOrden+` FROM ordenes_pago
		 WHERE proceso_id = $1
		 ORDER BY titular_id, id`, procesoID)
	if err != nil {
		return nil, traducirError(err, "liquidaciones del proceso %q", procesoID)
	}
	return escanearOrdenes(ctx, s, filas, "liquidaciones del proceso %q", procesoID)
}

func (s *Store) Guardar(ctx context.Context, ordenes []liquidacion.OrdenDePago) error {
	if len(ordenes) == 0 {
		return nil
	}
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		for _, o := range ordenes {
			if err := upsertOrden(ctx, tx, o); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertOrden(ctx context.Context, tx pgx.Tx, o liquidacion.OrdenDePago) error {
	arrastres := o.Arrastres
	if arrastres == nil {
		arrastres = []string{}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO ordenes_pago (
			id, proceso_id, titular_id, periodo, bruto, neto, estado, enviada, arrastres
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO UPDATE SET
			bruto     = EXCLUDED.bruto,
			neto      = EXCLUDED.neto,
			estado    = EXCLUDED.estado,
			arrastres = EXCLUDED.arrastres`,
		o.ID, o.ProcesoID, o.TitularID, o.Periodo,
		o.Bruto, o.Neto, string(o.Estado), o.EnviadaDia, arrastres)
	if err != nil {
		return traducirError(err, "guardar orden %q", o.ID)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM ordenes_pago_deducciones WHERE orden_id = $1`, o.ID); err != nil {
		return traducirError(err, "borrar deducciones de %q", o.ID)
	}
	for _, d := range o.Deducciones {
		if _, err := tx.Exec(ctx,
			`INSERT INTO ordenes_pago_deducciones (orden_id, concepto, monto)
			 VALUES ($1,$2,$3)`, o.ID, d.Concepto, d.Monto); err != nil {
			return traducirError(err, "guardar deduccion %q de %q", d.Concepto, o.ID)
		}
	}
	return nil
}

func (s *Store) DocumentosDe(ctx context.Context, titularID string) (liquidacion.Documentos, error) {
	todos, err := s.Documentos(ctx)
	if err != nil {
		return liquidacion.Documentos{}, err
	}
	return todos[titularID], nil
}

func (s *Store) Documentos(ctx context.Context) (map[string]liquidacion.Documentos, error) {
	filas, err := s.pool.Query(ctx,
		`SELECT titular_id, tipo FROM documentos_titular ORDER BY titular_id, tipo`)
	if err != nil {
		return nil, traducirError(err, "listar documentos de titulares")
	}
	defer filas.Close()

	docs := map[string]liquidacion.Documentos{}
	for filas.Next() {
		var titularID, tipo string
		if err := filas.Scan(&titularID, &tipo); err != nil {
			return nil, traducirError(err, "escanear documento de titular")
		}
		d := docs[titularID]
		switch tipo {
		case "rut":
			d.RUT = true
		case "certificacion_bancaria":
			d.CertificacionBancaria = true
		}
		docs[titularID] = d
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "listar documentos de titulares")
	}
	return docs, nil
}

func (s *Store) InsumoDeProceso(ctx context.Context, procesoID string) (aplicacion.InsumoLiquidacion, error) {
	var insumo aplicacion.InsumoLiquidacion
	insumo.ProcesoID = procesoID
	err := s.pool.QueryRow(ctx,
		`SELECT p.periodo, r.bruto, r.admin, r.social, r.reserva
		 FROM procesos p
		 JOIN resultados_proceso r ON r.proceso_id = p.id
		 WHERE p.id = $1`, procesoID).
		Scan(&insumo.Periodo, &insumo.Bruto, &insumo.Admin, &insumo.Social, &insumo.Reserva)
	if err != nil {
		return aplicacion.InsumoLiquidacion{}, traducirError(err, "insumo del proceso %q", procesoID)
	}

	filas, err := s.pool.Query(ctx, `
		SELECT obra_id, titular_id, ipi, porcentaje, importe
		FROM resultados_titular
		WHERE proceso_id = $1
		ORDER BY titular_id, obra_id`, procesoID)
	if err != nil {
		return aplicacion.InsumoLiquidacion{}, traducirError(err, "lineas del proceso %q", procesoID)
	}
	defer filas.Close()

	insumo.Titulares = []reparto.LineaTitular{}
	for filas.Next() {
		var linea reparto.LineaTitular
		if err := filas.Scan(&linea.ObraID, &linea.TitularID, &linea.IPI, &linea.Porcentaje, &linea.Importe); err != nil {
			return aplicacion.InsumoLiquidacion{}, traducirError(err, "escanear linea del proceso %q", procesoID)
		}
		insumo.Titulares = append(insumo.Titulares, linea)
	}
	if err := filas.Err(); err != nil {
		return aplicacion.InsumoLiquidacion{}, traducirError(err, "lineas del proceso %q", procesoID)
	}
	return insumo, nil
}

func (s *Store) SMMLVVigente(ctx context.Context, en time.Time) (decimal.Decimal, error) {
	var valor decimal.Decimal
	err := s.pool.QueryRow(ctx, `
		SELECT valor FROM parametros
		WHERE clave = $1
		  AND vigente_desde <= $2
		  AND (vigente_hasta IS NULL OR vigente_hasta > $2)`,
		claveSMMLV, en).Scan(&valor)
	if err != nil {
		traducido := traducirError(err, "smmlv vigente en %s", en.Format("2006-01-02"))
		if errors.Is(traducido, aplicacion.ErrNoEncontrado) {
			return decimal.Zero, fmt.Errorf("%w: %s", aplicacion.ErrParametroAusente, claveSMMLV)
		}
		return decimal.Zero, traducido
	}
	return valor, nil
}

func escanearOrdenes(ctx context.Context, s *Store, filas pgx.Rows, formato string, args ...any) ([]liquidacion.OrdenDePago, error) {
	defer filas.Close()

	ordenes := []liquidacion.OrdenDePago{}
	ids := make([]string, 0)
	for filas.Next() {
		o, err := escanearOrden(filas)
		if err != nil {
			return nil, traducirError(err, formato, args...)
		}
		ordenes = append(ordenes, o)
		ids = append(ids, o.ID)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, formato, args...)
	}

	deducciones, err := s.deduccionesDe(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range ordenes {
		ds := deducciones[ordenes[i].ID]
		if ds == nil {
			ds = []liquidacion.Deduccion{}
		}
		ordenes[i].Deducciones = ds
	}
	return ordenes, nil
}

func escanearOrden(fila pgx.Row) (liquidacion.OrdenDePago, error) {
	var (
		o       liquidacion.OrdenDePago
		estado  string
		enviada time.Time
	)
	err := fila.Scan(
		&o.ID, &o.ProcesoID, &o.TitularID, &o.Periodo,
		&o.Bruto, &o.Neto, &estado, &enviada, &o.Arrastres)
	if o.Arrastres == nil {
		o.Arrastres = []string{}
	}
	o.Estado = liquidacion.Estado(estado)
	o.EnviadaDia = enviada.Format("2006-01-02")
	return o, err
}

func (s *Store) deduccionesDe(ctx context.Context, ids []string) (map[string][]liquidacion.Deduccion, error) {
	out := map[string][]liquidacion.Deduccion{}
	if len(ids) == 0 {
		return out, nil
	}
	filas, err := s.pool.Query(ctx, `
		SELECT orden_id, concepto, monto
		FROM ordenes_pago_deducciones
		WHERE orden_id = ANY($1)
		ORDER BY orden_id, concepto`, ids)
	if err != nil {
		return nil, traducirError(err, "deducciones de ordenes")
	}
	defer filas.Close()

	for filas.Next() {
		var (
			ordenID string
			d       liquidacion.Deduccion
		)
		if err := filas.Scan(&ordenID, &d.Concepto, &d.Monto); err != nil {
			return nil, traducirError(err, "escanear deduccion")
		}
		out[ordenID] = append(out[ordenID], d)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "deducciones de ordenes")
	}
	return out, nil
}
