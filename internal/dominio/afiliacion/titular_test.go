package afiliacion

import (
	"errors"
	"testing"
)

// Las dos formas del padron que hay que poder leer. La segunda es la que
// justifica que exista el tipo: una productora SIN IPI esta en el padron y no
// puede cobrar reparto (`R-01`, `RD 4.5`).
func socia(t *testing.T) Titular {
	t.Helper()

	tit, err := NuevoTitular("tit-ana", "Ana Escritora", "IPI-00000001", true, ClaseSocio)
	if err != nil {
		t.Fatalf("construir la socia: %v", err)
	}
	return tit
}

func productora(t *testing.T) Titular {
	t.Helper()

	tit, err := NuevoTitular("tit-productora", "Productora del Caribe S.A.S.", "", false, ClaseAdministrado)
	if err != nil {
		t.Fatalf("construir la productora: %v", err)
	}
	return tit
}

func TestNuevoTitularAceptaLoQueElEsquemaAcepta(t *testing.T) {
	casos := []struct {
		nombre    string
		id        string
		tit       string
		ipi       string
		pers      bool
		clase     Clase
		idQuiero  string
		titQuiero string
		ipiQuiero string
	}{
		{
			nombre: "socia persona natural", id: "tit-ana", tit: "Ana Escritora",
			ipi: "IPI-00000001", pers: true, clase: ClaseSocio,
			idQuiero: "tit-ana", titQuiero: "Ana Escritora", ipiQuiero: "IPI-00000001",
		},
		{
			nombre: "administrado persona natural", id: "tit-beto", tit: "Beto Libretista",
			ipi: "IPI-00000002", pers: true, clase: ClaseAdministrado,
			idQuiero: "tit-beto", titQuiero: "Beto Libretista", ipiQuiero: "IPI-00000002",
		},
		// El CHECK `titular_natural_tiene_ipi` solo exige IPI a las personas
		// naturales, asi que la lectura del padron no puede ser mas estricta
		// que el: si lo fuera, una fila legitima de `titulares` no se podria
		// leer y el editor de reparto se quedaria sin padron.
		{
			nombre: "persona juridica sin IPI", id: "tit-prod", tit: "Productora del Caribe S.A.S.",
			pers: false, clase: ClaseAdministrado,
			idQuiero: "tit-prod", titQuiero: "Productora del Caribe S.A.S.",
		},
		{
			nombre: "nombre y codigos con espacios sobrantes", id: " tit-ana ", tit: "  Ana Escritora ",
			ipi: " IPI-00000001 ", pers: true, clase: ClaseSocio,
			idQuiero: "tit-ana", titQuiero: "Ana Escritora", ipiQuiero: "IPI-00000001",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			tit, err := NuevoTitular(c.id, c.tit, c.ipi, c.pers, c.clase)
			if err != nil {
				t.Fatalf("NuevoTitular: %v", err)
			}
			// Se guarda recortado, igual que valida el btrim del esquema: el
			// dominio y la base no pueden opinar distinto sobre lo que esta
			// vacio.
			if tit.ID() != c.idQuiero || tit.Nombre() != c.titQuiero || tit.IPI() != c.ipiQuiero {
				t.Fatalf("titular = {%q, %q, %q}", tit.ID(), tit.Nombre(), tit.IPI())
			}
			if tit.PersonaNatural() != c.pers || tit.Clase() != c.clase {
				t.Fatalf("titular = %+v", tit)
			}
		})
	}
}

// Una fila que no forma un titular se rechaza antes de tocar la base, y el
// fallo apunta al campo. Los casos son EXACTAMENTE los del esquema: ni uno
// mas, porque este constructor tambien reconstruye lo que se lee.
func TestNuevoTitularRechazaLoQueNoFormaUnTitular(t *testing.T) {
	casos := []struct {
		nombre string
		id     string
		tit    string
		ipi    string
		pers   bool
		clase  Clase
	}{
		{"sin identificador", "", "Ana Escritora", "IPI-00000001", true, ClaseSocio},
		{"identificador en blanco", "   ", "Ana Escritora", "IPI-00000001", true, ClaseSocio},
		{"sin nombre", "tit-ana", "", "IPI-00000001", true, ClaseSocio},
		{"nombre en blanco", "tit-ana", "  \t ", "IPI-00000001", true, ClaseSocio},
		{"clase que no esta en el reglamento", "tit-ana", "Ana Escritora", "IPI-00000001", true, "aspirante"},
		{"clase vacia", "tit-ana", "Ana Escritora", "IPI-00000001", true, ""},
		// R-01 y el CHECK `titular_natural_tiene_ipi`: una persona natural sin
		// IPI es un nombre suelto que no resuelve a nadie fuera de la sociedad.
		{"persona natural sin IPI", "tit-ana", "Ana Escritora", "", true, ClaseSocio},
		{"persona natural con el IPI en blanco", "tit-ana", "Ana Escritora", " ", true, ClaseAdministrado},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := NuevoTitular(c.id, c.tit, c.ipi, c.pers, c.clase)
			if !errors.Is(err, ErrTitularInvalido) {
				t.Fatalf("se esperaba ErrTitularInvalido, se obtuvo %v", err)
			}
		})
	}
}

// El cero del tipo no pasa por ningun sitio que lo acepte: su id esta vacio.
func TestElTitularCeroNoSePuedeLeer(t *testing.T) {
	var cero Titular
	if cero.ID() != "" || cero.PersonaNatural() || cero.Clase() != "" {
		t.Fatalf("el cero del tipo tiene contenido: %+v", cero)
	}
	if _, err := NuevoTitular(cero.ID(), cero.Nombre(), cero.IPI(), cero.PersonaNatural(), cero.Clase()); !errors.Is(err, ErrTitularInvalido) {
		t.Fatalf("el cero del tipo se puede reconstruir: %v", err)
	}
}

// R-01 / RD 4.5, literal: solo un escritor persona natural recibe orden de
// pago. Es la regla que el editor de reparto necesita para no ofrecer a una
// productora como parte, y la misma que impone el trigger
// `exigir_persona_natural` antes de que salga el dinero.
func TestSoloLaPersonaNaturalPuedeRecibirReparto(t *testing.T) {
	ana := socia(t)
	empresa := productora(t)

	if !ana.PuedeRecibirReparto() {
		t.Fatal("una escritora persona natural no puede recibir reparto")
	}
	if empresa.PuedeRecibirReparto() {
		t.Fatal("una productora puede recibir reparto: R-01 no se esta aplicando")
	}
	// Estar en el padron es lo que hace falta para que la respuesta anterior
	// signifique algo: la productora EXISTE y aun asi no cobra.
	if empresa.Nombre() == "" || empresa.Clase() == "" {
		t.Fatalf("la productora no es una entrada del padron: %+v", empresa)
	}
}

// La clase no decide quien cobra: un administrado persona natural cobra igual
// que un socio. Confundir las dos cosas dejaria fuera a medio padron.
func TestLaClaseNoDecideQuienRecibeReparto(t *testing.T) {
	administrado, err := NuevoTitular("tit-beto", "Beto Libretista", "IPI-00000002", true, ClaseAdministrado)
	if err != nil {
		t.Fatalf("NuevoTitular: %v", err)
	}
	if !administrado.PuedeRecibirReparto() {
		t.Fatal("un administrado persona natural tiene que poder recibir reparto")
	}
}
