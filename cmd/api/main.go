// Command api sirve la API HTTP.
//
// No aplica migraciones ni siembra datos al arrancar. Las migraciones son de
// goose y corren como paso propio del despliegue; el seed es cmd/seed y solo
// se invoca a mano en desarrollo.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/cripto"
	"github.com/rosvend/intela/internal/infraestructura/exportacion"
	"github.com/rosvend/intela/internal/infraestructura/httpapi"
	"github.com/rosvend/intela/internal/infraestructura/ingesta"
	"github.com/rosvend/intela/internal/infraestructura/objetos"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// dirObjetosPorDefecto es la boveda de reportes crudos cuando nadie fija
// OBJECT_DIR. Relativa al directorio de trabajo, igual que en cmd/seed y por
// lo mismo: `/data` no se puede crear en una maquina de desarrollo. En
// contenedor la ruta la fija OBJECT_DIR, que es lo que hace docker-compose.yml.
const dirObjetosPorDefecto = "./data/objetos"

func main() {
	log := config.Logger("api")
	if err := ejecutar(log); err != nil {
		log.Error("arranque fallido", slog.Any("error", err))
		os.Exit(1)
	}
}

// ejecutar devuelve error en vez de llamar a log.Fatal.
//
// log.Fatal llama a os.Exit(1), que NO corre los defer: con el patron
// anterior, el defer store.CerrarPool() era codigo muerto y el pool nunca se
// cerraba limpiamente.
func ejecutar(log *slog.Logger) error {
	// NotifyContext cancela el contexto al recibir SIGINT o SIGTERM, que es
	// lo que manda `docker compose down`.
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	dsn := config.Cadena("DATABASE_URL", "")
	if dsn == "" {
		return errors.New("falta DATABASE_URL")
	}

	ctxConexion, cancelar := context.WithTimeout(ctx, 10*time.Second)
	defer cancelar()

	store, err := postgres.Abrir(ctxConexion, dsn)
	if err != nil {
		return err
	}
	defer store.CerrarPool()

	// Aqui es donde se juntan las dos orillas: el nucleo declara los puertos y
	// este es el unico sitio del binario que sabe que adaptador satisface cada
	// uno. El mismo *Store satisface RepositorioAfiliacion y Sesiones; que sean
	// el mismo tipo es asunto suyo, el nucleo sigue viendo dos interfaces.
	autenticacion := aplicacion.Autenticacion{
		Usuarios: store,
		Claves:   cripto.Bcrypt{},
		Sesiones: store,
		Reloj:    reloj.Sistema{},
		Tokens:   cripto.TokensAleatorios{},
		TTL:      config.Duracion("SESION_TTL", 12*time.Hour),
	}

	// El mismo *Store satisface tambien CatalogoObras, BitacoraAuditoria,
	// UnidadDeTrabajo y -por el puerto GestionDeclaraciones- la lectura de la
	// declaracion vigente que el catalogo necesita para decir en que estado
	// esta cada obra. El nucleo sigue viendo puertos separados: que el
	// adaptador sea uno solo es asunto suyo, y es lo que permite que el
	// asiento del alta comparta transaccion con la obra (ADR 0006, #91).
	catalogo := aplicacion.Catalogo{
		Obras:         store,
		Bitacora:      store,
		Unidad:        store,
		Reloj:         reloj.Sistema{},
		Declaraciones: store,
	}

	// El padron de titulares, que es de donde el editor de splits saca las
	// partes de una declaracion. La satisface el mismo *Store, y con esto es
	// la primera lectura de `titulares` en produccion.
	padron := aplicacion.Titulares{Padron: store}

	// Y tambien GestionDeclaraciones: el editor de splits de la #30. El
	// asiento de auditoria (#23) lo escribe el propio adaptador dentro de la
	// misma transaccion -no un BitacoraAuditoria aparte-, ver puertos.go.
	//
	// El guardia de R-01 apunta al STORE, no a `padron`. Es el mismo adaptador
	// -por eso los dos satisfacen el puerto-, pero no es el mismo camino:
	// `padron` es el MODELO DE LECTURA, y un modelo de lectura recorta. Hoy
	// mete un tope por defecto de pagina
	// ([aplicacion.Titulares.BuscarTitulares] lo aplica con `ConDefecto`), y
	// cualquier dia puede recortar por algo mas -"el padron es de escritores"-
	// sin que nadie lo mire. Un guardia que mira otra cosa que la tabla que
	// guarda lo que se le pide comprueba lo que le dejen, y ese dia R-01
	// dejaria pasar a una sociedad en silencio, que es el defecto caro de esta
	// regla. El cableado es decision de este main, asi que la decision se
	// escribe aqui: el nucleo no conoce ninguno de los dos.
	declaraciones := aplicacion.Declaraciones{
		Gestion: store,
		Padron:  store,
		Reloj:   reloj.Sistema{},
	}

	// El lado del ingreso (#27). Dos puertos del mismo adaptador: se lee desde
	// mas sitios de los que se escriben, y quien solo consulta bolsas no tiene
	// por que poder registrar dinero.
	recaudo := aplicacion.Recaudo{
		Bolsas:  store,
		Gestion: store,
		Reloj:   reloj.Sistema{},
	}

	liquidaciones := aplicacion.ServicioLiquidacion{
		Repo: store,
		Exportador: exportacion.Combinado{
			XLSX: exportacion.GeneradorExcel{},
			Docs: exportacion.GeneradorPDF{},
		},
	}

	// La ingesta de reportes de uso: la base para el acuse y las filas, la
	// boveda de disco para la evidencia cruda, y el catalogo de adaptadores de
	// formato para leer lo que llega.
	//
	// El catalogo se construye AL ARRANCAR y su error tumba el proceso. Un mapa
	// de columnas mal escrito es un defecto del programa, no de la entrega:
	// descubrirlo aqui cuesta un arranque fallido, y descubrirlo en la primera
	// subida cuesta una entrega perdida con el cliente esperando.
	lectores, err := ingesta.CatalogoDelCliente()
	if err != nil {
		return fmt.Errorf("construir los adaptadores de ingesta: %w", err)
	}
	log.Info("adaptadores de ingesta listos", slog.Any("fuentes", ingesta.Fuentes(lectores)))

	recepcion := aplicacion.Ingesta{
		Reportes:              store,
		Almacen:               objetos.Disco{Dir: config.Cadena("OBJECT_DIR", dirObjetosPorDefecto)},
		Lectores:              lectores,
		SnapshotNormalizacion: store.SnapshotNormalizacion,
	}

	// El flujo de aprobaciones de RD 13.5 (#34). Seis puertos, un solo
	// *Store: es el mismo patron que declaraciones/recaudo de mas arriba,
	// aplicado a un agregado con mas costuras.
	procesos := aplicacion.Procesos{
		Repo:          store,
		Parametros:    store,
		Bolsas:        store,
		Declaraciones: store,
		Usos:          store,
		Resultados:    store,
		Unidad:        store,
	}

	api := httpapi.Nueva(httpapi.Casos{
		Salud:         store,
		Auth:          autenticacion,
		Catalogo:      catalogo,
		Padron:        padron,
		Ingesta:       recepcion,
		Declaraciones: declaraciones,
		Recaudo:       recaudo,
		Liquidaciones: liquidaciones,
		Procesos:      procesos,
		Cola:          aplicacion.Normalizacion{Reportes: store},
	}, httpapi.Opciones{
		OrigenesPermitidos: config.Lista("CORS_ORIGENES"),
		Log:                log,
	})

	srv := &http.Server{
		Addr:    config.Cadena("ADDR", ":8080"),
		Handler: api.Router(),

		// Los cuatro, no solo el de cabeceras. Con solo
		// ReadHeaderTimeout, una subida lenta a un endpoint de reportes
		// retiene la conexion indefinidamente.
		ReadHeaderTimeout: config.Duracion("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:       config.Duracion("HTTP_READ_TIMEOUT", 60*time.Second),
		WriteTimeout:      config.Duracion("HTTP_WRITE_TIMEOUT", 60*time.Second),
		IdleTimeout:       config.Duracion("HTTP_IDLE_TIMEOUT", 120*time.Second),

		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	errServidor := make(chan error, 1)
	go func() {
		log.Info("escuchando", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errServidor <- err
			return
		}
		errServidor <- nil
	}()

	select {
	case err := <-errServidor:
		return err
	case <-ctx.Done():
		log.Info("senal recibida, cerrando")
	}

	// Contexto nuevo: el de arriba ya esta cancelado, y con el no habria
	// margen para terminar las peticiones en vuelo.
	ctxApagado, cancelarApagado := context.WithTimeout(
		context.Background(), config.Duracion("SHUTDOWN_TIMEOUT", 15*time.Second))
	defer cancelarApagado()

	if err := srv.Shutdown(ctxApagado); err != nil {
		return err
	}
	log.Info("cerrado limpiamente")
	return nil
}
