package postgres

import (
	"os"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/embeddings"
	"github.com/rosvend/intela/internal/infraestructura/embeddings/falso"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
	"github.com/rosvend/intela/internal/infraestructura/reglamentos"
)

// TestIndexarYConsultarLosReglamentosReales es el camino de cmd/indexadorreglamento y de buscar_reglamento contra Postgres real.
func TestIndexarYConsultarLosReglamentosReales(t *testing.T) {
	s := Nuevo(testhelp.Pool(t))
	secciones, err := reglamentos.Leer(os.DirFS("../../../docs/reglamentos"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := aplicacion.IndexarReglamento{Motor: falso.Motor{}, Almacen: s}.Indexar(t.Context(), secciones)
	if err != nil || n != len(secciones) {
		t.Fatalf("Indexar: n=%d err=%v", n, err)
	}

	uc := aplicacion.ConsultarReglamento{Motor: falso.Motor{}, Almacen: s, Piso: embeddings.PisoFalso}
	actor := aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolDistribucion}

	r, err := uc.Consultar(t.Context(), actor, "tabla de ponderacion del tipo de obra: cinematografica, unitario, telenovela, sketches")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Encontrado || r.Secciones[0].Seccion.Cita != "RD 9.1.1" {
		t.Fatalf("se esperaba RD 9.1.1 primero; got %+v", r.Secciones)
	}

	r, err = uc.Consultar(t.Context(), actor, "receta de ajiaco santafereño con guascas")
	if err != nil {
		t.Fatal(err)
	}
	if r.Encontrado {
		t.Fatalf("nada del reglamento habla de ajiaco y se devolvio %+v", r.Secciones)
	}
}
