package recaudo

import (
	"errors"
	"slices"
	"testing"
)

func datosValidos() Datos {
	return Datos{Nombre: "Caracol Television S.A.", NIT: "860025674-2", Categoria: TVAbierta}
}

func TestNuevoUsuarioConstruyeYRecorta(t *testing.T) {
	u, err := NuevoUsuario("  caracol  ", Datos{
		Nombre:    "  Caracol Television S.A.  ",
		NIT:       "  860025674-2  ",
		Categoria: TVAbierta,
	})
	if err != nil {
		t.Fatalf("NuevoUsuario: %v", err)
	}
	if u.ID() != "caracol" {
		t.Fatalf("ID() = %q, se esperaba %q", u.ID(), "caracol")
	}
	d := u.Datos()
	if d.Nombre != "Caracol Television S.A." || d.NIT != "860025674-2" {
		t.Fatalf("Datos() = %+v, se esperaba sin espacios alrededor", d)
	}
	if d.Categoria != TVAbierta {
		t.Fatalf("categoria = %q, se esperaba %q", d.Categoria, TVAbierta)
	}
}

func TestNuevoUsuarioAceptaTodasLasCategorias(t *testing.T) {
	// Si el reglamento anade una categoria y alguien la mete en el enum sin
	// tocar el CHECK de la migracion, este test sigue verde y la insercion
	// revienta en produccion. El que cubre esa pareja es el de integracion en
	// postgres/recaudo_test.go; aqui solo se comprueba que el constructor no
	// deja fuera ninguna de las que declara.
	for _, c := range CategoriasUsuario() {
		t.Run(string(c), func(t *testing.T) {
			d := datosValidos()
			d.Categoria = c
			if _, err := NuevoUsuario("u-1", d); err != nil {
				t.Fatalf("NuevoUsuario con categoria %q: %v", c, err)
			}
		})
	}
}

func TestNuevoUsuarioSinNITEsValido(t *testing.T) {
	// El NIT no lo exige ningun numeral: hay pagadores del circuito
	// internacional -sociedades hermanas- que no tienen uno colombiano.
	d := datosValidos()
	d.NIT = ""
	if _, err := NuevoUsuario("dago-films", d); err != nil {
		t.Fatalf("NuevoUsuario sin NIT: %v", err)
	}
}

func TestNuevoUsuarioRechaza(t *testing.T) {
	sinNombre := datosValidos()
	sinNombre.Nombre = "   "

	categoriaInventada := datosValidos()
	categoriaInventada.Categoria = CategoriaUsuario("radio")

	sinCategoria := datosValidos()
	sinCategoria.Categoria = ""

	casos := []struct {
		nombre string
		id     string
		datos  Datos
	}{
		{"sin identificador", "", datosValidos()},
		{"identificador en blanco", "   ", datosValidos()},
		{"sin nombre", "caracol", sinNombre},
		// El conjunto esta cerrado porque decide que formula de reparto aplica
		// aguas abajo (formulas.md 9.1, 9.2, 9.7). Un valor libre deja una
		// bolsa que ningun modelo de calculo sabe ponderar.
		{"categoria inventada", "caracol", categoriaInventada},
		{"categoria vacia", "caracol", sinCategoria},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := NuevoUsuario(c.id, c.datos)
			if !errors.Is(err, ErrUsuarioInvalido) {
				t.Fatalf("err = %v, se esperaba ErrUsuarioInvalido", err)
			}
		})
	}
}

func TestSinClasificarEsUnaCategoriaDelEnum(t *testing.T) {
	// La migracion 00009 rellena con `sin_clasificar` los pagadores que las
	// bolsas ya citaban antes de que existiera la tabla. Si el dominio no la
	// admitiera, esas filas no se podrian volver a leer (ADR 0004: el hueco se
	// declara, no se inventa un valor comodo).
	if !slices.Contains(CategoriasUsuario(), SinClasificar) {
		t.Fatal("SinClasificar tiene que estar entre las categorias validas")
	}
}
