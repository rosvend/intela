package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/afiliacion"
	"github.com/rosvend/intela/internal/dominio/anticipos"
)

// indiceUnVigente es el indice unico parcial de la migracion 00031: un solo
// anticipo en tramite o aprobado por titular (R-32). Se reconoce por su nombre
// y no solo por el SQLSTATE 23505: la clave primaria de `anticipos` es otra
// violacion de unicidad, y un id repetido es un generador roto, no un socio que
// ya tiene anticipo.
const indiceUnVigente = "anticipos_uno_vigente_por_titular"

// repoAnticipos es *Store visto por [aplicacion.RepositorioAnticipos].
//
// Envoltorio y no metodos del propio *Store por lo mismo que
// [Store.CatalogoObras] y [Store.AvisoPortal]: *Store ya tiene un Listar y un
// DeTitular (liquidaciones) y un PorID (catalogo) con otras firmas, y el
// puerto de anticipos usa esos mismos nombres.
type repoAnticipos struct {
	s *Store
}

var _ aplicacion.RepositorioAnticipos = repoAnticipos{}

// Anticipos devuelve el repositorio de anticipos sobre este *Store. Comparte
// la unidad de trabajo: cada metodo pide su ejecutor con [Store.ejecutorDe].
func (s *Store) Anticipos() aplicacion.RepositorioAnticipos {
	return repoAnticipos{s: s}
}

const columnasAnticipo = `id, titular_id, monto, saldo, estado, justificacion,
	clave_soporte, motivo_rechazo, solicitado, resuelto, resuelto_por,
	girado, girado_por, soporte_giro`

func escanearAnticipo(fila pgx.Row) (aplicacion.AnticipoRegistrado, error) {
	var (
		r           aplicacion.AnticipoRegistrado
		estado      string
		resuelto    *time.Time
		resueltoPor *string
		girado      *time.Time
		giradoPor   *string
	)
	if err := fila.Scan(
		&r.Anticipo.ID, &r.Anticipo.TitularID, &r.Anticipo.Monto, &r.Anticipo.Saldo, &estado,
		&r.Anticipo.Justificacion, &r.Anticipo.Soporte, &r.Anticipo.MotivoRechazo,
		&r.Solicitado, &resuelto, &resueltoPor,
		&girado, &giradoPor, &r.Anticipo.SoporteGiro,
	); err != nil {
		return aplicacion.AnticipoRegistrado{}, err
	}
	r.Anticipo.Estado = anticipos.Estado(estado)
	if resuelto != nil {
		r.Resuelto = *resuelto
	}
	if resueltoPor != nil {
		r.ResueltoPor = *resueltoPor
	}
	if girado != nil {
		r.Anticipo.Girado = true
		r.Girado = *girado
	}
	if giradoPor != nil {
		r.GiradoPor = *giradoPor
	}
	return r, nil
}

func (r repoAnticipos) listar(ctx context.Context, que, donde string, args ...any) ([]aplicacion.AnticipoRegistrado, error) {
	filas, err := r.s.ejecutorDe(ctx).Query(ctx,
		`SELECT `+columnasAnticipo+` FROM anticipos `+donde+` ORDER BY solicitado DESC, id`, args...)
	if err != nil {
		return nil, traducirError(err, "%s", que)
	}
	defer filas.Close()

	lista := []aplicacion.AnticipoRegistrado{}
	for filas.Next() {
		a, err := escanearAnticipo(filas)
		if err != nil {
			return nil, traducirError(err, "%s", que)
		}
		lista = append(lista, a)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "%s", que)
	}
	return lista, nil
}

// ClaseDe lee `titulares.clase`, que es lo que decide R-30.
func (r repoAnticipos) ClaseDe(ctx context.Context, titularID string) (afiliacion.Clase, error) {
	var clase string
	err := r.s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT clase FROM titulares WHERE id = $1`, titularID).Scan(&clase)
	if err != nil {
		return "", traducirError(err, "clase del titular %q", titularID)
	}
	return afiliacion.Clase(clase), nil
}

// EmbargoVigente dice si el titular tenia un embargo reportado y no levantado
// el dia civil de dia (RA 3.1.e). La vigencia es [reportado, levantado), en
// dias UTC como el resto de fechas civiles del sistema.
func (r repoAnticipos) EmbargoVigente(ctx context.Context, titularID string, dia time.Time) (bool, error) {
	var hay bool
	err := r.s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM embargos
		    WHERE titular_id = $1
		      AND reportado <= $2::date
		      AND (levantado IS NULL OR levantado > $2::date))`,
		titularID, diaUTC(dia)).Scan(&hay)
	if err != nil {
		return false, traducirError(err, "embargos del titular %q", titularID)
	}
	return hay, nil
}

// DerechosLiquidados es la base del 25% de RA 3.1.g (ADR 0027): la suma de las
// lineas de reparto del titular (`resultados_titular.importe`) de las corridas
// que entraron en alguna orden de pago SUYA enviada en (desde, hasta].
//
// Por que las lineas y no la orden:
//
//   - El neto de la orden ya lleva descontado el anticipo anterior (R-33):
//     sumarlo le bajaria el cupo a quien ya pago uno, como si esos derechos no
//     los hubiera generado.
//   - El neto de una orden que absorbio una diferida (R-11) lleva tambien el
//     de esa diferida, que es otra orden: contaria dos veces el mismo derecho
//     si la diferida cayo dentro de la ventana.
//
// Y por que solo las corridas que llegaron a una orden: una corrida que sigue
// en verificacion todavia puede cambiar, y los derechos "generados ... en los
// procesos de reparto" son los de un proceso que ya se liquido. Las bolsas
// accesorias (reserva liberada, rendimientos) no pasan por
// `resultados_titular`, asi que no cuentan como reparto ordinario.
//
// `IN` y no un JOIN: una corrida aparece en una sola orden por titular -la
// orden es por (titular, periodo, circuito) y la corrida es de un periodo y un
// circuito-, pero con el JOIN un dato inconsistente duplicaria lineas en
// silencio, y con `IN` no.
func (r repoAnticipos) DerechosLiquidados(ctx context.Context, titularID string, desde, hasta time.Time) (decimal.Decimal, error) {
	var total decimal.Decimal
	err := r.s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT COALESCE(SUM(rt.importe), 0)
		   FROM resultados_titular rt
		  WHERE rt.titular_id = $1
		    AND rt.proceso_id IN (
		          SELECT unnest(array_append(o.procesos, o.proceso_id))
		            FROM ordenes_pago o
		           WHERE o.titular_id = $1
		             AND o.enviada > $2::date
		             AND o.enviada <= $3::date)`,
		titularID, diaUTC(desde), diaUTC(hasta)).Scan(&total)
	if err != nil {
		return decimal.Zero, traducirError(err, "derechos liquidados de %q", titularID)
	}
	return total, nil
}

// DeTitular devuelve todos los anticipos del titular, en cualquier estado.
func (r repoAnticipos) DeTitular(ctx context.Context, titularID string) ([]aplicacion.AnticipoRegistrado, error) {
	return r.listar(ctx, fmt.Sprintf("anticipos de %q", titularID), `WHERE titular_id = $1`, titularID)
}

// Listar es el listado de administracion, todos los titulares.
func (r repoAnticipos) Listar(ctx context.Context) ([]aplicacion.AnticipoRegistrado, error) {
	return r.listar(ctx, "listar anticipos", ``)
}

// PorID devuelve un anticipo o [aplicacion.ErrNoEncontrado].
func (r repoAnticipos) PorID(ctx context.Context, id string) (aplicacion.AnticipoRegistrado, error) {
	a, err := escanearAnticipo(r.s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT `+columnasAnticipo+` FROM anticipos WHERE id = $1`, id))
	if err != nil {
		return aplicacion.AnticipoRegistrado{}, traducirError(err, "anticipo %q", id)
	}
	return a, nil
}

// Registrar inserta una solicitud nueva. Si el titular ya tiene otra en
// tramite o aprobada, el indice parcial de la 00031 rechaza la fila y aqui se
// traduce a [anticipos.ErrAnticipoVigente]: es lo que sostiene R-32 cuando dos
// solicitudes del mismo socio llegan a la vez y las dos pasaron la
// comprobacion del caso de uso.
func (r repoAnticipos) Registrar(ctx context.Context, a anticipos.Anticipo, cuando time.Time) error {
	_, err := r.s.ejecutorDe(ctx).Exec(ctx,
		`INSERT INTO anticipos (id, titular_id, monto, saldo, estado, justificacion,
		                        clave_soporte, motivo_rechazo, solicitado)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		a.ID, a.TitularID, a.Monto, a.Saldo, string(a.Estado), a.Justificacion,
		a.Soporte, a.MotivoRechazo, cuando)
	if err != nil {
		if esDuplicadoDe(err, indiceUnVigente) {
			return fmt.Errorf("registrar el anticipo %q de %q: %w", a.ID, a.TitularID, anticipos.ErrAnticipoVigente)
		}
		return traducirError(err, "registrar el anticipo %q", a.ID)
	}
	return nil
}

// Resolver escribe la aprobacion o el rechazo SOLO si el anticipo sigue
// `solicitado`. La condicion va en el WHERE y no en una lectura previa: dos
// decisiones simultaneas sobre la misma solicitud leerian las dos
// `solicitado`, y sin ella la segunda pisaria a la primera -una aprobacion
// convertida en rechazo despues de girada, o al reves-.
func (r repoAnticipos) Resolver(ctx context.Context, a anticipos.Anticipo, cuando time.Time, actorID string) error {
	ej := r.s.ejecutorDe(ctx)
	etiqueta, err := ej.Exec(ctx,
		`UPDATE anticipos
		    SET estado = $2, saldo = $3, clave_soporte = $4, motivo_rechazo = $5,
		        resuelto = $6, resuelto_por = $7
		  WHERE id = $1 AND estado = $8`,
		a.ID, string(a.Estado), a.Saldo, a.Soporte, a.MotivoRechazo, cuando, actorID,
		string(anticipos.EstadoSolicitado))
	if err != nil {
		return traducirError(err, "resolver el anticipo %q", a.ID)
	}
	if etiqueta.RowsAffected() == 1 {
		return nil
	}
	return r.porQueNoSeEscribio(ctx, a.ID, "resolver")
}

// RegistrarGiro escribe la transferencia SOLO si el anticipo sigue aprobado y
// sin girar, por lo mismo que [repoAnticipos.Resolver]: dos registros
// simultaneos del mismo giro no pueden quedar los dos.
func (r repoAnticipos) RegistrarGiro(ctx context.Context, a anticipos.Anticipo, cuando time.Time, actorID string) error {
	etiqueta, err := r.s.ejecutorDe(ctx).Exec(ctx,
		`UPDATE anticipos
		    SET girado = $2, girado_por = $3, soporte_giro = $4
		  WHERE id = $1 AND estado = $5 AND girado IS NULL`,
		a.ID, cuando, actorID, a.SoporteGiro, string(anticipos.EstadoAprobado))
	if err != nil {
		return traducirError(err, "registrar el giro del anticipo %q", a.ID)
	}
	if etiqueta.RowsAffected() == 1 {
		return nil
	}
	return r.porQueNoSeEscribio(ctx, a.ID, "registrar el giro de")
}

// PorDescontar devuelve el anticipo aprobado Y GIRADO del titular con su fila
// bloqueada (`FOR UPDATE`), o false si no tiene. Uno aprobado que tesoreria no
// ha girado no se descuenta (RA 3.2.4).
//
// Exige transaccion, igual que [Store.DiferidasDeTitular] y por lo mismo: dos
// liquidaciones del mismo socio que NO comparten el cerrojo de aviso del
// periodo -dos periodos distintos- leerian el mismo saldo y descontarian las
// dos sobre el. Con el cerrojo, la segunda espera el commit de la primera y lee
// el saldo que esta dejo.
func (r repoAnticipos) PorDescontar(ctx context.Context, titularID string) (anticipos.Anticipo, bool, error) {
	tx, hay := txDe(ctx)
	if !hay {
		return anticipos.Anticipo{}, false, fmt.Errorf("anticipo por descontar de %q: %w", titularID, errFueraDeUnidad)
	}
	a, err := escanearAnticipo(tx.QueryRow(ctx,
		`SELECT `+columnasAnticipo+` FROM anticipos
		  WHERE titular_id = $1 AND estado = $2 AND girado IS NOT NULL
		  FOR UPDATE`, titularID, string(anticipos.EstadoAprobado)))
	if errors.Is(err, pgx.ErrNoRows) {
		return anticipos.Anticipo{}, false, nil
	}
	if err != nil {
		return anticipos.Anticipo{}, false, traducirError(err, "anticipo por descontar de %q", titularID)
	}
	return a.Anticipo, true, nil
}

// Descontar escribe el saldo y el estado que dejo un descuento, solo si la
// fila sigue aprobada y con saldoAnterior. Es la segunda barrera detras del
// cerrojo de [repoAnticipos.PorDescontar]: si alguien descontara sin el, la
// escritura vieja no pisa a la nueva, falla.
func (r repoAnticipos) Descontar(ctx context.Context, a anticipos.Anticipo, saldoAnterior decimal.Decimal) error {
	etiqueta, err := r.s.ejecutorDe(ctx).Exec(ctx,
		`UPDATE anticipos SET saldo = $2, estado = $3
		  WHERE id = $1 AND estado = $4 AND saldo = $5`,
		a.ID, a.Saldo, string(a.Estado), string(anticipos.EstadoAprobado), saldoAnterior)
	if err != nil {
		return traducirError(err, "descontar el anticipo %q", a.ID)
	}
	if etiqueta.RowsAffected() == 1 {
		return nil
	}
	return r.porQueNoSeEscribio(ctx, a.ID, "descontar")
}

// porQueNoSeEscribio separa las dos razones por las que un UPDATE condicional
// no toca ninguna fila: el anticipo no existe (404) o existe en otro estado o
// con otro saldo (409). Las dos se confunden si solo se mira RowsAffected.
func (r repoAnticipos) porQueNoSeEscribio(ctx context.Context, id, accion string) error {
	var existe bool
	if err := r.s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM anticipos WHERE id = $1)`, id).Scan(&existe); err != nil {
		return traducirError(err, "%s el anticipo %q", accion, id)
	}
	if !existe {
		return fmt.Errorf("%s el anticipo %q: %w", accion, id, aplicacion.ErrNoEncontrado)
	}
	return fmt.Errorf("%s el anticipo %q: otra escritura lo movio antes: %w", accion, id, anticipos.ErrEstadoInvalido)
}

// esDuplicadoDe es [esClaveDuplicada] para una tabla con mas de una
// restriccion de unicidad: pregunta por el nombre, como [esClaveForaneaDe].
func esDuplicadoDe(err error, restriccion string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoUnicidad && pgErr.ConstraintName == restriccion
}

// diaUTC es el dia civil de t como lo escribe aplicacion (diaCivil): UTC.
func diaUTC(t time.Time) string {
	return t.UTC().Format(time.DateOnly)
}
