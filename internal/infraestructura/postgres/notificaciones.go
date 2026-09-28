package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/rosvend/intela/internal/aplicacion"
)

// destinoPortal es donde queda el aviso: la vista del titular que lista sus
// ordenes de pago (`GET /mis-liquidaciones`).
const destinoPortal = "/mis-liquidaciones"

// errAvisoFueraDeUnidad es lo que devuelve [avisoPortal.Notificar] sin una
// transaccion en curso. Un aviso confirmado por separado de la orden que
// anuncia puede sobrevivir a una orden revertida, y entonces el titular ve un
// plazo de R-10 corriendo sobre una orden que no existe.
var errAvisoFueraDeUnidad = errors.New(
	"aviso de portal fuera de una unidad de trabajo: tiene que confirmarse con la orden que anuncia")

// avisoPortal satisface [aplicacion.Notificador] poniendo el aviso a
// disposicion del titular en el portal: RD 13.8.8 cuenta como notificacion
// "la puesta a disposicion ... en la pagina web de la sociedad", y
// `notificaciones.via` admite `portal` desde la migracion 00001.
//
// Escribe en la MISMA transaccion que la orden (#193). Es lo que el adaptador
// de log que habia antes no podia dar: aquel devolvia un acuse sin que nada
// llegara a ningun sitio, y con el el plazo de R-10 empezaba a correr contra
// un titular que no tenia donde ver su liquidacion. Aqui el aviso existe si y
// solo si la orden existe, y la orden es visible en `/mis-liquidaciones` desde
// el mismo commit.
//
// El envio por correo de RD 13.2 es otro canal y otra issue (#55): cuando
// llegue, escribe su propia fila con `via = 'email'`.
type avisoPortal struct {
	s *Store
}

// AvisoPortal devuelve el notificador de portal sobre este *Store. Es un
// envoltorio y no un metodo Notificar del propio *Store por la misma razon que
// [Store.CatalogoObras]: el nombre del puerto no dice que canal usa, y el
// cableado de cmd/api tiene que poder decirlo.
func (s *Store) AvisoPortal() aplicacion.Notificador {
	return avisoPortal{s: s}
}

// Notificar inserta el aviso y devuelve su acuse.
//
// El acuse es la huella del contenido anunciado -- titular, corrida, asunto y
// cuerpo -- y no un id aleatorio: el cuerpo se reconstruye de la orden
// persistida, asi que la huella se puede recalcular anos despues y comprobar
// que lo que se puso a disposicion es lo que dice el libro (ADR 0006).
func (a avisoPortal) Notificar(ctx context.Context, aviso aplicacion.Aviso) (string, error) {
	tx, hay := txDe(ctx)
	if !hay {
		return "", fmt.Errorf("avisar a %s: %w", aviso.TitularID, errAvisoFueraDeUnidad)
	}
	suma := sha256.Sum256([]byte(
		aviso.TitularID + "\x00" + aviso.ProcesoID + "\x00" + aviso.Asunto + "\x00" + aviso.Cuerpo))
	acuse := "portal:" + hex.EncodeToString(suma[:])

	if _, err := tx.Exec(ctx,
		`INSERT INTO notificaciones (titular_id, proceso_id, via, destino, acuse)
		 VALUES ($1, $2, 'portal', $3, $4)`,
		aviso.TitularID, aviso.ProcesoID, destinoPortal, acuse); err != nil {
		return "", traducirError(err, "avisar a %s de %s", aviso.TitularID, aviso.ProcesoID)
	}
	return acuse, nil
}

var _ aplicacion.Notificador = avisoPortal{}
