package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/recaudo"
)

var (
	_ aplicacion.RepositorioRecaudo = (*Store)(nil)
	_ aplicacion.GestionRecaudo     = (*Store)(nil)
)

const columnasBolsa = `id, usuario_id, periodo, circuito, bruto, convenio, tarifa, factura`

const columnasUsuarioRecaudo = `id, nombre, nit, categoria`

// AsientoRecaudo es el payload del asiento que deja el alta de una bolsa.
//
// Lleva la procedencia entera -- convenio, tarifa, factura -- porque la
// pregunta 1 del ADR 0006 es de donde salio el dinero, y la respuesta tiene que
// poder leerse del asiento sin volver a consultar la fila, que para entonces
// puede haber cambiado.
type AsientoRecaudo struct {
	UsuarioID string `json:"usuario_id"`
	Periodo   string `json:"periodo"`
	Circuito  string `json:"circuito"`
	Bruto     string `json:"bruto"`
	Convenio  string `json:"convenio"`
	Tarifa    string `json:"tarifa"`
	Factura   string `json:"factura"`
}

// RegistrarBolsa da de alta lo cobrado a un usuario en un periodo.
//
// La bolsa y su asiento entran en la MISMA transaccion: una bolsa sin asiento
// es dinero que entro sin que nadie pueda decir de donde salio.
func (s *Store) RegistrarBolsa(ctx context.Context, b aplicacion.BolsaPersistida, ahora time.Time, actorID string) error {
	// La columna de `asientos.cuando` es TIMESTAMPTZ, que guarda microsegundos.
	// Truncar aqui deja el valor escrito igual al que se compara despues.
	ahora = ahora.Truncate(time.Microsecond)

	return s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		// El bruto viaja como texto con cast a numeric: pasarlo como float64
		// seria meter binario de coma flotante en el unico sitio del sistema
		// donde el ADR 0005 exige aritmetica decimal exacta.
		_, err := tx.Exec(ctx,
			`INSERT INTO bolsas (`+columnasBolsa+`)
			 VALUES ($1, $2, $3, $4, $5::text::numeric, $6, $7, $8)`,
			b.ID, b.UsuarioID, b.Periodo, string(b.Circuito), b.Bruto.String(),
			b.Convenio, b.Tarifa, b.Factura)
		if err != nil {
			// Cada llamada decide que significa un duplicado o una foranea EN
			// SU tabla, antes de pasar por traducirError.
			if esClaveDuplicada(err) {
				return fmt.Errorf("registrar la bolsa %q: %w", b.ID, aplicacion.ErrBolsaDuplicada)
			}
			if esClaveForaneaDe(err, "bolsas_usuario_id_fkey") {
				return fmt.Errorf("registrar la bolsa %q del usuario %q: %w",
					b.ID, b.UsuarioID, aplicacion.ErrUsuarioRecaudoInexistente)
			}
			return traducirError(err, "registrar la bolsa %q", b.ID)
		}

		payload, err := json.Marshal(AsientoRecaudo{
			UsuarioID: b.UsuarioID,
			Periodo:   b.Periodo,
			Circuito:  string(b.Circuito),
			Bruto:     b.Bruto.String(),
			Convenio:  b.Convenio,
			Tarifa:    b.Tarifa,
			Factura:   b.Factura,
		})
		if err != nil {
			return fmt.Errorf("serializar el asiento de la bolsa %q: %w", b.ID, err)
		}

		return asentar(ctx, tx, aplicacion.Asiento{
			Hecho:   "recaudo.registrado",
			RefTipo: "bolsa",
			RefID:   b.ID,
			ActorID: actorID,
			Payload: payload,
			Cuando:  ahora,
		})
	})
}

// RegistrarUsuario da de alta un pagador.
func (s *Store) RegistrarUsuario(ctx context.Context, u recaudo.Usuario, ahora time.Time, actorID string) error {
	ahora = ahora.Truncate(time.Microsecond)
	d := u.Datos()

	return s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO usuarios_recaudo (`+columnasUsuarioRecaudo+`) VALUES ($1, $2, $3, $4)`,
			u.ID(), d.Nombre, d.NIT, string(d.Categoria))
		if err != nil {
			if esClaveDuplicada(err) {
				return fmt.Errorf("dar de alta el usuario de recaudo %q: %w",
					u.ID(), aplicacion.ErrUsuarioDeRecaudoDuplicado)
			}
			return traducirError(err, "dar de alta el usuario de recaudo %q", u.ID())
		}

		payload, err := json.Marshal(map[string]string{
			"nombre":    d.Nombre,
			"nit":       d.NIT,
			"categoria": string(d.Categoria),
		})
		if err != nil {
			return fmt.Errorf("serializar el asiento del usuario %q: %w", u.ID(), err)
		}

		return asentar(ctx, tx, aplicacion.Asiento{
			Hecho:   "recaudo.usuario_dado_de_alta",
			RefTipo: "usuario_recaudo",
			RefID:   u.ID(),
			ActorID: actorID,
			Payload: payload,
			Cuando:  ahora,
		})
	})
}

// ListarBolsas devuelve todas las bolsas.
func (s *Store) ListarBolsas(ctx context.Context) ([]aplicacion.BolsaPersistida, error) {
	// ORDER BY id: el ADR 0005 exige que una corrida se reproduzca bit a bit,
	// y una lista sin orden explicito no lo es -PostgreSQL no promete ninguno.
	filas, err := s.ejecutorDe(ctx).Query(ctx, `SELECT `+columnasBolsa+` FROM bolsas ORDER BY id`)
	if err != nil {
		return nil, traducirError(err, "listar bolsas")
	}
	return escanearBolsas(filas, "listar bolsas")
}

// BolsasDePeriodo devuelve las bolsas de un periodo. Sin coincidencias devuelve
// una lista vacia, no ErrNoEncontrado: la consulta fue bien y no hay filas.
func (s *Store) BolsasDePeriodo(ctx context.Context, periodo string) ([]aplicacion.BolsaPersistida, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT `+columnasBolsa+` FROM bolsas WHERE periodo = $1 ORDER BY id`, periodo)
	if err != nil {
		return nil, traducirError(err, "listar bolsas del periodo %q", periodo)
	}
	return escanearBolsas(filas, "listar bolsas del periodo %q", periodo)
}

// BolsaPorID devuelve una bolsa, o ErrNoEncontrado.
func (s *Store) BolsaPorID(ctx context.Context, id string) (aplicacion.BolsaPersistida, error) {
	var b aplicacion.BolsaPersistida
	var circuito string

	err := s.ejecutorDe(ctx).QueryRow(ctx, `SELECT `+columnasBolsa+` FROM bolsas WHERE id = $1`, id).
		Scan(&b.ID, &b.UsuarioID, &b.Periodo, &circuito, &b.Bruto, &b.Convenio, &b.Tarifa, &b.Factura)
	if err != nil {
		return aplicacion.BolsaPersistida{}, traducirError(err, "bolsa %q", id)
	}
	b.Circuito = recaudo.Circuito(circuito)
	return b, nil
}

// ListarUsuarios devuelve los pagadores dados de alta.
func (s *Store) ListarUsuarios(ctx context.Context) ([]recaudo.Usuario, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT `+columnasUsuarioRecaudo+` FROM usuarios_recaudo ORDER BY id`)
	if err != nil {
		return nil, traducirError(err, "listar usuarios de recaudo")
	}
	defer filas.Close()

	usuarios := make([]recaudo.Usuario, 0)
	for filas.Next() {
		var id, nombre, nit, categoria string
		if err := filas.Scan(&id, &nombre, &nit, &categoria); err != nil {
			return nil, traducirError(err, "escanear usuario de recaudo")
		}
		// Se reconstruye con el constructor del dominio, no rellenando campos:
		// lo que sale de la base cumple asi las mismas invariantes que lo que
		// entro por el caso de uso. Si una fila antigua dejara de cumplirlas,
		// se ve aqui y no tres capas mas arriba.
		u, err := recaudo.NuevoUsuario(id, recaudo.Datos{
			Nombre:    nombre,
			NIT:       nit,
			Categoria: recaudo.CategoriaUsuario(categoria),
		})
		if err != nil {
			return nil, fmt.Errorf("reconstruir el usuario de recaudo %q: %w", id, err)
		}
		usuarios = append(usuarios, u)
	}
	// No es opcional: un fallo a mitad de stream sale por aqui, y sin esta
	// comprobacion una lista TRUNCADA se devuelve como lista completa.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "listar usuarios de recaudo")
	}
	return usuarios, nil
}

// escanearBolsas consume un cursor de bolsas. Las dos lecturas de lista piden
// las mismas columnas en el mismo orden, asi que el escaneo es uno solo.
func escanearBolsas(filas pgx.Rows, formato string, args ...any) ([]aplicacion.BolsaPersistida, error) {
	defer filas.Close()

	bolsas := make([]aplicacion.BolsaPersistida, 0)
	for filas.Next() {
		var b aplicacion.BolsaPersistida
		var circuito string
		if err := filas.Scan(&b.ID, &b.UsuarioID, &b.Periodo, &circuito,
			&b.Bruto, &b.Convenio, &b.Tarifa, &b.Factura); err != nil {
			return nil, traducirError(err, "escanear bolsa")
		}
		b.Circuito = recaudo.Circuito(circuito)
		bolsas = append(bolsas, b)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, formato, args...)
	}
	return bolsas, nil
}
