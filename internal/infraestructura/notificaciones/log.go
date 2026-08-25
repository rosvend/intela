// Package notificaciones adapta el puerto aplicacion.Notificador.
package notificaciones

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Bitacora no envia nada: registra la notificacion y devuelve un acuse
// derivado del contenido. Es el adaptador de desarrollo.
//
// El acuse es determinista a proposito: es lo que se guarda como prueba de
// que se notifico, y tiene que poder recalcularse anos despues.
type Bitacora struct {
	Log *slog.Logger
}

func (b Bitacora) Notificar(ctx context.Context, dest, asunto, cuerpo string) (string, error) {
	sum := sha256.Sum256([]byte(dest + "\x00" + asunto + "\x00" + cuerpo))
	acuse := hex.EncodeToString(sum[:12])

	log := b.Log
	if log == nil {
		log = slog.Default()
	}
	// El cuerpo no se registra: puede llevar cifras de liquidacion de una
	// persona concreta.
	log.InfoContext(ctx, "notificacion",
		slog.String("destino", dest),
		slog.String("asunto", asunto),
		slog.String("acuse", acuse),
	)
	return acuse, nil
}

var _ aplicacion.Notificador = Bitacora{}
