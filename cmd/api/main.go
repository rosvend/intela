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
	"github.com/rosvend/intela/internal/infraestructura/modelolenguaje"
	"github.com/rosvend/intela/internal/infraestructura/objetos"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
	"github.com/rosvend/intela/internal/infraestructura/triage"
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

	// S3 si hay OBJECT_BUCKET; si no, disco, que es lo de desarrollo y docker compose.
	boveda, err := objetos.Boveda(ctx, config.Cadena("OBJECT_BUCKET", ""), config.Cadena("OBJECT_DIR", dirObjetosPorDefecto))
	if err != nil {
		return err
	}

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
		// ASVS V3.3.2: sin uso durante este lapso la sesion caduca en el servidor (ADR 0025).
		Inactividad: config.Duracion("SESION_INACTIVIDAD", 30*time.Minute),
	}

	admision := aplicacion.Admision{
		Solicitudes: store,
		Objetos:     boveda,
		IDs:         cripto.TokensAleatorios{},
		Claves:      cripto.Bcrypt{},
	}

	// La ingesta de reportes de uso: la base para el acuse y las filas, la
	// boveda para la evidencia cruda, y el catalogo de adaptadores de
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

	// Cinco puertos y no dos desde el ADR 0019 y el 0006: emitir una orden de
	// pago son la orden, el cierre de las diferidas que absorbe, el asiento de
	// cada una y la notificacion que arranca el plazo de R-10, y las cuatro son
	// UN hecho. El mismo *Store satisface el repositorio, la bitacora y la
	// unidad de trabajo; el nucleo sigue viendo tres puertos distintos.
	//
	// El aviso va por el portal (RD 13.8.8) y en la misma transaccion que la
	// orden: el titular la ve en /mis-liquidaciones desde el commit que la
	// emite, y un aviso no puede sobrevivir a una orden revertida (#193).
	ordenes := aplicacion.Liquidaciones{
		Ordenes:     store,
		Reloj:       reloj.Sistema{},
		Notificador: store.AvisoPortal(),
		Bitacora:    store,
		Unidad:      store,
	}

	// El mismo *Store cubre ONI, declaraciones, padron y recaudo, y tambien
	// CatalogoObras, BitacoraAuditoria y UnidadDeTrabajo. CatalogoObras va por
	// un envoltorio (ver postgres/catalogo.go): PorID ya es el de la bitacora.
	// Declaraciones se lee aparte para componer el estado de cada obra. El
	// nucleo sigue viendo puertos separados: que el adaptador sea uno solo es
	// asunto suyo, y es lo que permite que el asiento del alta comparta
	// transaccion con la obra (ADR 0006, #91).
	catalogo := aplicacion.Catalogo{
		Obras:         store.CatalogoObras(),
		Bitacora:      store,
		Unidad:        store,
		Reloj:         reloj.Sistema{},
		Declaraciones: store,
	}

	// El padron de titulares, que es de donde el editor de splits saca las
	// partes de una declaracion. La satisface el mismo *Store, y con esto es
	// la primera lectura de `titulares` en produccion.
	padron := aplicacion.Titulares{Padron: store}

	// El asiento de auditoria de declaraciones y recaudo lo escribe el
	// propio adaptador dentro de la misma transaccion -no un
	// BitacoraAuditoria aparte-, ver puertos.go.
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

	recaudo := aplicacion.Recaudo{
		Bolsas:  store,
		Gestion: store,
		Reloj:   reloj.Sistema{},
	}

	reporte := aplicacion.ServicioLiquidacion{
		Repo: store,
		Exportador: exportacion.Combinado{
			XLSX: exportacion.GeneradorExcel{},
			Docs: exportacion.GeneradorPDF{},
		},
	}

	recepcion := aplicacion.Ingesta{
		Reportes:              store,
		Almacen:               boveda,
		Lectores:              lectores,
		SnapshotNormalizacion: store.SnapshotNormalizacion,
	}

	// La deteccion de anomalias de un periodo (#37). Seis puertos del mismo
	// *Store, y tres de ellos son ESTRECHOS a proposito: `Entregas` solo lee
	// -- no puede llamar a GuardarEntrega, que quema la huella de un archivo
	// --, `Declaraciones` es el mismo LectorDeDeclaraciones que usa el
	// catalogo y `Coautores` es una sola consulta. La unidad de trabajo esta
	// porque las alertas y su asiento tienen que ser un solo hecho (ADR 0006).
	anomalias := aplicacion.Anomalias{
		Entregas:      store,
		Declaraciones: store,
		Coautores:     store,
		Alertas:       store,
		// Cerrar una critica corrige el dato (#164): sin esto POST
		// /alertas/{id}/resolver falla cerrado en vez de cerrar sin corregir.
		Correcciones: store,
		Bitacora:     store,
		Unidad:       store,
		Reloj:        reloj.Sistema{},
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
		Anomalias:     anomalias,
		Bitacora:      store,
		Reloj:         reloj.Sistema{},
		Origen:        store,
		// Al entrar a liquidacion_final del nacional emite las ordenes del
		// periodo en la misma unidad que la etapa (#193, ADR 0024).
		Liquidacion: ordenes,
	}

	// Sin ruta: la abre #34. El caso de uso queda armado para que liberar
	// una reserva y asentar reserva.liberada sean una sola unidad (#177).
	bolsas := aplicacion.BolsasAccesorias{
		Resultados:    store,
		Reservas:      store,
		Rendimientos:  store,
		Reclamaciones: store,
		Corridas:      store,
		Bitacora:      store,
		Unidad:        store,
		Reloj:         reloj.Sistema{},
	}

	// El asistente de solo lectura (#66). Sin proveedor configurado responde "no disponible": nunca tumba el arranque.
	modelo, proveedor := modelolenguaje.Elegir(
		config.Cadena("AGENTE_PROVEEDOR", ""),
		config.Cadena("ANTHROPIC_API_KEY", ""),
		config.Cadena("AGENTE_MODELO", ""),
	)
	log.Info("asistente", slog.String("proveedor", proveedor))
	herramientas, err := aplicacion.NuevoCatalogoHerramientas()
	if err != nil {
		return err
	}
	agente := aplicacion.AgenteConsulta{
		Modelo:       modelo,
		Herramientas: herramientas,
		Reloj:        reloj.Sistema{},
		Log:          log,
		Plazo:        config.Duracion("AGENTE_PLAZO", 50*time.Second),
	}

	api := httpapi.Nueva(httpapi.Casos{
		Salud:      store,
		Auth:       autenticacion,
		Ordenes:    ordenes,
		Admision:   admision,
		Catalogo:   catalogo,
		ListadoONI: aplicacion.ConsultarListadoONI{ONI: store},
		PublicarONI: aplicacion.PublicarListadoONI{
			ONI:         store,
			Bitacora:    store,
			Reloj:       reloj.Sistema{},
			Tx:          store,
			Fisica:      config.Cadena("ONI_DIRECCION_FISICA", ""),
			Electronica: config.Cadena("ONI_DIRECCION_ELECTRONICA", ""),
		},
		Padron:        padron,
		Ingesta:       recepcion,
		Declaraciones: declaraciones,
		Recaudo:       recaudo,
		Reporte:       reporte,
		Procesos:      procesos,
		Cola:          aplicacion.Normalizacion{Reportes: store},
		Anomalias:     anomalias,
		Auditoria:     aplicacion.Auditoria{Bitacora: store},
		Identificacion: aplicacion.CasosIdentificacion{
			Repo: store, Ejemplos: store, Rankeador: triage.Heuristico{},
		},
		Resolucion: aplicacion.ResolucionIdentificacion{
			Repo: store, Bitacora: store, Unidad: store, Reloj: reloj.Sistema{},
			Ejemplos: store, Rankeador: triage.Heuristico{},
		},
		Explicar: aplicacion.ExplicarCifra{Bitacora: store},
		Ingresos: aplicacion.ConsultaIngresos{Repo: store},
		Tablero:  aplicacion.Tablero{Repo: store},
		Bolsas:   bolsas,
		Agente:   agente,
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
