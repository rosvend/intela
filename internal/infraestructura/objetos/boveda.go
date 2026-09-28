package objetos

import (
	"context"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Boveda elige S3 si hay bucket (OBJECT_BUCKET) y Disco en dir si no, que es lo de desarrollo y compose.
func Boveda(ctx context.Context, bucket, dir string) (aplicacion.AlmacenObjetos, error) {
	if bucket == "" {
		return Disco{Dir: dir}, nil
	}
	return NuevoS3(ctx, bucket)
}
