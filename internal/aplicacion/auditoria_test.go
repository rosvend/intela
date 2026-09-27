package aplicacion

import (
	"context"
	"errors"
	"testing"
	"time"
)

// bitacoraFalsa registra lo que le llego y devuelve lo que le pongan. Sirve
// tanto a los tests de Auditoria (paginacion / referencia de obra antes de
// tocar la base) como a los de Catalogo (ADR 0006: si Asentar falla, el caso
// de uso falla).
type bitacoraFalsa struct {
	asientos        []Asiento
	err             error
	pagRecibida     Paginacion
	refTipoRecibido string
	refIDRecibido   string
}

func (b *bitacoraFalsa) Asentar(_ context.Context, a Asiento) error {
	if b.err != nil {
		return b.err
	}
	b.asientos = append(b.asientos, a)
	return nil
}

func (b *bitacoraFalsa) De(_ context.Context, refTipo, refID string) ([]Asiento, error) {
	b.refTipoRecibido = refTipo
	b.refIDRecibido = refID
	if b.err != nil {
		return nil, b.err
	}
	var out []Asiento
	for _, a := range b.asientos {
		if a.RefTipo == refTipo && a.RefID == refID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (b *bitacoraFalsa) AsientoPorID(_ context.Context, id string) (Asiento, error) {
	for _, a := range b.asientos {
		if a.ID == id {
			return a, nil
		}
	}
	return Asiento{}, ErrNoEncontrado
}

func (b *bitacoraFalsa) ListarAsientos(_ context.Context, pag Paginacion) ([]Asiento, error) {
	b.pagRecibida = pag
	return b.asientos, b.err
}

func asientoDePrueba(hecho, refTipo, refID string) Asiento {
	return Asiento{
		ID:      hecho + "/" + refID,
		Hecho:   hecho,
		RefTipo: refTipo,
		RefID:   refID,
		ActorID: "admin-1",
		Payload: []byte(`{}`),
		Cuando:  time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC),
	}
}

func TestAsientosAplicaPaginacionPorDefecto(t *testing.T) {
	bitacora := &bitacoraFalsa{}

	if _, err := (Auditoria{Bitacora: bitacora}).Asientos(t.Context(), Paginacion{}); err != nil {
		t.Fatalf("Asientos: %v", err)
	}
	if bitacora.pagRecibida.Limite != LimiteObrasPorDefecto {
		t.Fatalf("Limite = %d, se esperaba %d",
			bitacora.pagRecibida.Limite, LimiteObrasPorDefecto)
	}
	if bitacora.pagRecibida.Desplazamiento != 0 {
		t.Fatalf("Desplazamiento = %d, se esperaba 0", bitacora.pagRecibida.Desplazamiento)
	}
}

func TestAsientosDevuelveLoQueTraeLaBitacora(t *testing.T) {
	bitacora := &bitacoraFalsa{asientos: []Asiento{
		asientoDePrueba("declaracion.guardada", "obra", "ob-1"),
		asientoDePrueba("recaudo.registrado", "bolsa", "bo-1"),
	}}

	got, err := (Auditoria{Bitacora: bitacora}).Asientos(t.Context(), Paginacion{Limite: 10})
	if err != nil {
		t.Fatalf("Asientos: %v", err)
	}
	if len(got) != 2 || got[0].Hecho != "declaracion.guardada" || got[1].Hecho != "recaudo.registrado" {
		t.Fatalf("asientos = %+v, se esperaban los dos de la bitacora en orden", got)
	}
}

func TestAsientosPropagaElErrorDeLaBitacora(t *testing.T) {
	fallo := errors.New("base caida")
	bitacora := &bitacoraFalsa{err: fallo}

	if _, err := (Auditoria{Bitacora: bitacora}).Asientos(t.Context(), Paginacion{}); !errors.Is(err, fallo) {
		t.Fatalf("err = %v, se esperaba %v", err, fallo)
	}
}

func TestHistorialDeObraLeeLaReferenciaObra(t *testing.T) {
	bitacora := &bitacoraFalsa{asientos: []Asiento{
		asientoDePrueba("declaracion.guardada", "obra", "ob-1"),
	}}

	got, err := (Auditoria{Bitacora: bitacora}).HistorialDeObra(t.Context(), " ob-1 ")
	if err != nil {
		t.Fatalf("HistorialDeObra: %v", err)
	}
	if bitacora.refTipoRecibido != "obra" || bitacora.refIDRecibido != "ob-1" {
		t.Fatalf("referencia = %q %q, se esperaba \"obra\" \"ob-1\"",
			bitacora.refTipoRecibido, bitacora.refIDRecibido)
	}
	if len(got) != 1 {
		t.Fatalf("asientos = %+v, se esperaba el unico de la obra", got)
	}
}

func TestHistorialDeObraSinAsientosEsListaVacia(t *testing.T) {
	bitacora := &bitacoraFalsa{}

	got, err := (Auditoria{Bitacora: bitacora}).HistorialDeObra(t.Context(), "ob-sin-historia")
	if err != nil {
		t.Fatalf("HistorialDeObra: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("asientos = %+v, se esperaba lista vacia y no 404", got)
	}
}
