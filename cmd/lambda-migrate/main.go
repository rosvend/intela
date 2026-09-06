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
	"log/slog"
	"time"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/migraciones"
)

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
