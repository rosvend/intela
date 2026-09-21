package aplicacion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Parametros son los casos de uso sobre los parametros normativos del ADR
// 0004: consultarlos, congelar el corte que una corrida va a usar, y volver a
// leer uno ya congelado.
//
// Es un servicio de tres operaciones y de una linea cada una. Eso es
// deliberado y no un envoltorio vacio: httpapi/doc.go deja escrito que "cada
// lectura tiene que tener su caso de uso, aunque al principio muchos sean de
// una linea", porque es lo que hace que la autorizacion y el asiento tengan
// donde vivir el dia que hagan falta. Antes de esa regla habia trece handlers
// consultando la base, y los parametros normativos en crudo eran uno de ellos.
//
// # El reloj entra aqui y no en el adaptador
//
// [ParametrosNormativos.Vigentes] recibe el instante como parametro, igual que
// todo lo demas del nucleo (ADR 0002): quien lo obtiene es este caso de uso,
// por el puerto [Reloj]. Un time.Now() dentro del adaptador haria que la lista
// dependiera de la hora del servidor de base de datos.
//
// # Lo que este servicio NO hace
//
// No escribe en `parametros`. Cargar una vigencia nueva es un hecho normativo
// -- lo aprueba un organo, en un acta -- y su camino es el mismo que el de
// [Recaudo.Registrar]: un puerto de escritura aparte y un asiento de bitacora
// en la misma transaccion (ADR 0006), porque un porcentaje de deduccion
// cambiado sin rastro es una cifra que nadie puede explicar en auditoria.
// Mientras ese puerto no exista, las filas entran por el sembrador con SQL
// directo; esta escrito en semilla/cargar.go, donde se decide.
//
// Y no calcula. El motor recibe el snapshot como argumento y no ve este
// servicio (ADR 0005): si el valor no viene en el snapshot, el codigo no
// compila.
type Parametros struct {
	Fuente ParametrosNormativos
	Reloj  Reloj
}

// Vigentes lista lo que rige ahora, con su organo aprobador y su reglamento.
//
// Es la lectura de administracion: la respuesta a "de donde salio este numero"
// para el dia de hoy. La de una corrida ya hecha NO se pregunta aqui -- se
// pregunta con [Parametros.Congelado] sobre su snapshot --, porque lo vigente
// hoy y lo que se aplico entonces son dos cosas distintas y confundirlas es
// justo el error que el ADR 0004 existe para impedir.
func (p Parametros) Vigentes(ctx context.Context) ([]FilaParametro, error) {
	filas, err := p.Fuente.Vigentes(ctx, p.Reloj.Ahora())
	if err != nil {
		return nil, fmt.Errorf("parametros vigentes: %w", err)
	}
	return filas, nil
}

// Congelar resuelve los parametros a la fecha del periodo y fija el corte que
// la corrida va a usar. Devuelve el id del snapshot y el snapshot.
//
// Se llama UNA VEZ, al abrir el proceso, y el id se guarda en `procesos`. Un
// recalculo posterior lee ese id con [Parametros.Congelado]; no vuelve a pasar
// por aqui. Volver a pasar seria resolver contra el estado actual de las
// vigencias, y entonces una fila cargada la semana pasada cambiaria la cifra
// de una corrida cerrada sin que nadie tocara la corrida.
//
// Es idempotente por construccion: el id esta direccionado por contenido, asi
// que congelar dos veces la misma fecha sobre los mismos valores devuelve el
// mismo id y no deja un segundo snapshot. Si entre las dos llamadas cambio un
// valor, el id es OTRO -- que es la respuesta correcta, no un fallo.
//
// La fecha es la del PERIODO, no la del reloj. Por eso entra como parametro y
// no se toma de [Reloj]: repartir en marzo el periodo de enero tiene que usar
// lo que regia en enero.
func (p Parametros) Congelar(ctx context.Context, fechaPeriodo time.Time) (string, reparto.Snapshot, error) {
	// Sin esta guarda, una fecha cero resuelve contra el ano 1 y el fallo que
	// sale es "faltan las trece clausulas", que manda a cargar parametros a
	// quien lo que tiene es una fecha sin rellenar.
	if fechaPeriodo.IsZero() {
		return "", reparto.Snapshot{}, errors.New("congelar parametros: falta la fecha del periodo")
	}

	id, snap, err := p.Fuente.SnapshotEnFecha(ctx, fechaPeriodo)
	if err != nil {
		// Sin envolver: quien llama distingue ErrParametroAusente con
		// errors.Is y las claves que faltan con errors.As, y el adaptador ya
		// le puso la fecha.
		return "", reparto.Snapshot{}, err
	}
	return id, snap, nil
}

// Congelado recupera un snapshot ya fijado. Es la lectura que hace reproducible
// una corrida (ADR 0005).
//
// Devuelve ErrNoEncontrado si ese id no se congelo nunca, y ErrSnapshotCorrupto
// si lo que hay bajo el id no es el snapshot que el id anuncia. Lo segundo no
// se degrada a lo primero a proposito: "no existe" invita a volver a resolver
// la fecha, y eso produciria una cifra distinta de la que se pago.
func (p Parametros) Congelado(ctx context.Context, id string) (reparto.Snapshot, error) {
	snap, err := p.Fuente.SnapshotPorID(ctx, id)
	if err != nil {
		return reparto.Snapshot{}, err
	}
	return snap, nil
}
