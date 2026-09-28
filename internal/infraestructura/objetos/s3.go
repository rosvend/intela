package objetos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Provisional (ADR 0023): GOVERNANCE hasta que los datos dejen de ser sinteticos.
const modoRetencion = types.ObjectLockModeGovernance

// aniosRetencion: R-13 / RD 13.4, diez anos de conservacion.
const aniosRetencion = 10

// S3 es la boveda de produccion: un bucket con Object Lock y versionado (infra/modules/storage).
type S3 struct {
	Cliente *s3.Client
	Bucket  string
}

// NuevoS3 construye el adaptador con la cadena de credenciales por defecto del SDK.
func NuevoS3(ctx context.Context, bucket string) (S3, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return S3{}, fmt.Errorf("configurar el cliente de S3: %w", err)
	}
	return S3{Cliente: s3.NewFromConfig(cfg), Bucket: bucket}, nil
}

// Poner escribe solo si la clave esta libre (If-None-Match) y deja el objeto retenido.
func (s S3) Poner(ctx context.Context, clave string, datos []byte) error {
	if err := validarClave(clave); err != nil {
		return err
	}
	_, err := s.Cliente.PutObject(ctx, &s3.PutObjectInput{
		Bucket:                    aws.String(s.Bucket),
		Key:                       aws.String(clave),
		Body:                      bytes.NewReader(datos),
		IfNoneMatch:               aws.String("*"),
		ChecksumAlgorithm:         types.ChecksumAlgorithmSha256,
		ObjectLockMode:            modoRetencion,
		ObjectLockRetainUntilDate: aws.Time(time.Now().AddDate(aniosRetencion, 0, 0)),
	})
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
		return fmt.Errorf("%w: %q", aplicacion.ErrObjetoYaExiste, clave)
	}
	return err
}

// Obtener lee la version vigente; con If-None-Match cada clave tiene una sola.
func (s S3) Obtener(ctx context.Context, clave string) ([]byte, error) {
	if err := validarClave(clave); err != nil {
		return nil, err
	}
	out, err := s.Cliente.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(clave),
	})
	var noHay *types.NoSuchKey
	if errors.As(err, &noHay) {
		return nil, aplicacion.ErrNoEncontrado
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = out.Body.Close() }()
	return io.ReadAll(out.Body)
}

// Borrar pone un marcador de borrado: la version retenida queda como huerfana inerte (ADR 0023).
func (s S3) Borrar(ctx context.Context, clave string) error {
	if err := validarClave(clave); err != nil {
		return err
	}
	_, err := s.Cliente.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(clave),
	})
	return err
}

var _ aplicacion.AlmacenObjetos = S3{}
