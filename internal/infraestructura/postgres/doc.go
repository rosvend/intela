// Package postgres adapta los puertos de persistencia contra PostgreSQL.
//
// Este documento fija el patron que siguen todos los repositorios. Vive aqui
// y no en docs/ para que `go doc` lo sirva junto al codigo que describe, y
// para que quien anada un repositorio lo tenga delante y no en otra pestana.
//
// # Un fichero por puerto
//
// afiliacion.go, repertorio.go, y asi con cada uno. El nombre es el del modulo
// del ADR 0003, no el del tipo.
//
// Importa porque el ADR 0003 partio los puertos precisamente para que no
// existiera una interfaz de 39 metodos: cada caso de uso declara solo el que
// usa, y eso es lo que hace exigible la frontera en revision. Un store.go de
// 39 metodos deshace esa separacion en el adaptador -y no lo nota nadie,
// porque sigue compilando.
//
// # La asercion de compilacion
//
// Cada fichero declara, justo despues de los imports:
//
//	var _ aplicacion.RepositorioRepertorio = (*Store)(nil)
//
// Antes del SQL, para que quien lea vea el contrato primero. Sin ella, un
// desajuste entre puerto y adaptador no aparece hasta que se compila cmd/api,
// que es tarde y lejos del cambio que lo causo.
//
// Un solo *Store satisface varios puertos. Eso es asunto del adaptador: el
// nucleo sigue viendo interfaces separadas.
//
// # Errores
//
// Todo error de pgx sale por [traducirError]. pgx.ErrNoRows se convierte en
// aplicacion.ErrNoEncontrado envuelto con contexto; cualquier otro conserva su
// causa. Nunca se devuelve un centinela pelado: un "no encontrado" a secas no
// dice cual de las consultas fue.
//
// Dos corolarios que se olvidan y cuestan caro:
//
//   - Un conjunto de resultados VACIO no es ErrNoEncontrado. Rows.Err() es nil
//     con cero filas; solo QueryRow(...).Scan produce pgx.ErrNoRows. Listar una
//     tabla vacia devuelve una lista vacia y ningun error.
//   - filas.Err() despues de cada bucle no es opcional. Un fallo a mitad de
//     stream sale solo por ahi: sin la comprobacion, una lista TRUNCADA se
//     devuelve como lista completa, y nadie se entera.
//
// # Los campos derivados se calculan en el dominio
//
// aplicacion.Obra.EstadoDecl no sale de ninguna columna -obras no la tiene-:
// sale de repertorio.Declaracion.Estado().
//
// No es una preferencia. R-04 (RD 13.1.3) tiene tres clausulas independientes
// -la suma da 100, cada porcentaje es positivo, y ninguna parte va sin IPI- y
// las tres viven juntas en el dominio. Un SUM(porcentaje) = 100 en SQL
// reimplementa una de las tres y discrepa en silencio de las otras dos. La
// propia migracion lo dice al declarar la tabla: el invariante es de agregado
// y no se puede expresar como CHECK de fila.
//
// La regla general: si una cifra se puede calcular mal, se calcula una sola
// vez, en el dominio, y el adaptador la pide.
//
// # Limites de transaccion
//
// Hay dos formas, y la diferencia es cuantos PUERTOS abarca el limite.
//
// Dentro de un solo puerto, el limite cabe en el metodo: [Store.EnTransaccion]
// abre la transaccion y las funciones que participan RECIBEN la pgx.Tx como
// parametro. Es lo que hacen [Store.Registrar] con `obras` y `obra_coautores`,
// o [Store.Guardar] con la version de la declaracion y su asiento.
//
// Cuando el limite abarca DOS puertos, lo declara el caso de uso con
// [Store.EnUnidad], que satisface [aplicacion.UnidadDeTrabajo]. Es el caso del
// catalogo desde la #91: [aplicacion.Catalogo] sostiene CatalogoObras y
// BitacoraAuditoria por separado -- el ADR 0003 no deja meter Asentar en el
// contrato del modulo -- y aun asi la obra y su asiento tienen que ser un solo
// hecho (ADR 0006).
//
// La transaccion de una unidad viaja EN EL CONTEXTO, y por eso un metodo solo
// participa si pide su ejecutor con [Store.ejecutorDe] (lecturas y escrituras
// sueltas) o abre con [Store.enTransaccionDe] (las que ya tenian transaccion
// propia). Desde la revision de PR #134 lo hace TODO el paquete: catalogo.go
// y bitacora.go, que son los dos puertos que la unidad del catalogo abarca
// (#91); parametros.go, que desde la #118 congela el snapshot con el que se
// abre un proceso -- el corte y el `procesos.snapshot_id` que lo referencia
// son un solo hecho --; y el resto (afiliacion.go, calendario.go, cola.go,
// declaraciones.go, identificacion.go, ingesta.go, provision.go, recaudo.go,
// reparto.go, repertorio.go, sesiones.go). Un metodo que se queda en `s.pool` es un
// metodo que NO puede participar en la unidad de otro puerto el dia que
// alguien lo necesite, y ese dia no avisa con un fallo de compilacion: avisa
// con una escritura que se confirma sola (ver
// TestRegistrarBolsaDentroDeUnaUnidadRevierteConLaDeFuera en
// recaudo_test.go) o con un interbloqueo por agotamiento del pool si la
// unidad de fuera ya tiene su conexion y la de dentro pide otra (ver
// TestRegistrarBolsaDentroDeUnaUnidadNoPideSegundaConexion, mismo fichero).
//
// La UNICA excepcion, y es a proposito, es [Store.Tomar] (cola.go): una sola
// sentencia (`FOR UPDATE SKIP LOCKED` + `UPDATE`) pensada para soltar su
// cerrojo de fila lo antes posible. Correr dentro de la transaccion de quien
// la llame alargaria ese cerrojo hasta que ESA transaccion entera termine,
// justo lo que el diseno de la cola quiere evitar. Tomar sigue yendo contra
// `s.pool` sin mirar el contexto, asi que NO participa en ninguna unidad de
// trabajo aunque el contexto traiga una: reclamar un trabajo desde dentro de
// una EnUnidad abre hoy una conexion aparte para esa reclamacion, sin atarla
// al destino de la unidad que la envuelve. Que un metodo pida su ejecutor es
// una promesa metodo por metodo, no una garantia de que TODA la logica de
// este paquete sea atomica con cualquier unidad que la envuelva -- Tomar es
// el que se queda fuera, y quien lo llame desde dentro de una unidad tiene
// que saberlo.
//
// Quien meta un puerto nuevo en una unidad tiene que revisar que sus metodos
// pidan su ejecutor: por el pool escribirian FUERA de la transaccion y se
// confirmarian aparte, que es justo el fallo que la unidad existe para
// impedir.
//
// Lo que no se va a hacer, y conviene decirlo antes de que alguien lo intente:
// meter en Store un campo mutable con la transaccion en curso. *Store es un
// singleton del proceso; dos casos de uso concurrentes se pisarian la
// transaccion, y el fallo no seria un panico sino una cifra distinta. El
// contexto no tiene ese problema porque es de la llamada, no del proceso.
//
// Una lectura o escritura fuera de cualquier unidad sigue yendo contra el
// pool -- [Store.ejecutorDe] se resuelve a el cuando el contexto no trae
// transaccion --, que es lo que hace que el mismo metodo sirva suelto y
// dentro de una unidad sin dos firmas.
//
// # Pruebas
//
// Contra PostgreSQL de verdad, nunca contra un doble: el esquema lleva
// invariantes de negocio -CHECKs, triggers, la EXCLUDE de vigencias- que un
// mock no tiene (ADR 0010). El harness es
// [github.com/rosvend/intela/internal/infraestructura/postgres/testhelp]: un
// contenedor por binario de pruebas, aislamiento por Restore, y skip con
// -short para que `go test -short ./...` no necesite Docker.
//
// # sqlc esta aplazado
//
// La tabla de stack del ADR 0010 dice "pgx v5 + sqlc". Este adaptador lleva
// SQL escrito a mano: sqlc anade un paso de generacion a CI antes de que haya
// consultas que merezcan generarse. Se revisa cuando crezca el volumen. No es
// un incumplimiento del ADR sin registrar, es esta linea.
package postgres
