// Command lambda-migrate aplica las migraciones desde dentro de la VPC.
//
// Hace falta un binario propio porque la base no tiene endpoint publico: el
// runner de GitHub Actions no la alcanza, asi que goose tiene que correr en
// algo que viva en las subredes privadas. Y un despliegue de Lambda es un unico
// ejecutable, de modo que no se puede reutilizar cmd/migrate pasandole
// argumentos.
//
// La mecanica es la misma: internal/infraestructura/migraciones. Este fichero
// solo traduce un evento de invocacion en una orden de goose.
//
// Lo invoca Terraform (aws_lambda_invocation en modules/migrations), no el
// workflow, para que el orden migrar-antes-de-servir quede en el grafo de
// dependencias: si esto falla, el apply falla y la funcion de la API nunca
// llega a actualizarse. Es lo que pide docs/cd.md y lo que el ADR 0008
// justifica.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/migraciones"
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

// peticion es lo que manda Terraform: {"orden":"up"}.
type peticion struct {
	Orden string `json:"orden"`
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
