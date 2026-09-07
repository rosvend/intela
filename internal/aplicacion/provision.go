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

	// Claves comprueba que lo que llega sea un hash de verdad. El nucleo no
	// sabe cual es el algoritmo -- por eso lo pregunta al puerto.
	Claves Hasher
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
	if err := p.Validar(id, email, nombre, hash); err != nil {
		return Usuario{}, err
	}

	u := Usuario{
		ID:     strings.TrimSpace(id),
		Email:  strings.TrimSpace(email),
		Nombre: strings.TrimSpace(nombre),
		Rol:    RolAdministrador,
	}
	if err := p.Usuarios.CrearPrimerAdministrador(ctx, u, strings.TrimSpace(hash)); err != nil {
		// Sin el email: este error se registra Y se devuelve desde la Lambda,
		// asi que la plataforma lo guarda una segunda vez por su cuenta.
		return Usuario{}, fmt.Errorf("crear el primer administrador: %w", err)
	}
	return u, nil
}

// Validar comprueba los datos sin tocar la base.
//
// Metodo y no funcion suelta porque necesita el Hasher: la regla "el hash tiene
// que ser verificable" solo la puede contestar el adaptador que lo produce.
//
// Publico, y no un detalle privado de [Provision.CrearPrimerAdministrador],
// porque quien invoca esto vive dentro de la VPC y tiene que poder rechazar una
// peticion mal formada ANTES de abrir un pool contra la base. Es la misma
// propiedad que la lista de ordenes de cmd/lambda-migrate: lo que no va a
// ejecutarse no debe llegar a conectar. CrearPrimerAdministrador lo llama
// tambien, asi que la regla vive en un solo sitio y no pueden discrepar.
//
// Ningun mensaje lleva el VALOR del email, solo el nombre del campo. El camino
// feliz ya se cuida de no registrarlo, y estos errores se registran y ademas se
// devuelven desde la Lambda, asi que la plataforma los guarda otra vez por su
// cuenta.
//
// Recorta los blancos para decidir, por la misma razon que
// [Ingesta.GuardarUsos]: un email de solo espacios no es "" para una
// comparacion ingenua, entra en la base sin ruido -- el CHECK solo pide que
// contenga '@' -- y despues no casa con el que llega del formulario de login.
func (p Provision) Validar(id, email, nombre, hash string) error {
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
		return fmt.Errorf("%w: el email no tiene arroba", ErrUsuarioInvalido)
	case nombre == "":
		return fmt.Errorf("%w: falta el nombre", ErrUsuarioInvalido)
	case hash == "":
		return fmt.Errorf("%w: falta el hash de la clave", ErrUsuarioInvalido)
	case p.Claves == nil:
		// Sin Hasher no se puede comprobar la forma, y dejar pasar el hash
		// seria peor que fallar: es el camino que dejaba la instalacion
		// cerrada para siempre.
		return fmt.Errorf("%w: falta el Hasher con el que comprobar el hash", ErrUsuarioInvalido)
	case !p.Claves.EsHash(hash):
		// LA comprobacion que importa. La longitud sola no basta: una clave en
		// claro de 20 caracteres o mas la pasaba, se guardaba tal cual, y el
		// login fallaba despues con la clave correcta -- sin arreglo posible,
		// porque esta operacion no corre dos veces y ninguna ruta HTTP crea
		// usuarios ni resetea claves.
		return fmt.Errorf(
			"%w: el valor de %d caracteres no tiene forma de hash; se espera un hash, no la clave en claro (vease el runbook)",
			ErrUsuarioInvalido, len(hash))
	}
	return nil
}
