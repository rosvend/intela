package objetos

import (
	"context"
	"testing"
)

func TestBovedaSinBucketEsDisco(t *testing.T) {
	a, err := Boveda(context.Background(), "", "/tmp/x")
	if err != nil {
		t.Fatalf("Boveda: %v", err)
	}
	if d, ok := a.(Disco); !ok || d.Dir != "/tmp/x" {
		t.Fatalf("se esperaba Disco{/tmp/x}, se obtuvo %#v", a)
	}
}

func TestBovedaConBucketEsS3(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	a, err := Boveda(context.Background(), "intela-reportes", "/tmp/x")
	if err != nil {
		t.Fatalf("Boveda: %v", err)
	}
	if s, ok := a.(S3); !ok || s.Bucket != "intela-reportes" || s.Cliente == nil {
		t.Fatalf("se esperaba S3{intela-reportes}, se obtuvo %#v", a)
	}
}
