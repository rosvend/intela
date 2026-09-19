import {
  ROTULO_NOMBRE_EN_PADRON_ACTUAL,
  formatearPorcentaje,
} from "./declaracion";
import type { Parte } from "./tipos";

/**
 * Las partes de UNA version de la Declaracion de Obra: una fila por titular,
 * con el porcentaje de cada uno.
 *
 * Vive en un archivo propio, y no dentro de `DetalleObra.tsx` donde nacio (paso
 * 6), porque ya no es de una pantalla sola: el historial de versiones (paso 7)
 * pinta la MISMA tabla, con las mismas cuatro columnas y el mismo vacio en la
 * del nombre, una vez por version. Dos copias del mismo mapeo -`titular_id` en
 * mono, el guion de "no se conoce", el porcentaje a cuatro decimales- es la
 * clase de defecto que el repo ya pago: el dia que una de las dos cambie, las
 * dos pantallas dicen cosas distintas del mismo dato, y ningun test lo nota
 * porque cada una se prueba por su lado. Lo que dos pantallas hermanas
 * comparten va al lado de lo que ya comparten: `tipos.ts`, `declaracion.ts` y
 * `EtiquetaDeDeclaracion.tsx`.
 *
 * La columna del nombre se rotula con `ROTULO_NOMBRE_EN_PADRON_ACTUAL`, no con
 * "Nombre" a secas: el nombre no viaja en la parte y no es un dato de la
 * version -se resuelve contra el padron de HOY, asi que un titular renombrado
 * apareceria con su nombre nuevo hasta en las versiones antiguas (D-006)-. El
 * rotulo dice de donde saldria el dato; el `titular_id` va en la primera
 * columna, visible, para poder conciliar la pantalla con la API y para
 * distinguir homonimos.
 *
 * Esa columna va vacia, con el guion que el catalogo ya usa para "no se
 * conoce": resolver el nombre aqui exigiria barrer el padron -`GET /titulares`
 * se sirve paginado y no admite filtrar por identificador, asi que una pagina
 * que no traiga al titular NO prueba que no exista-, y la celda no puede decir
 * ni el nombre ni que falte. Lo mismo vale para una version antigua, y con mas
 * motivo: el nombre de hoy no es el que tenia entonces.
 *
 * `titulo` es el nombre accesible de la tabla y lo unico que cambia entre las
 * dos pantallas que la montan: el detalle la titula por la version vigente y el
 * historial por la version que pinta cada bloque. Un nombre accesible tiene que
 * decir de que tabla es -dos tablas identicas en la misma pantalla no se
 * distinguen de otro modo-, y es tambien lo que deja a un test decir cual esta
 * leyendo. `tabla-partes` sigue siendo la clase compartida, la lea quien la lea.
 */
export function TablaDePartes({
  partes,
  titulo,
}: {
  partes: readonly Parte[];
  titulo: string;
}) {
  return (
    <>
      <div className="catalogo-caja">
        <table className="tabla-partes" aria-label={titulo}>
          <thead>
            <tr>
              <th scope="col">Titular</th>
              <th scope="col">IPI</th>
              <th scope="col">{ROTULO_NOMBRE_EN_PADRON_ACTUAL}</th>
              <th scope="col" className="tabla-partes-numero">
                Porcentaje
              </th>
            </tr>
          </thead>
          <tbody>
            {partes.map((parte) => (
              // El `titular_id` como clave: el backend rechaza una declaracion
              // con un titular repetido, asi que no hay dos filas con el mismo.
              <tr key={parte.titular_id}>
                <td className="detalle-identificador">{parte.titular_id}</td>
                <td className="detalle-identificador">{parte.ipi}</td>
                <td className="muted">—</td>
                <td className="tabla-partes-numero">
                  {formatearPorcentaje(parte.porcentaje)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="muted detalle-nota">
        Esta pantalla no busca el nombre en el padrón: se sirve por páginas y no
        admite filtrar por identificador, así que una página que no traiga al
        titular no probaría que no exista. La columna se rotula «en el padrón
        actual» porque, el día que se resuelva, el nombre será el de hoy y no el
        de la fecha de esta versión (D-006).
      </p>
    </>
  );
}
