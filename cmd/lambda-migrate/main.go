// Command lambda-migrate aplica las migraciones desde dentro de la VPC.
//
// Hace falta un binario propio porque la base no tiene endpoint publico: el
// runner de GitHub Actions no la alcanza, asi que goose tiene que correr en
// algo que viva en las subredes privadas. Y un despliegue de Lambda es un unico
// ejecutable, de modo que no se puede reutilizar cmd/migrate pasandole
// argumentos.
//
// La mecanica es la misma: internal/infraestructura/migraciones. Este fichero
// traduce un evento de invocacion en una orden de goose.
//
// Y una que NO es de goose: `primer-administrador`. Esta aqui porque el
// problema que resuelve es el mismo -- hay que ejecutar algo DENTRO de la VPC
// contra una base sin endpoint publico -- y esta es la unica funcion que ya
// vive ahi con DATABASE_URL. Levantar una Lambda propia para una operacion que
// se corre una vez en la vida de una instalacion es infraestructura que hay
// que mantener para siempre. La deuda que si se asume: este binario ya no es
// solo goose, y si aparece una segunda operacion de este tipo conviene sacarlas
// las dos a su propia funcion.
//
// Lo invoca Terraform (aws_lambda_invocation en modules/migrations), no el
// workflow, para que el orden migrar-antes-de-servir quede en el grafo de
// dependencias: si esto falla, el apply falla y la funcion de la API nunca
// llega a actualizarse. Es lo que pide docs/cd.md y lo que el ADR 0008
// justifica.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/cripto"
	"github.com/rosvend/intela/internal/infraestructura/migraciones"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
)

// ordenesPermitidas es lo que esta funcion acepta hacer. Todo lo demas se
// rechaza sin abrir la conexion.
//
// goose entiende bastante mas -`down`, `down-to`, `reset`- y `reset` tira el
// esquema entero. Esta funcion es alcanzable por cualquiera que tenga
// lambda:InvokeFunction en la cuenta, y su nombre se publica en los outputs de
// Terraform para poder usarla en un incidente: sin esta lista, ese permiso
// equivale a borrar la base, sin pasar por Terraform y sin que la guarda de
// destruccion del pipeline se entere de nada.
//
// La lista vive aqui y NO en internal/infraestructura/migraciones a proposito.
// El otro llamante de ese paquete es cmd/migrate, que corre una persona con
// credenciales delante de una terminal, y ahi revertir es legitimo. Lo que hay
// que acotar no es la mecanica de goose, es esta superficie.
var ordenesPermitidas = []string{"up", "up-by-one", "status", "version"}

// ordenPrimerAdministrador provisiona la cuenta inicial. Deliberadamente FUERA
// de ordenesPermitidas: no es una orden de goose y no debe llegar a Aplicar.
const ordenPrimerAdministrador = "primer-administrador"

// peticion es lo que manda Terraform: {"orden":"up"}.
//
// Los cuatro campos de abajo solo los usa `primer-administrador`, y llegan en
// el evento en vez de en el entorno a proposito: son de un solo uso, y una
// variable de entorno de la Lambda se queda ahi -- visible en la consola y en
// cada `terraform plan` -- mucho despues de que la cuenta exista.
//
// Hash y no la clave en claro. La calcula quien invoca, en su terminal, asi que
// la credencial no viaja en el evento, no queda en el registro de la plataforma
// y este proceso no la ve nunca.
type peticion struct {
	Orden string `json:"orden"`

	ID     string `json:"id,omitempty"`
	Email  string `json:"email,omitempty"`
	Nombre string `json:"nombre,omitempty"`
	Hash   string `json:"hash,omitempty"`
}

type respuesta struct {
	Orden  string `json:"orden"`
	Estado string `json:"estado"`
}

func main() {
	lambda.Start(atender(config.Logger("lambda-migrate")))
}

func atender(log *slog.Logger) func(context.Context, peticion) (respuesta, error) {
	return func(ctx context.Context, p peticion) (respuesta, error) {
		orden := p.Orden
		if orden == "" {
			orden = migraciones.OrdenPorDefecto
		}

		if orden == ordenPrimerAdministrador {
			return provisionar(ctx, p, log)
		}

		// Antes de conectar: una orden rechazada no debe llegar a tocar la base
		// ni a dejar una conexion abierta.
		if !slices.Contains(ordenesPermitidas, orden) {
			err := fmt.Errorf("orden %q no permitida; solo %v", orden, ordenesPermitidas)
			log.Error("orden rechazada", slog.Any("error", err))
			return respuesta{}, err
		}

		// Derivado del contexto de la invocacion: gana el plazo mas corto entre
		// MIGRATE_TIMEOUT y lo que le quede a la Lambda. Asi el error es un
		// error de goose y no una muerte por timeout de plataforma, que no dice
		// en que sentencia se quedo.
		ctx, cancelar := context.WithTimeout(ctx,
			config.Duracion("MIGRATE_TIMEOUT", 4*time.Minute))
		defer cancelar()

		if err := migraciones.Aplicar(ctx, config.Cadena("DATABASE_URL", ""), orden, log); err != nil {
			// Devolver el error hace fallar la invocacion, que hace fallar el
			// apply. Es exactamente lo que tiene que pasar.
			log.Error("migracion fallida", slog.Any("error", err))
			return respuesta{}, err
		}

		return respuesta{Orden: orden, Estado: "aplicadas"}, nil
	}
}

// provisionar crea la cuenta inicial de una instalacion vacia.
//
// El caso de uso valida ANTES de que esto conecte, igual que la lista de
// ordenes se comprueba antes de llamar a goose: una peticion mal formada no
// tiene por que abrir un pool contra la base.
//
// ErrYaHayUsuarios NO se devuelve como error de la invocacion. La operacion es
// de una sola vez; que ya se haya hecho no es un fallo, y hacerla fallar
// convertiria un reintento inocuo en una alarma. Se responde con estado
// "ya provisionada" y se registra.
func provisionar(ctx context.Context, p peticion, log *slog.Logger) (respuesta, error) {
	// Se valida sin tocar la base. Si algo falta, el error nombra el campo --
	// nunca su valor -- y la conexion no se abre.
	//
	// cripto.Bcrypt se construye aqui y no despues porque la comprobacion que
	// de verdad importa es la de la FORMA del hash, y esa la contesta el
	// adaptador. Construirlo no cuesta nada: no tiene estado ni E/S.
	provision := aplicacion.Provision{Claves: cripto.Bcrypt{}}
	if err := provision.Validar(p.ID, p.Email, p.Nombre, p.Hash); err != nil {
		log.Error("provision rechazada", slog.Any("error", err))
		return respuesta{}, err
	}

	ctx, cancelar := context.WithTimeout(ctx,
		config.Duracion("MIGRATE_TIMEOUT", 4*time.Minute))
	defer cancelar()

	store, err := postgres.Abrir(ctx, config.Cadena("DATABASE_URL", ""))
	if err != nil {
		log.Error("abrir la base", slog.Any("error", err))
		return respuesta{}, err
	}
	defer store.CerrarPool()

	provision.Usuarios = store
	u, err := provision.CrearPrimerAdministrador(ctx, p.ID, p.Email, p.Nombre, p.Hash)
	switch {
	case errors.Is(err, aplicacion.ErrYaHayUsuarios):
		log.Info("la instalacion ya estaba provisionada; no se crea nada")
		return respuesta{Orden: ordenPrimerAdministrador, Estado: "ya provisionada"}, nil
	case err != nil:
		log.Error("provision fallida", slog.Any("error", err))
		return respuesta{}, err
	}

	// Sin el email ni el hash en el registro: basta con QUE cuenta quedo.
	log.Info("primer administrador creado", slog.String("id", u.ID), slog.String("rol", string(u.Rol)))
	return respuesta{Orden: ordenPrimerAdministrador, Estado: "creado"}, nil
}
