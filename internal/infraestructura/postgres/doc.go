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
// Los fija el caso de uso en aplicacion, no el adaptador. La forma es
// [Store.EnTransaccion], y los metodos que participan RECIBEN la pgx.Tx como
// parametro.
//
// Lo que no se va a hacer, y conviene decirlo antes de que alguien lo intente:
// meter en Store un campo mutable con la transaccion en curso. *Store es un
// singleton del proceso; dos casos de uso concurrentes se pisarian la
// transaccion, y el fallo no seria un panico sino una cifra distinta.
//
// Las lecturas de este paquete van directas al pool: no hay nada que
// coordinar en una sola consulta.
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
