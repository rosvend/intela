package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.Calendario = (*Store)(nil)

// Pendientes devuelve los periodos cuya apertura ya llego y que siguen sin
// disparar.
//
// # La comparacion es de fecha, no de instante
//
// `fecha_apertura` es DATE y `hoy` es un instante. Comparar los dos directamente
// deja que PostgreSQL convierta la fecha a instante usando la zona horaria de
// la SESION, que depende del servidor y no del codigo: la misma consulta
// dispararia un dia distinto en dos despliegues. Por eso el instante se reduce
// aqui a una fecha y se compara fecha con fecha.
//
// Se reduce en UTC, que es lo que devuelve reloj.Sistema. Bogota va cinco horas
// por detras, asi que un periodo se considera vencido desde las 19:00 del dia
// anterior, hora local. Sobre una ventana de fechas que el Consejo Directivo
// fija en semanas (RD 12) y que una vez abierta sigue abierta hasta dispararse,
// esas horas no cambian nada; si algun dia el calendario pasa a tener
// granularidad horaria, esto hay que revisarlo.
//
// # El orden
//
// Por fecha de apertura y luego por periodo. Sin ORDER BY, dos pasadas del
// scheduler podrian encolar los mismos periodos en distinto orden, y el ADR
// 0005 pide que una corrida se reproduzca igual.
func (s *Store) Pendientes(ctx context.Context, hoy time.Time) ([]string, error) {
	filas, err := s.pool.Query(ctx,
		`SELECT periodo
		   FROM calendario
		  WHERE NOT disparado AND fecha_apertura <= $1::date
		  ORDER BY fecha_apertura, periodo`,
		hoy.UTC().Format(time.DateOnly))
	if err != nil {
		return nil, traducirError(err, "periodos pendientes")
	}
	defer filas.Close()

	var periodos []string
	for filas.Next() {
		var periodo string
		if err := filas.Scan(&periodo); err != nil {
			return nil, traducirError(err, "escanear periodo del calendario")
		}
		periodos = append(periodos, periodo)
	}
	// Un fallo a mitad de stream solo sale por aqui. Sin esto, una lista
	// truncada pasaria por lista completa y el periodo que faltara no se
	// dispararia este ano.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "periodos pendientes")
	}
	return periodos, nil
}

// MarcarDisparado deja constancia de que el periodo ya se encolo.
//
// Es idempotente: marcar dos veces el mismo periodo actualiza la misma fila al
// mismo valor. Lo que si es un error es marcar un periodo que no esta en el
// calendario -seria una fecha inventada por el codigo en vez de leida del dato
// que administra el Consejo Directivo (ADR 0004)-, y por eso cero filas
// afectadas sale como ErrNoEncontrado.
func (s *Store) MarcarDisparado(ctx context.Context, periodo string) error {
	etiqueta, err := s.pool.Exec(ctx,
		`UPDATE calendario SET disparado = TRUE WHERE periodo = $1`, periodo)
	if err != nil {
		return traducirError(err, "marcar disparado %q", periodo)
	}
	if etiqueta.RowsAffected() == 0 {
		return fmt.Errorf("marcar disparado %q: %w", periodo, aplicacion.ErrNoEncontrado)
	}
	return nil
}
