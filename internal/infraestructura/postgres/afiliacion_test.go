package postgres

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

// El hash sale APARTE del Usuario, no dentro. Es el contrato del puerto
// (internal/aplicacion/puertos.go): asi es el Hasher quien lo verifica y el
// resto del nucleo nunca ve una credencial.
func TestUsuarioPorEmailDevuelveElHashAparte(t *testing.T) {
	s, _ := sembrar(t)

	u, hash, err := s.UsuarioPorEmail(t.Context(), emailTitular)
	if err != nil {
		t.Fatalf("UsuarioPorEmail: %v", err)
	}

	if u.ID != usuarioTitular {
		t.Fatalf("ID = %q, se esperaba %q", u.ID, usuarioTitular)
	}
	if u.Email != emailTitular {
		t.Fatalf("Email = %q, se esperaba %q", u.Email, emailTitular)
	}
	if u.Nombre != "Ana Escritora" {
		t.Fatalf("Nombre = %q, se esperaba \"Ana Escritora\"", u.Nombre)
	}
	if u.Rol != aplicacion.RolTitular {
		t.Fatalf("Rol = %q, se esperaba %q", u.Rol, aplicacion.RolTitular)
	}
	if u.TitularID != titularAna {
		t.Fatalf("TitularID = %q, se esperaba %q", u.TitularID, titularAna)
	}
	if hash != hashBcrypt {
		t.Fatalf("hash = %q, se esperaba el hash sembrado", hash)
	}
}

// titular_id es NULL para los roles que no son titular. Sin el COALESCE, el
// escaneo a un string revienta; con el, la cadena vacia dice lo mismo sin
// traer un tipo nullable al adaptador.
func TestUsuarioSinTitularDevuelveTitularIDVacio(t *testing.T) {
	s, _ := sembrar(t)

	u, _, err := s.UsuarioPorEmail(t.Context(), emailAdmin)
	if err != nil {
		t.Fatalf("UsuarioPorEmail: %v", err)
	}
	if u.Rol != aplicacion.RolAdministrador {
		t.Fatalf("Rol = %q, se esperaba %q", u.Rol, aplicacion.RolAdministrador)
	}
	if u.TitularID != "" {
		t.Fatalf("TitularID = %q, se esperaba vacio para un administrador", u.TitularID)
	}
}

func TestUsuarioPorID(t *testing.T) {
	s, _ := sembrar(t)

	u, err := s.UsuarioPorID(t.Context(), usuarioAdmin)
	if err != nil {
		t.Fatalf("UsuarioPorID: %v", err)
	}
	if u.Email != emailAdmin {
		t.Fatalf("Email = %q, se esperaba %q", u.Email, emailAdmin)
	}
	if u.Rol != aplicacion.RolAdministrador {
		t.Fatalf("Rol = %q, se esperaba %q", u.Rol, aplicacion.RolAdministrador)
	}
}

// El camino de "no hay fila". Tiene que ser ErrNoEncontrado y no un error
// cualquiera: #16 decide con esto si devuelve 401 o 500, y confundirlos
// convierte un fallo de base de datos en "credenciales invalidas".
func TestUsuarioNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	t.Run("por email", func(t *testing.T) {
		_, _, err := s.UsuarioPorEmail(ctx, "nadie@redes.co")
		if !errors.Is(err, aplicacion.ErrNoEncontrado) {
			t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
		}
	})

	t.Run("por id", func(t *testing.T) {
		_, err := s.UsuarioPorID(ctx, "usr-inexistente")
		if !errors.Is(err, aplicacion.ErrNoEncontrado) {
			t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
		}
	})
}
