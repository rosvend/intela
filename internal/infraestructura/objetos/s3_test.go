package objetos

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/rosvend/intela/internal/aplicacion"
)

// imagenEmulador: LocalStack y no MinIO porque minio/minio ya no se puede descargar de Docker Hub.
const imagenEmulador = "localstack/localstack:4.9"

var (
	emuladorUnaVez sync.Once
	emuladorURL    string
	emuladorErr    error
	buckets        atomic.Int64
)

func clienteEmulador(t *testing.T) *s3.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("prueba de integracion: necesita Docker")
	}
	emuladorUnaVez.Do(func() {
		c, err := testcontainers.Run(context.Background(), imagenEmulador,
			testcontainers.WithExposedPorts("4566/tcp"),
			testcontainers.WithEnv(map[string]string{"SERVICES": "s3"}),
			testcontainers.WithWaitStrategy(wait.ForHTTP("/_localstack/health").WithPort("4566/tcp").
				WithStartupTimeout(2*time.Minute)),
		)
		if err != nil {
			emuladorErr = err
			return
		}
		emuladorURL, emuladorErr = c.PortEndpoint(context.Background(), "4566/tcp", "http")
	})
	if emuladorErr != nil {
		t.Fatalf("levantar el emulador de S3: %v", emuladorErr)
	}
	return s3.New(s3.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(emuladorURL),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	})
}

// bovedaDePrueba crea un bucket con Object Lock, como infra/modules/storage, y devuelve su adaptador.
func bovedaDePrueba(t *testing.T) S3 {
	t.Helper()
	cliente := clienteEmulador(t)
	bucket := fmt.Sprintf("boveda-%d", buckets.Add(1))
	_, err := cliente.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket:                     aws.String(bucket),
		ObjectLockEnabledForBucket: aws.Bool(true),
	})
	if err != nil {
		t.Fatalf("crear bucket: %v", err)
	}
	return S3{Cliente: cliente, Bucket: bucket}
}

func TestS3CumpleElContrato(t *testing.T) {
	probarContrato(t, func(t *testing.T) aplicacion.AlmacenObjetos { return bovedaDePrueba(t) })
}

// ADR 0023: cada objeto queda retenido en GOVERNANCE diez anos.
func TestS3PonerRetieneElObjeto(t *testing.T) {
	b := bovedaDePrueba(t)
	ctx := context.Background()
	clave := "reportes/abc"
	if err := b.Poner(ctx, clave, []byte("evidencia")); err != nil {
		t.Fatalf("Poner: %v", err)
	}

	ret, err := b.Cliente.GetObjectRetention(ctx, &s3.GetObjectRetentionInput{Bucket: &b.Bucket, Key: &clave})
	if err != nil {
		t.Fatalf("GetObjectRetention: %v", err)
	}
	if ret.Retention.Mode != types.ObjectLockRetentionModeGovernance {
		t.Fatalf("modo %q, se esperaba GOVERNANCE", ret.Retention.Mode)
	}
	if minimo := time.Now().AddDate(10, 0, -1); ret.Retention.RetainUntilDate.Before(minimo) {
		t.Fatalf("retenido hasta %v, se esperaba al menos %v", ret.Retention.RetainUntilDate, minimo)
	}
}

// Contra el bucket, no contra el codigo: la version retenida no se puede borrar.
func TestS3LaVersionRetenidaNoSeBorra(t *testing.T) {
	b := bovedaDePrueba(t)
	ctx := context.Background()
	clave := "afiliaciones/afil-1/rut"
	if err := b.Poner(ctx, clave, []byte("%PDF")); err != nil {
		t.Fatalf("Poner: %v", err)
	}
	if err := b.Borrar(ctx, clave); err != nil {
		t.Fatalf("Borrar: %v", err)
	}

	versiones, err := b.Cliente.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: &b.Bucket, Prefix: &clave})
	if err != nil {
		t.Fatalf("ListObjectVersions: %v", err)
	}
	if len(versiones.Versions) != 1 {
		t.Fatalf("Borrar tiene que dejar la version retenida como huerfana inerte; versiones: %d", len(versiones.Versions))
	}
	_, err = b.Cliente.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &b.Bucket, Key: &clave, VersionId: versiones.Versions[0].VersionId,
	})
	if err == nil {
		t.Fatal("el bucket dejo borrar una version retenida")
	}
}
