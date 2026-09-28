import { useApi } from "../useApi";
import {
  ROTULO_NOMBRE_EN_PADRON_ACTUAL,
  formatearPorcentaje,
} from "./declaracion";
import { esTitular, type Parte, type Titular } from "./tipos";

/**
 * Los nombres que una pantalla consiguio resolver: `titular_id` -> nombre del
 * padron de HOY.
 *
 * Un `Map` y no un objeto, y no es un capricho de estilo: la clave es un
 * identificador que viene de fuera y con un objeto un `titular_id` llamado
 * `constructor` o `toString` encontraria una propiedad heredada en vez de nada.
 *
 * Los valores son SIEMPRE nombres con contenido: quien lo construye no guarda
 * una cadena vacia, asi que `get` devuelve o un nombre que se puede pintar o
 * `undefined`, y la tabla no tiene que distinguir "no se conoce" de "se conoce y
 * esta en blanco" -que seria un hueco en la celda-.
 */
export type NombresDeTitulares = ReadonlyMap<string, string>;

/**
 * Ningun nombre resuelto.
 *
 * Es lo que se le pasa a la tabla cuando la pantalla no tiene un solo
 * identificador que preguntar -una obra cuyas versiones no traen partes-, y lo
 * que deja la columna del nombre en el guion que significa "no se conoce".
 */
export const SIN_NOMBRES: NombresDeTitulares = new Map();

/**
 * Los nombres de los titulares que una pantalla va a pintar, en UNA peticion.
 *
 * Contrato: recibe los `titular_id` de TODAS las tablas que la pantalla monta
 * -no los de una fila, ni los de una tabla, ni los de una version- y devuelve un
 * mapa `id -> nombre` con lo que el padron haya reconocido. Nunca lanza y nunca
 * devuelve `undefined`: cuando no hay nombres, devuelve un mapa vacio y la tabla
 * se queda con el guion. **La lista no puede venir vacia**: quien llama
 * garantiza que hay algo que preguntar, porque el contrato de `ids` dice que una
 * lista vacia NO filtra -o sea que pide el padron entero-, y una peticion que no
 * puede cambiar lo que se ve no se hace.
 *
 * Pide EXACTAMENTE los identificadores que se van a pintar, con el filtro
 * acotado por identificador (`GET /titulares?ids=...`, item 9b). No pide el
 * padron paginado para cruzarlo en el cliente: eso traeria cientos de filas para
 * quedarse con dos, y dejaria sin nombre a cualquier titular que no cupiera en la
 * pagina -que es el defecto que ese filtro existe para no repetir-.
 *
 * Lo que no puede crecer con los datos es el numero de PETICIONES, y por eso el
 * hook vive aqui y no dentro de `TablaDePartes`: el historial monta una tabla por
 * version, y quien pregunta es la pantalla -una vez, por la union de los ids de
 * todas sus tablas-, no cada tabla por su cuenta. Abrir una version mas anade
 * identificadores a la misma consulta, no otra consulta.
 *
 * La lista se reduce y se ordena antes de viajar, tambien a proposito: `useApi`
 * vuelve a pedir en cuanto cambia el `path`, asi que el mismo conjunto de ids
 * tiene que producir la misma cadena en cada render o el efecto se dispararia una
 * y otra vez.
 *
 * No devuelve el estado de la peticion, y es deliberado: quien pinta no tiene
 * nada que decir cuando el padron no se pudo leer o no reconocio un id. La
 * columna del nombre es un dato de HOY y el guion significa "no se conoce", que
 * es exactamente lo que se sabe en los dos casos. Un aviso de error por una
 * consulta que solo rellena una columna accesoria taparia las cifras que si
 * llegaron -el identificador, el IPI y el porcentaje, que son el reparto-.
 */
export function useNombresDeTitulares(
  ids: readonly string[],
): NombresDeTitulares {
  const unicos = [...new Set(ids)].sort();
  const parametros = new URLSearchParams();
  for (const id of unicos) parametros.append("ids", id);

  const { datos } = useApi<Titular[]>(
    `/api/titulares?${parametros.toString()}`,
  );

  const nombres = new Map<string, string>();
  // La guarda es la misma que usa el padron del editor (`esTitular`) y por la
  // misma razon: `useApi<Titular[]>` promete una lista de titulares y no la
  // comprueba, asi que un 2xx con otra forma llegaria hasta un `.nombre` que no
  // existe. Lo que no se pudo leer no se inventa: la fila se queda sin nombre.
  if (Array.isArray(datos) && datos.every(esTitular)) {
    for (const titular of datos) {
      if (titular.nombre !== "") nombres.set(titular.id, titular.nombre);
    }
  }
  return nombres;
}

/**
 * Las partes de UNA version de la Declaracion de Obra: una fila por titular,
 * con el porcentaje de cada uno.
 *
 * Vive en un archivo propio, y no dentro de `DetalleObra.tsx` donde nacio (paso
 * 6), porque ya no es de una pantalla sola: el historial de versiones (paso 7)
 * pinta la MISMA tabla, con las mismas cuatro columnas, una vez por version. Dos
 * copias del mismo mapeo -`titular_id` en mono, el guion de "no se conoce", el
 * porcentaje a cuatro decimales- es la clase de defecto que el repo ya pago: el
 * dia que una de las dos cambie, las dos pantallas dicen cosas distintas del
 * mismo dato, y ningun test lo nota porque cada una se prueba por su lado. Lo que
 * dos pantallas hermanas comparten va al lado de lo que ya comparten: `tipos.ts`,
 * `declaracion.ts` y `EtiquetaDeDeclaracion.tsx`. Los nombres viajan en el mismo
 * espiritu: quien los resuelve es el hook de arriba, en una sola peticion por
 * pantalla, y aqui llegan ya resueltos.
 *
 * La columna del nombre se rotula con `ROTULO_NOMBRE_EN_PADRON_ACTUAL`, no con
 * "Nombre" a secas: el nombre no viaja en la parte y no es un dato de la version
 * -se resuelve contra el padron de HOY, asi que un titular renombrado aparece con
 * su nombre nuevo hasta en las versiones antiguas (D-006)-. El rotulo dice de
 * donde sale el dato; el `titular_id` va en la primera columna, visible, para
 * poder conciliar la pantalla con la API y para distinguir homonimos.
 *
 * Esa columna SI se pinta, y hasta el item 9b no se pintaba porque el handler de
 * `GET /titulares` no leia el parametro `ids` -el puerto y el SQL ya lo
 * soportaban-, no porque el padron no se pudiera preguntar. La acusacion que el
 * comentario de este sitio le hacia al contrato estaba equivocada, y con el
 * parametro leido la columna recibe los nombres de los identificadores que la
 * tabla muestra.
 *
 * Lo que NO cambia es lo que la columna significa, y el guion sigue siendo parte
 * de ello: una fila sin nombre -porque el padron no la reconocio, porque no se
 * pudo leer, o porque el nombre de hoy no es el que la version tenia cuando se
 * declaro (D-006)- se pinta con el guion que el catalogo ya usa para "no se
 * conoce". La celda no puede decir el nombre ni que el titular falte, y por eso
 * el guion no se sustituye ni por un hueco ni por un texto nuevo.
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
  nombres,
}: {
  partes: readonly Parte[];
  titulo: string;
  nombres: NombresDeTitulares;
}) {
  return (
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
              <td className="muted">{nombres.get(parte.titular_id) ?? "—"}</td>
              <td className="tabla-partes-numero">
                {formatearPorcentaje(parte.porcentaje)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
