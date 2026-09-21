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
// Y dos que NO son de goose: `primer-administrador` y `sembrar-dataset`.
// Estan aqui porque el problema que resuelven es el mismo -- hay que ejecutar
// algo DENTRO de la VPC contra una base sin endpoint publico -- y esta es la
// unica funcion que ya vive ahi con DATABASE_URL. Levantar una Lambda propia
// para cada operacion de un solo uso es infraestructura que hay que mantener
// para siempre.
//
// Deuda (ADR 0017): con la segunda orden ajena a goose, este binario ya no es
// solo migraciones. La siguiente de este tipo tiene que sacar las tres a su
// propia funcion; no anadir una cuarta aqui.
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
	"github.com/rosvend/intela/internal/infraestructura/objetos"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/semilla"
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

// ordenSembrarDataset carga el dataset sintetico completo (cmd/seed /
// semilla.Cargar): titulares, obras, declaraciones, bolsas, reportes, usos
// identificados y parametros. Misma razon que primer-administrador: la base no
// tiene endpoint publico y cmd/seed no viaja en la imagen de la API.
//
// Alias: `sembrar-titulares-demo` sigue aceptandose y hace lo mismo -- el
// nombre viejo del PR que solo sembraba el padron.
const ordenSembrarDataset = "sembrar-dataset"

const ordenSembrarTitularesDemo = "sembrar-titulares-demo" // alias de sembrar-dataset

// dirObjetosLambda es donde caen los bytes de los reportes del seed. /tmp es lo
// unico escribible en provided.al2023; la API no los relee desde aqui (solo el
// metadato en Postgres). Cuando exista el adaptador S3, se cablea igual que en
// cmd/lambda.
const dirObjetosLambda = "/tmp/objetos"

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
//
// Reset solo lo usa `sembrar-dataset`: equivale a SEED_RESET=true. Se niega si
// hay asientos o datos que no son del dataset (mismas guardas que cmd/seed).
type peticion struct {
	Orden string `json:"orden"`

	ID     string `json:"id,omitempty"`
	Email  string `json:"email,omitempty"`
	Nombre string `json:"nombre,omitempty"`
	Hash   string `json:"hash,omitempty"`

	Reset bool `json:"reset,omitempty"`
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
		if orden == ordenSembrarDataset || orden == ordenSembrarTitularesDemo {
			return sembrarDataset(ctx, p, log)
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

// sembrarDataset persiste el dataset sintetico via semilla.Cargar.
//
// Idempotente sin reset: si el juego ya esta completo, responde "ya sembrado".
// Con reset:true aplica las mismas guardas que SEED_RESET en cmd/seed.
//
// El admin provisionado se conserva (ON CONFLICT en usuarios). Las otras
// cuentas demo (distribucion, contabilidad, auditor, titular) se crean con las
// SEED_CLAVE_* del entorno, defaults de docs/ARRANQUE.md.
func sembrarDataset(ctx context.Context, p peticion, log *slog.Logger) (respuesta, error) {
	ctx, cancelar := context.WithTimeout(ctx,
		config.Duracion("MIGRATE_TIMEOUT", 4*time.Minute))
	defer cancelar()

	store, err := postgres.Abrir(ctx, config.Cadena("DATABASE_URL", ""))
	if err != nil {
		log.Error("abrir la base", slog.Any("error", err))
		return respuesta{}, err
	}
	defer store.CerrarPool()

	esperado := len(semilla.Construir().Obras)
	var obrasAntes int
	if err := store.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM obras`).Scan(&obrasAntes); err != nil {
		log.Error("contar obras", slog.Any("error", err))
		return respuesta{}, fmt.Errorf("contar obras: %w", err)
	}

	almacen := objetos.Disco{Dir: config.Cadena("OBJECT_DIR", dirObjetosLambda)}
	claves := semilla.Claves{
		Admin:        config.Cadena("SEED_CLAVE_ADMIN", "admin-local"),
		Distribucion: config.Cadena("SEED_CLAVE_DISTRIBUCION", "distribucion-local"),
		Contabilidad: config.Cadena("SEED_CLAVE_CONTABILIDAD", "contabilidad-local"),
		Auditor:      config.Cadena("SEED_CLAVE_AUDITOR", "auditor-local"),
		Titular:      config.Cadena("SEED_CLAVE_TITULAR", "ana-local"),
	}

	if err := semilla.Cargar(ctx, store, almacen, cripto.Bcrypt{}, claves, p.Reset, log); err != nil {
		log.Error("semilla fallida", slog.Any("error", err))
		return respuesta{}, err
	}

	estado := "cargado"
	if !p.Reset && obrasAntes == esperado {
		estado = "ya sembrado"
	}
	log.Info("dataset sintetico", slog.String("estado", estado), slog.Bool("reset", p.Reset))
	return respuesta{Orden: ordenSembrarDataset, Estado: estado}, nil
}
