package aplicacion

import (
	"context"
	"fmt"
	"strings"
)

// Provision crea la primera cuenta de una instalacion vacia.
//
// # Por que existe
//
// Todas las rutas de la API menos /health y /ready piden sesion, y una sesion
// sale de un usuario. Una base recien migrada no tiene ninguno: ninguna
// migracion inserta usuarios -- y no debe, un credencial no es esquema -- y
// `cmd/seed` no viaja en la imagen del servicio a proposito, porque su
// SEED_RESET borra tablas. Sin esto, una instalacion nueva queda cerrada para
// siempre: no hay forma de crear la cuenta que crearia las demas.
//
// # Una sola vez, y lo decide la base
//
// La invariante es "exactamente un primer usuario, nunca dos". No se comprueba
// aqui con un recuento previo porque entre el SELECT y el INSERT cabe otra
// invocacion, y el resultado seria una segunda cuenta de administrador que
// nadie pidio. El puerto la impone en la misma sentencia que inserta, y
// devuelve [ErrYaHayUsuarios] si ya habia alguien.
//
// Con eso, la superficie se cierra sola: en cuanto existe un usuario, esta
// operacion no puede volver a hacer nada, aunque siga siendo invocable.
//
// # El rol no es un parametro
//
// Siempre `administrador`. Es la unica cuenta que existe en ese momento y
// tiene que poder crear las demas; un titular o un auditor dejarian la
// instalacion igual de cerrada. Y `titular_id` se queda vacio: el CHECK
// titular_tiene_titular_id solo lo exige para el rol titular, y un
// administrador con titular_id se leeria como si representara a un socio.
//
// # La clave no pasa por aqui
//
// Se recibe YA hasheada. Quien invoca esta superficie lo hace desde una
// terminal, y el hash se calcula alli: asi la clave en claro no viaja en el
// evento de invocacion, no queda en el registro de la plataforma y no la ve
// este proceso. Es la misma division que en [Autenticacion], donde el nucleo
// nunca ve una credencial, solo su hash.
type Provision struct {
	Usuarios RepositorioProvisionInicial
}

// CrearPrimerAdministrador compone el usuario y lo manda al puerto.
//
// Devuelve el Usuario creado para que quien invoca pueda registrar QUE cuenta
// quedo provisionada sin volver a consultarla.
//
// Recorta los blancos una sola vez y arriba, por la misma razon que
// [Ingesta.GuardarUsos]: un email con espacios entra en la base sin ruido
// -- el CHECK solo pide que contenga '@' -- y despues no casa con el que llega
// del formulario de login, que viene limpio. El sintoma no seria un error sino
// una cuenta que existe y con la que no se puede entrar.
func (p Provision) CrearPrimerAdministrador(ctx context.Context, id, email, nombre, hash string) (Usuario, error) {
	if err := ValidarPrimerAdministrador(id, email, nombre, hash); err != nil {
		return Usuario{}, err
	}

	u := Usuario{
		ID:     strings.TrimSpace(id),
		Email:  strings.TrimSpace(email),
		Nombre: strings.TrimSpace(nombre),
		Rol:    RolAdministrador,
	}
	if err := p.Usuarios.CrearPrimerAdministrador(ctx, u, strings.TrimSpace(hash)); err != nil {
		return Usuario{}, fmt.Errorf("crear el primer administrador %q: %w", u.Email, err)
	}
	return u, nil
}

// ValidarPrimerAdministrador comprueba los datos sin tocar nada.
//
// Exportada, y no un detalle privado de [Provision.CrearPrimerAdministrador],
// porque quien invoca esto vive dentro de la VPC y tiene que poder rechazar una
// peticion mal formada ANTES de abrir un pool contra la base. Es la misma
// propiedad que la lista de ordenes de cmd/lambda-migrate: lo que no va a
// ejecutarse no debe llegar a conectar.
//
// El metodo la llama tambien, asi que la regla vive en un solo sitio y no
// pueden discrepar.
//
// Recorta los blancos para decidir, por la misma razon que
// [Ingesta.GuardarUsos]: un email de solo espacios no es "" para una
// comparacion ingenua, entra en la base sin ruido -- el CHECK solo pide que
// contenga '@' -- y despues no casa con el que llega del formulario de login.
func ValidarPrimerAdministrador(id, email, nombre, hash string) error {
	id = strings.TrimSpace(id)
	email = strings.TrimSpace(email)
	nombre = strings.TrimSpace(nombre)
	hash = strings.TrimSpace(hash)

	// Cada regla refleja una restriccion de la tabla `usuarios`, y la
	// duplicacion es deliberada: un 23514 desde el INSERT aborta la
	// invocacion con un SQLSTATE que no dice que campo estaba mal, y esta
	// superficie la usa una persona en una terminal.
	switch {
	case id == "":
		return fmt.Errorf("%w: falta el id", ErrUsuarioInvalido)
	case email == "":
		return fmt.Errorf("%w: falta el email", ErrUsuarioInvalido)
	case !strings.Contains(email, "@"):
		return fmt.Errorf("%w: el email %q no tiene arroba", ErrUsuarioInvalido, email)
	case nombre == "":
		return fmt.Errorf("%w: falta el nombre", ErrUsuarioInvalido)
	case hash == "":
		return fmt.Errorf("%w: falta el hash de la clave", ErrUsuarioInvalido)
	case len(hash) < hashMinimo:
		// El CHECK del esquema es length(password_hash) >= 20. Un hash mas
		// corto no es un hash: es una clave en claro que alguien mando por
		// error, y conviene rechazarla antes de escribirla.
		return fmt.Errorf(
			"%w: el hash de la clave tiene %d caracteres y el minimo es %d; se espera un hash, no la clave en claro",
			ErrUsuarioInvalido, len(hash), hashMinimo)
	}
	return nil
}

// hashMinimo es el length(password_hash) >= 20 del esquema. Un bcrypt real
// mide 60; el minimo esta puesto para que no entre una clave en claro corta.
const hashMinimo = 20
