import type { components } from "../contrato";

export type Rechazo = components["schemas"]["Rechazo"];

/**
 * Si un valor sin tipar tiene la forma de un `Rechazo`: los cuatro campos que
 * lee esta tabla, y solo esos.
 *
 * Se comprueba el TIPO y no que el texto traiga algo dentro, porque vacio si es
 * un valor legitimo aqui: una fila cuyo titulo venia en blanco es justo una de
 * las que se rechaza (`internal/aplicacion/ingesta.go`, `validarUso`: "titulo
 * vacio: sin titulo no hay nada que identificar"), asi que llega con
 * `titulo: ""`. Exigir contenido convertiria una respuesta real en un error de
 * la pantalla. Lo que no puede pasar es un objeto o una lista: React no los
 * acepta como hijo -"Objects are not valid as a React child"- y el panel entero
 * se muere. Es el caso que reproduce la revision: un rechazo con `ids_fuente`
 * objeto mientras el contrato lo declara `string` (api/openapi.yaml).
 */
export function esRechazo(valor: unknown): valor is Rechazo {
  if (typeof valor !== "object" || valor === null) return false;
  const rechazo = valor as {
    id?: unknown;
    titulo?: unknown;
    ids_fuente?: unknown;
    motivo?: unknown;
  };
  return (
    typeof rechazo.id === "string" &&
    typeof rechazo.titulo === "string" &&
    typeof rechazo.ids_fuente === "string" &&
    typeof rechazo.motivo === "string"
  );
}

/**
 * El log de rechazos de una entrega: una fila por cada fila del archivo que
 * no se pudo normalizar, con su motivo. La usan el panel de resultado de la
 * subida y el listado de cargas.
 *
 * La primera columna se titula "Id" y no "Fila" a proposito: lo que trae es el
 * id interno del rechazo (`rep-<huella>-<n>`). Ese `n` cuenta TODAS las filas
 * parseadas del archivo, las aceptadas incluidas (es la posicion en el lote, no
 * entre los rechazos), asi que no es un orden de rechazos ni es la linea de la
 * hoja: un `rep-...-1` puede ser el primer rechazo y a la vez la segunda fila
 * del archivo. La linea solo aparece dentro del motivo, y no todos los motivos
 * la traen (los de validacion de campos y los de normalizacion no la numeran).
 * Titularla "Fila" prometeria una ubicacion en el archivo que el dato no da;
 * guardar la linea de la hoja en `usos_rechazados` es un seguimiento aparte. El
 * id, en cambio, si es unico y sirve para reportar el caso.
 *
 * Solo lleva id, titulo, ids de la fuente y motivo. Nunca medidas ni importes
 * (ADR 0016): un rechazo se explica por lo que le falto, no por lo que habria
 * pesado en el reparto. Con la lista vacia dice que no hay, en vez de pintar
 * una tabla sin filas.
 */
export default function TablaRechazos({
  rechazos,
}: {
  rechazos: readonly Rechazo[];
}) {
  if (rechazos.length === 0) {
    return <p className="muted">No hay filas rechazadas.</p>;
  }

  return (
    <table className="tabla-rechazos" aria-label="Filas rechazadas">
      <thead>
        <tr>
          <th scope="col">Id</th>
          <th scope="col">Título</th>
          <th scope="col">IDs de la fuente</th>
          <th scope="col">Motivo</th>
        </tr>
      </thead>
      <tbody>
        {rechazos.map((rechazo) => (
          <tr key={rechazo.id}>
            <td className="tabla-rechazos-id">{rechazo.id}</td>
            <td>{rechazo.titulo}</td>
            {/* Una linea `clave=valor` por identificador (ADR 0018). */}
            <td className="tabla-rechazos-ids">{rechazo.ids_fuente}</td>
            <td>{rechazo.motivo}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
