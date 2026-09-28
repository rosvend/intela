package oni

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestProyectarCopiaSoloLoIdentificatorio(t *testing.T) {
	t.Parallel()

	d := DatosIdentificatorios{
		ID:        "uso-1",
		Titulo:    "Serie Desconocida",
		Fuente:    "caracol",
		IDsFuente: "ID-99",
		Modalidad: "tv",
		Periodo:   "2026-01",
	}

	p, err := Proyectar(d)
	if err != nil {
		t.Fatalf("Proyectar: %v", err)
	}
	if p.ID != d.ID || p.Titulo != d.Titulo || p.Fuente != d.Fuente {
		t.Fatalf("no copio lo identificatorio: %+v", p)
	}
	if p.IDsFuente != d.IDsFuente || p.Modalidad != d.Modalidad || p.Periodo != d.Periodo {
		t.Fatalf("no copio lo identificatorio: %+v", p)
	}
}

func TestProyectarRechazaSinTitulo(t *testing.T) {
	t.Parallel()

	_, err := Proyectar(DatosIdentificatorios{ID: "uso-1", Fuente: "caracol"})
	if !errors.Is(err, ErrTituloAusente) {
		t.Fatalf("se esperaba ErrTituloAusente, se obtuvo %v", err)
	}
}

// R-18 / RD 13.8.2: titulos e informacion identificatoria, sin montos.
//
// El tipo es el invariante. Si alguien anade Importe, Bruto o Taquilla a
// ProyeccionPublica, esta prueba falla antes de que el campo llegue a un
// handler o a la vista SQL.
func TestProyeccionPublicaNoTieneCamposDeDinero(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(ProyeccionPublica{})
	prohibidos := []string{
		"monto", "importe", "bruto", "neto", "valor", "taquilla",
		"puntos", "bolsa", "pago", "precio", "cantidad", "reserva",
	}

	for i := 0; i < typ.NumField(); i++ {
		campo := typ.Field(i)
		nombre := strings.ToLower(campo.Name)
		for _, p := range prohibidos {
			if strings.Contains(nombre, p) {
				t.Fatalf("ProyeccionPublica.%s parece dinero; R-18 lo prohibe", campo.Name)
			}
		}
		switch campo.Type.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			t.Fatalf("ProyeccionPublica.%s es numerico (%s); R-18 no admite montos",
				campo.Name, campo.Type.Kind())
		}
	}

	// Lo mismo del insumo: si DatosIdentificatorios crece un importe, Proyectar
	// tendria de donde copiarlo.
	insumo := reflect.TypeOf(DatosIdentificatorios{})
	for i := 0; i < insumo.NumField(); i++ {
		campo := insumo.Field(i)
		nombre := strings.ToLower(campo.Name)
		for _, p := range prohibidos {
			if strings.Contains(nombre, p) {
				t.Fatalf("DatosIdentificatorios.%s parece dinero; el insumo de Proyectar no puede tenerlo", campo.Name)
			}
		}
	}
}

func TestValidarMetadatos(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre      string
		periodo     string
		fisica      string
		electronica string
		quiero      error
	}{
		{
			nombre:      "completo",
			periodo:     "2026-01",
			fisica:      "Calle 1 #2-3, Bogota",
			electronica: "oni@redes.co",
		},
		{
			nombre:      "sin periodo",
			fisica:      "Calle 1",
			electronica: "oni@redes.co",
			quiero:      ErrPeriodoAusente,
		},
		{
			nombre:      "sin fisica",
			periodo:     "2026",
			electronica: "oni@redes.co",
			quiero:      ErrDireccionAusente,
		},
		{
			nombre:  "sin electronica",
			periodo: "2026",
			fisica:  "Calle 1",
			quiero:  ErrDireccionAusente,
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			err := ValidarMetadatos(c.periodo, c.fisica, c.electronica)
			if !errors.Is(err, c.quiero) {
				t.Fatalf("ValidarMetadatos = %v, se esperaba %v", err, c.quiero)
			}
		})
	}
}
