package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

var _ aplicacion.GestionDeclaraciones = (*Store)(nil)

const columnasParteEscritura = `titular_id, ipi, porcentaje`

// Guardar cierra la version abierta de la obra -si la hay- y abre una nueva
// con las partes que llegan, y asienta el hecho en la bitacora (ADR 0006),
// todo en una sola transaccion. Devuelve la version nueva y el vigente_desde
// que de verdad quedo escrito, que no siempre es el ahora que llego: ver el
// tercer parrafo.
//
// "Declarar por primera vez" y "editar" son la misma operacion aqui: la unica
// diferencia es si existia una fila en declaracion_versiones con
// vigente_hasta IS NULL para cerrar antes. El EXCLUDE de la migracion
// 00008 es la ultima linea de defensa contra un solape; esta funcion nunca
// deja dos versiones abiertas por su cuenta.
//
// El numero de la version nueva es el consecutivo del HISTORIAL -MAX(version)
// mas uno de esa obra, o 1 si no hay ninguna-, y no el de la version abierta
// mas uno: una obra puede tener versiones y NINGUNA abierta, y ese caso no es
// "la primera declaracion" aunque el SELECT de la version abierta no devuelva
// filas.
//
// El SELECT ... FOR UPDATE sobre `obras` bloquea la fila antes de mirar cual
// es la version abierta. Sin el, dos PUT concurrentes sobre la misma obra
// pueden leer la misma version abierta, calcular version+1 los dos, e
// intentar abrir la misma PK -uno de los dos pierde con un 500 aunque su
// declaracion fuera valida-. El lock tambien resuelve "la obra no existe"
// (ErrNoEncontrado) sin esperar a la FK del INSERT de mas abajo, que se deja
// como red de seguridad.
//
// ahora se trunca a microsegundo ANTES de compararlo con nada, no en el
// momento de escribir: TIMESTAMPTZ solo guarda microsegundos y pgx trunca al
// codificar sin avisar. Comparar en la resolucion de nanosegundo de Go contra
// un desdeAnterior que ya viene truncado deja pasar un ahora que en Go es
// estrictamente posterior pero cae en el MISMO microsegundo una vez escrito
// -el UPDATE de mas abajo terminaria con vigente_hasta == vigente_desde, y
// declaracion_vigencia_coherente rechazaria una edicion valida con un 500
// opaco-. Truncar primero hace que la comparacion ocurra en la resolucion
// real de la base, y que el valor que este metodo devuelve coincida siempre
// con el que quedo escrito.
//
// ahora tambien nunca queda menor o igual que el vigente_desde de la version
// que cierra: dos ediciones SECUENCIALES (no concurrentes: el FOR UPDATE no
// ayuda aqui, la primera ya confirmo) pueden pedir el mismo instante una vez
// truncado. Sin este ajuste, la version nueva abriria con el mismo
// vigente_desde que vigente_hasta de la que cierra, y
// declaracion_vigencia_coherente rechazaria esa edicion tambien.
//
// El asiento se escribe con la MISMA tx que la version: ADR 0006 dice que es
// parte de la definicion de hecho de esta operacion. Antes se escribia con
// una llamada aparte a BitacoraAuditoria.Asentar despues de confirmar esta
// transaccion, y un fallo ahi dejaba la version guardada sin asiento, sin
// forma de revertirla ni de saber que quedo huerfana.
func (s *Store) Guardar(ctx context.Context, d repertorio.Declaracion, ahora time.Time, actorID string) (int, time.Time, error) {
	ahora = ahora.Truncate(time.Microsecond)
	var version int
	err := s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		var existe string
		if err := tx.QueryRow(ctx, `SELECT id FROM obras WHERE id = $1 FOR UPDATE`, d.ObraID).
			Scan(&existe); err != nil {
			return traducirError(err, "bloquear la obra %q para declarar", d.ObraID)
		}

		var versionAnterior int
		var desdeAnterior time.Time
		err := tx.QueryRow(ctx,
			`SELECT version, vigente_desde FROM declaracion_versiones WHERE obra_id = $1 AND vigente_hasta IS NULL`,
			d.ObraID).Scan(&versionAnterior, &desdeAnterior)
		switch {
		case err == nil:
			version = versionAnterior + 1
			if !ahora.After(desdeAnterior) {
				ahora = desdeAnterior.Add(time.Microsecond)
			}
			if _, err := tx.Exec(ctx,
				`UPDATE declaracion_versiones SET vigente_hasta = $3 WHERE obra_id = $1 AND version = $2`,
				d.ObraID, versionAnterior, ahora); err != nil {
				return traducirError(err, "cerrar la version %d de la obra %q", versionAnterior, d.ObraID)
			}
		case errors.Is(err, pgx.ErrNoRows):
			// No habia ninguna version abierta. Eso NO es lo mismo que "esta
			// es la primera declaracion de la obra", que es lo que decia el
			// `version = 1` de antes: puede haber versiones anteriores y
			// todas cerradas -una edicion que cerro la ultima y no llego a
			// abrir la siguiente, un historial importado-, y entonces el 1
			// choca con la clave primaria (obra_id, version) y la edicion
			// muere con una violacion de unicidad que no dice nada de lo que
			// pasa. El consecutivo sale del HISTORIAL, no del numero de
			// versiones abiertas, que es 0 o 1.
			//
			// Va dentro del FOR UPDATE de `obras` que ya se tiene arriba: dos
			// guardados de la misma obra no pueden leer el mismo MAX, que es
			// lo que hace que este consecutivo no colisione.
			if err := tx.QueryRow(ctx,
				`SELECT COALESCE(MAX(version), 0) + 1 FROM declaracion_versiones WHERE obra_id = $1`,
				d.ObraID).Scan(&version); err != nil {
				return traducirError(err, "buscar la ultima version de la obra %q", d.ObraID)
			}
		default:
			return traducirError(err, "buscar la version abierta de la obra %q", d.ObraID)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO declaracion_versiones (obra_id, version, vigente_desde) VALUES ($1, $2, $3)`,
			d.ObraID, version, ahora); err != nil {
			// La unica FK de esta tabla es obra_id -> obras: una violacion aqui
			// solo puede ser esa. El FOR UPDATE de arriba deja esta rama
			// practicamente inalcanzable; se queda como red de seguridad.
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
			// declaraciones_titular_id_fkey (titular_id -> titulares) es la unica
			// de las dos FK de esta tabla que puede violarse aqui de verdad: la de
			// obra_id+version -> declaracion_versiones ya la satisface el INSERT
			// de la cabecera, unas lineas arriba, en la MISMA transaccion. Se
			// discrimina por nombre igual -no con esClaveForanea a secas- para no
			// traducir un fallo ajeno como "titular inexistente" si esa segunda FK
			// alguna vez se alcanza.
			if esClaveForaneaDe(err, "declaraciones_titular_id_fkey") {
				return fmt.Errorf("escribir las partes de la version %d de la obra %q: %w",
					version, d.ObraID, aplicacion.ErrTitularInexistente)
			}
			return traducirError(err, "escribir las partes de la version %d de la obra %q", version, d.ObraID)
		}

		payload, err := json.Marshal(aplicacion.AsientoDeclaracion{
			Version: version,
			Estado:  d.Estado(),
			Partes:  d.Partes,
		})
		if err != nil {
			return fmt.Errorf("serializar asiento de la obra %q: %w", d.ObraID, err)
		}
		if err := asentar(ctx, tx, aplicacion.Asiento{
			Hecho:   "declaracion.guardada",
			RefTipo: "obra",
			RefID:   d.ObraID,
			ActorID: actorID,
			Payload: payload,
			Cuando:  ahora,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, time.Time{}, err
	}
	return version, ahora, nil
}

// Historial devuelve una pagina de versiones de la declaracion de una obra,
// en orden ascendente. ORDER BY explicito por lo mismo que en repertorio.go:
// reproducible (ADR 0005).
//
// # La pagina va de la version mas RECIENTE hacia atras
//
// La version abierta es la de MAX(version), y quien lee el historial la
// necesita: `conciliarConElHistorial` (web) compara contra ella lo que dice el
// catalogo. Si la pagina por defecto empezara por la version 1, una obra con
// mas versiones que el limite dejaria a las dos pantallas sin version abierta
// y dirian "descuadrado" sin que haya descuadre. Con el desplazamiento se
// alcanzan las viejas: ninguna version queda inalcanzable, que es lo que hacia
// el error duro que esto reemplaza (a partir de la 501 no se servia NINGUNA).
// Dentro de la pagina el orden sigue siendo de mas antigua a mas reciente.
//
// [aplicacion.LimiteSinTope] pide el historial entero; solo tiene sentido en
// lecturas internas, la ruta HTTP lo rechaza.
//
// # Una sola consulta, y por que
//
// Antes eran 1 + N: el SELECT de las versiones y un `partesDeVersion` por cada
// una. Medido con `log_statement = 'all'`: 14 sentencias para una obra de 12
// versiones, con el SELECT de `declaraciones` repetido 12 veces. Ahora es una
// con el mismo LEFT JOIN que [Store.VigentesDeObras] ya usaba -el historial de
// la version sin partes del ejemplo es el de una version abierta y sin
// declaracion, y ahi el join tiene que devolver la version con las columnas de
// la parte en NULL, no saltarse la fila-.
//
// El LIMIT va sobre las VERSIONES, no sobre el resultado: puesto en la consulta
// de fuera recortaria filas de PARTES, y una version con tres coautores podria
// llegar con dos. De ahi la CTE.
func (s *Store) Historial(ctx context.Context, obraID string, pag aplicacion.Paginacion) ([]aplicacion.VersionDeclaracion, error) {
	pag = pag.ConDefecto()
	// LIMIT NULL es "sin limite" en PostgreSQL.
	var limite *int
	if pag.Limite != aplicacion.LimiteSinTope {
		limite = &pag.Limite
	}
	filas, err := s.pool.Query(ctx,
		`WITH versiones AS (
		     SELECT obra_id, version, vigente_desde, vigente_hasta
		       FROM declaracion_versiones
		      WHERE obra_id = $1
		      ORDER BY version DESC
		      LIMIT $2 OFFSET $3
		 )
		 SELECT v.version, v.vigente_desde, v.vigente_hasta, d.titular_id, d.ipi, d.porcentaje
		   FROM versiones v
		   LEFT JOIN declaraciones d
		     ON d.obra_id = v.obra_id AND d.version = v.version
		  ORDER BY v.version, d.titular_id`,
		obraID, limite, pag.Desplazamiento)
	if err != nil {
		return nil, traducirError(err, "historial de declaraciones de la obra %q", obraID)
	}
	defer filas.Close()

	var versiones []aplicacion.VersionDeclaracion
	for filas.Next() {
		var (
			version int
			desde   time.Time
			hasta   *time.Time
			titular *string
			ipi     *string
			pct     *decimal.Decimal
		)
		if err := filas.Scan(&version, &desde, &hasta, &titular, &ipi, &pct); err != nil {
			return nil, traducirError(err, "escanear version de la obra %q", obraID)
		}
		// El ORDER BY es por version, asi que las filas de una misma version
		// llegan juntas y en orden: la cabecera que hay que completar es la
		// ultima, y no hace falta un mapa por version.
		if len(versiones) == 0 || versiones[len(versiones)-1].Version != version {
			versiones = append(versiones, aplicacion.VersionDeclaracion{
				Version:      version,
				VigenteDesde: desde,
				VigenteHasta: hasta,
				Declaracion:  repertorio.Declaracion{ObraID: obraID},
			})
		}
		// Una version sin ninguna parte llega con las tres columnas en NULL: es
		// el LEFT JOIN, y se salta en vez de inventar una parte vacia.
		if titular != nil {
			ultima := &versiones[len(versiones)-1]
			ultima.Declaracion.Partes = append(ultima.Declaracion.Partes, repertorio.Parte{
				TitularID: *titular,
				// ipi y porcentaje son NOT NULL en la misma fila que
				// titular_id, asi que si la parte existe los tres existen.
				IPI:        *ipi,
				Porcentaje: *pct,
			})
		}
	}
	// No es opcional: un fallo a mitad de stream dejaria un historial TRUNCADO
	// pasando por completo.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "historial de declaraciones de la obra %q", obraID)
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

// VigentesDeObras lee la version abierta de cada una de las obras pedidas, con
// sus partes, en UNA consulta.
//
// Es la misma cuenta que partesDeObras (repertorio.go) para el otro extremo de
// la relacion: con N obras, preguntar la version vigente de cada una es N+1
// viajes contra la base. Aqui la fila base es `declaracion_versiones` -una por
// obra declarada-, y las partes entran por LEFT JOIN.
//
// # Por que LEFT JOIN y no INNER
//
// Porque la version es el ORIGEN del estado: una version abierta sin ninguna
// parte sigue siendo una version declarada, y con INNER JOIN desapareceria del
// resultado y se leeria como "esta obra no tiene declaracion" -que es
// exactamente la confusion que `version_vigente` existe para evitar-. Las
// columnas de la parte llegan a NULL en ese caso y se saltan al armar la
// lista: la version se reporta, la parte no.
//
// # Una obra sin ninguna version no sale
//
// Ausencia en el mapa es "no tiene declaracion", y el mapa no se rellena con
// ceros por obra pedida: una declaracion vacia daria el mismo Estado() que una
// sin declarar (`incompleta`, `R-04`) y el llamador perderia la distincion.
// Por eso lo que devuelve es el mapa tal como salio de la consulta, sin
// completar las obras que faltan.
func (s *Store) VigentesDeObras(ctx context.Context, obraIDs []string) (map[string]aplicacion.VersionDeclaracion, error) {
	// Un slice vacio no consulta: ANY('{}') devuelve cero filas, pero no hay
	// por que ir a la base a comprobarlo.
	if len(obraIDs) == 0 {
		return map[string]aplicacion.VersionDeclaracion{}, nil
	}

	// ORDER BY obra_id, titular_id: reproducibilidad (ADR 0005), y dentro de
	// cada obra las partes en el mismo orden que en el resto del paquete. La
	// version no entra en el orden porque no hace falta: el EXCLUDE de la
	// migracion 00008 garantiza una sola version abierta por obra, asi que
	// obra_id ya la determina.
	filas, err := s.pool.Query(ctx,
		`SELECT dv.obra_id, dv.version, dv.vigente_desde, d.titular_id, d.ipi, d.porcentaje
		   FROM declaracion_versiones dv
		   LEFT JOIN declaraciones d
		     ON d.obra_id = dv.obra_id AND d.version = dv.version
		  WHERE dv.vigente_hasta IS NULL AND dv.obra_id = ANY($1)
		  ORDER BY dv.obra_id, d.titular_id`, obraIDs)
	if err != nil {
		return nil, traducirError(err, "declaraciones vigentes de obras")
	}
	defer filas.Close()

	vigentes := map[string]aplicacion.VersionDeclaracion{}
	for filas.Next() {
		var (
			obraID  string
			version int
			desde   time.Time
			titular *string
			ipi     *string
			pct     *decimal.Decimal
		)
		if err := filas.Scan(&obraID, &version, &desde, &titular, &ipi, &pct); err != nil {
			return nil, traducirError(err, "escanear declaracion vigente")
		}
		// La cabecera se toma de la PRIMERA fila de la obra y las partes se
		// acumulan sobre la entrada que ya estaba: una version con dos partes
		// llega como dos filas -es el precio de traerlas todas en una consulta
		// plana-, y asignar la entrada en cada vuelta dejaria solo la ultima.
		vd, hay := vigentes[obraID]
		if !hay {
			vd = aplicacion.VersionDeclaracion{
				Version:      version,
				VigenteDesde: desde,
				Declaracion:  repertorio.Declaracion{ObraID: obraID},
			}
		}
		if titular != nil {
			vd.Declaracion.Partes = append(vd.Declaracion.Partes, repertorio.Parte{
				TitularID: *titular,
				// ipi y porcentaje son NOT NULL en la misma fila que
				// titular_id, asi que si la parte existe los tres existen.
				IPI:        *ipi,
				Porcentaje: *pct,
			})
		}
		vigentes[obraID] = vd
	}
	// No es opcional: un fallo a mitad de stream dejaria un mapa TRUNCADO
	// pasando por pagina completa.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "declaraciones vigentes de obras")
	}
	return vigentes, nil
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
