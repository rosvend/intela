import { useEffect, useState } from "react";
import { apiPublica } from "../api";

export type ObraONIPublica = {
  id: string;
  titulo: string;
  fuente: string;
  ids_fuente: string;
  modalidad: string;
};

export type ListadoONI = {
  periodo: string;
  fecha_proceso: string;
  direccion_fisica: string;
  direccion_electronica: string;
  explicacion: string;
  obras: ObraONIPublica[];
};

/**
 * Pagina publica del listado ONI (R-18, RD 13.8.1 a 13.8.4).
 *
 * No pide sesion: la publicacion en la web es la obligacion legal y el
 * ancla de los tres anos de prescripcion (R-19). No pinta montos.
 */
export default function ListadoONI() {
  const [listado, setListado] = useState<ListadoONI | null>(null);
  const [error, setError] = useState("");
  const [cargando, setCargando] = useState(true);

  useEffect(() => {
    let vigente = true;
    apiPublica("/api/publico/oni")
      .then((r) => {
        if (!vigente) return;
        setListado(r);
        setCargando(false);
      })
      .catch((e: Error) => {
        if (!vigente) return;
        setError(e.message);
        setCargando(false);
      });
    return () => {
      vigente = false;
    };
  }, []);

  return (
    <section>
      <p className="badge">Listado publico</p>
      <h1>Obras no identificadas (ONI)</h1>
      {cargando ? (
        <p>Consultando el listado publicado...</p>
      ) : error ? (
        <p role="alert">No se pudo cargar el listado: {error}</p>
      ) : !listado ? (
        <p>No hay un listado ONI publicado todavia.</p>
      ) : (
        <Contenido listado={listado} />
      )}
    </section>
  );
}

function Contenido({ listado }: { listado: ListadoONI }) {
  const fecha = formatearFecha(listado.fecha_proceso);

  return (
    <>
      <div className="card">
        <p>
          <strong>Periodo:</strong> {listado.periodo}
        </p>
        <p>
          <strong>Fecha del proceso:</strong> {fecha}
        </p>
        <p>
          <strong>Direccion fisica:</strong> {listado.direccion_fisica}
        </p>
        <p>
          <strong>Direccion electronica:</strong>{" "}
          <a href={`mailto:${listado.direccion_electronica}`}>
            {listado.direccion_electronica}
          </a>
        </p>
        <p className="muted">{listado.explicacion}</p>
      </div>

      {listado.obras.length === 0 ? (
        <p>Este periodo no dejo obras sin identificar.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Titulo</th>
              <th>Fuente</th>
              <th>Identificadores</th>
              <th>Modalidad</th>
            </tr>
          </thead>
          <tbody>
            {listado.obras.map((o) => (
              <tr key={o.id}>
                <td>{o.titulo}</td>
                <td>{o.fuente}</td>
                <td>{o.ids_fuente || "—"}</td>
                <td>{o.modalidad}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

function formatearFecha(rfc3339: string) {
  const d = new Date(rfc3339);
  if (Number.isNaN(d.getTime())) return rfc3339;
  return d.toLocaleDateString("es-CO", { dateStyle: "long", timeZone: "UTC" });
}
