package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// Review de PR #146, S3: la bandeja de candidatos y el match de una ONI son un
// solo hecho. Estas pruebas ejercen el rollback de verdad, que el doble de la
// capa de aplicacion no puede: ahi solo se comprueba QUE decide la unidad.

// GuardarCandidatos abre su transaccion con enTransaccionDe: dentro de una unidad
// escribe en la de la unidad, y si la unidad falla, se va con ella y la bandeja
// anterior sigue intacta.
func TestUnaUnidadQueFallaRevierteLaBandejaYDejaLaAnterior(t *testing.T) {
	s, _ := sembrarIdentificacion(t)
	ctx := t.Context()

	anterior := []identificacion.Candidato{
		{ObraID: obraIda, Puntaje: decimal.RequireFromString("0.52"), TituloConsultado: "de antes"},
	}
	if err := s.GuardarCandidatos(ctx, "u-3", anterior); err != nil {
		t.Fatalf("bandeja anterior: %v", err)
	}

	fallo := errors.New("el match no se pudo escribir")
	err := s.EnUnidad(ctx, func(ctx context.Context) error {
		nueva := []identificacion.Candidato{
			{ObraID: obraImdb, Puntaje: decimal.RequireFromString("0.49"), TituloConsultado: "de ahora"},
		}
		if err := s.GuardarCandidatos(ctx, "u-3", nueva); err != nil {
			return err
		}
		// Dentro de la unidad la bandeja nueva ya se ve.
		dentro, err := s.CandidatosDeUso(ctx, "u-3")
		if err != nil {
			return err
		}
		if len(dentro) != 1 || dentro[0].ObraID != obraImdb {
			t.Errorf("dentro de la unidad tenia que verse la bandeja nueva: %+v", dentro)
		}
		return fallo
	})
	if !errors.Is(err, fallo) {
		t.Fatalf("EnUnidad = %v, se esperaba el error de la unidad", err)
	}

	despues, err := s.CandidatosDeUso(ctx, "u-3")
	if err != nil {
		t.Fatalf("CandidatosDeUso: %v", err)
	}
	if len(despues) != 1 || despues[0].ObraID != obraIda || despues[0].TituloConsultado != "de antes" {
		t.Fatalf("la unidad fallo y la bandeja anterior tenia que quedar intacta: %+v", despues)
	}
}

// usosQueCambianAlLeer lee los usos como siempre y, justo despues, ejecuta alLeer:
// es la ventana entre la lectura de la cascada y su escritura, donde otro proceso
// (una resolucion manual en la cola, #39) puede cambiar la fila.
type usosQueCambianAlLeer struct {
	ingestaDePrueba
	alLeer func()
}

func (u usosQueCambianAlLeer) UsosDePeriodo(ctx context.Context, periodo string) ([]aplicacion.UsoPersistido, error) {
	usos, err := u.ingestaDePrueba.UsosDePeriodo(ctx, periodo)
	if err == nil {
		u.alLeer()
	}
	return usos, err
}

// El caso que describe el review: la cascada lee u-3 pendiente, otro proceso la
// resuelve a mano, y la cascada -que la ve ONI sin candidatos- intenta vaciar su
// bandeja y marcarla ONI. El match no se escribe (la fila ya es 'manual'), y sin
// la unidad la bandeja ya habria quedado vaciada. Con ella, todo se revierte.
func TestResolverUsosIntegracionONIQuePierdeLaCarreraConservaLaBandejaAnterior(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()
	r := cascadaDePrueba(t, s, pool)

	anterior := []identificacion.Candidato{
		{ObraID: obraIda, Puntaje: decimal.RequireFromString("0.52"), TituloConsultado: "de antes"},
	}
	if err := s.GuardarCandidatos(ctx, "u-3", anterior); err != nil {
		t.Fatalf("bandeja anterior: %v", err)
	}

	r.Usos = usosQueCambianAlLeer{
		ingestaDePrueba: r.Usos.(ingestaDePrueba),
		alLeer: func() {
			if _, err := pool.Exec(ctx,
				`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
				 VALUES ('revisor-1', 'revisor@redes.test', 'Revisor', 'administrador', $1)`,
				hashBcrypt); err != nil {
				t.Errorf("sembrar el revisor: %v", err)
				return
			}
			if _, err := pool.Exec(ctx,
				`UPDATE usos
				    SET escalon = 'manual', obra_id = $1, oni = FALSE,
				        resuelto_por = 'revisor-1', resuelto_en = now()
				  WHERE id = 'u-3'`, obraVacia); err != nil {
				t.Errorf("resolver u-3 a mano: %v", err)
			}
		},
	}

	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("una fila cambiada por otro no es un error de la corrida: %v", err)
	}

	// La decision humana sigue en pie: la cascada no la pisa (D6).
	f := leerUso(t, pool, "u-3")
	if f.escalon != identificacion.EscalonManual || f.obraID != obraVacia || f.oni {
		t.Fatalf("la cascada piso una resolucion manual: %+v", f)
	}

	// Y la bandeja que la cascada iba a reemplazar no se toco.
	bandeja, err := s.CandidatosDeUso(ctx, "u-3")
	if err != nil {
		t.Fatalf("CandidatosDeUso: %v", err)
	}
	if len(bandeja) != 1 || bandeja[0].ObraID != obraIda || bandeja[0].TituloConsultado != "de antes" {
		t.Fatalf("la carrera perdida tenia que dejar la bandeja como estaba: %+v", bandeja)
	}
}

// El arreglo derivado, de punta a punta: una fila que una corrida dejo en la
// bandeja y que en la siguiente cae por debajo del piso se queda con la bandeja
// VACIA, no con los candidatos viejos.
func TestResolverUsosIntegracionUnaONISinCandidatosVaciaLaBandejaVieja(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()
	r := cascadaDePrueba(t, s, pool)

	vieja := []identificacion.Candidato{
		{ObraID: obraIda, Puntaje: decimal.RequireFromString("0.52"), TituloConsultado: "de antes"},
	}
	if err := s.GuardarCandidatos(ctx, "u-3", vieja); err != nil {
		t.Fatalf("bandeja vieja: %v", err)
	}

	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}

	f := leerUso(t, pool, "u-3")
	if f.escalon != identificacion.EscalonONI || !f.oni {
		t.Fatalf("u-3 no trae ids ni se parece a nada: tenia que quedar ONI: %+v", f)
	}
	bandeja, err := s.CandidatosDeUso(ctx, "u-3")
	if err != nil {
		t.Fatalf("CandidatosDeUso: %v", err)
	}
	if len(bandeja) != 0 {
		t.Fatalf("sin candidatos sobre el piso la bandeja tenia que quedar vacia: %+v", bandeja)
	}
}
