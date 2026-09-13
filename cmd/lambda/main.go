// Command lambda sirve la API HTTP sobre AWS Lambda.
//
// Es un adaptador primario MAS, hermano de cmd/api, no un reemplazo: los dos
// consumen la misma costura, httpapi.(*API).Router(), que devuelve un
// http.Handler corriente. cmd/api sigue siendo lo que levanta `docker compose`
// en local, y lo que exige docs/context.md como entregable.
//
// Que anadir un runtime entero sea un `main` nuevo y cero cambios en
// internal/ es el retorno de la arquitectura hexagonal que el repositorio ya
// pago. Ver ADR 0014.
//
// Lo que NO hay aqui, y esta en cmd/api a proposito: servidor HTTP, senales,
// apagado ordenado y ADDR. Lambda es duena del socket y del ciclo de vida, asi
// que configurarlos seria configuracion que parece significar algo y no hace
// nada.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/cripto"
	"github.com/rosvend/intela/internal/infraestructura/httpapi"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

var (
	mu        sync.Mutex
	manejador http.Handler
	registro  *slog.Logger
)

func main() {
	registro = config.Logger("lambda")
	lambda.Start(atender)
}

func atender(ctx context.Context, ev events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	h, err := obtenerManejador()
	if err != nil {
		// Se registra como error -sobre lo que si se puede alarmar- pero se
		// responde 503 con la misma forma de error que el resto de la API, en
		// vez de devolver el error a Lambda y que el cliente reciba un 502
		// generico.
		registro.Error("arranque fallido", slog.Any("error", err))
		return events.LambdaFunctionURLResponse{
			StatusCode: http.StatusServiceUnavailable,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"servicio no disponible"}`,
		}, nil
	}

	r, err := aHTTP(ctx, ev)
	if err != nil {
		registro.Warn("peticion ilegible", slog.Any("error", err))
		return events.LambdaFunctionURLResponse{
			StatusCode: http.StatusBadRequest,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"peticion invalida"}`,
		}, nil
	}

	g := nuevoGrabador()
	h.ServeHTTP(g, r)
	return g.aEvento(), nil
}

// obtenerManejador construye el arbol de dependencias una vez y lo reutiliza
// mientras el contenedor siga vivo.
//
// Es un mutex y no un sync.Once justamente por el caso de error: sync.Once
// cachearia un fallo de arranque para siempre, y un contenedor que arranco
// mientras la base estaba caida serviria 503 hasta que Lambda lo reciclara.
// Aqui un fallo no se guarda, asi que la invocacion siguiente reintenta.
func obtenerManejador() (http.Handler, error) {
	mu.Lock()
	defer mu.Unlock()

	if manejador != nil {
		return manejador, nil
	}

	h, err := construir()
	if err != nil {
		return nil, err
	}

	manejador = h
	return h, nil
}

func construir() (http.Handler, error) {
	// Contexto propio, desligado del de la invocacion: el pool tiene que
	// sobrevivir a la peticion que lo creo. Solo acota el connect y el ping.
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()

	// Comprobado antes de conectar, igual que en cmd/api. Sin esto, una funcion
	// desplegada sin la variable no dice que le falta: se queda diez segundos
	// intentando conectar contra la cadena vacia y muere por el timeout de
	// arriba, que es el sintoma de "la base no responde" -exactamente la pista
	// equivocada durante un incidente-.
	dsn := config.Cadena("DATABASE_URL", "")
	if dsn == "" {
		return nil, errors.New("falta DATABASE_URL")
	}

	// El tamano del pool entra por el DSN (pool_max_conns), que pgxpool lee al
	// parsearlo. Es lo que impide que N contenedores tibios se coman las
	// conexiones de la instancia; el otro lado del limite es la concurrencia
	// reservada de la funcion.
	store, err := postgres.Abrir(ctx, dsn)
	if err != nil {
		return nil, err
	}
	// Sin defer store.Cerrar(): el pool tiene que seguir abierto entre
	// invocaciones. Lo cierra el reciclado del contenedor.

	autenticacion := aplicacion.Autenticacion{
		Usuarios: store,
		Claves:   cripto.Bcrypt{},
		Sesiones: store,
		Reloj:    reloj.Sistema{},
		Tokens:   cripto.TokensAleatorios{},
		TTL:      config.Duracion("SESION_TTL", 12*time.Hour),
	}

	// El mismo *Store satisface tambien CatalogoObras. El nucleo sigue viendo
	// puertos separados: que el adaptador sea uno solo es asunto suyo.
	catalogo := aplicacion.Catalogo{Obras: store}

	// Y tambien GestionDeclaraciones: el editor de splits de la #30. El
	// asiento de auditoria (#23) lo escribe el propio adaptador dentro de la
	// misma transaccion -no un BitacoraAuditoria aparte-, ver puertos.go.
	declaraciones := aplicacion.Declaraciones{
		Gestion: store,
		Reloj:   reloj.Sistema{},
	}

	api := httpapi.Nueva(store, httpapi.Casos{
		Auth:          autenticacion,
		Catalogo:      catalogo,
		Declaraciones: declaraciones,
	}, httpapi.Opciones{
		OrigenesPermitidos: config.Lista("CORS_ORIGENES"),
		Log:                registro,
	})

	return api.Router(), nil
}
