import { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Recurso } from "./tipos";

type Props<T> = {
  titulo: string;
  descripcion?: string;
  to?: string;
  etiquetaEnlace?: string;
  recurso: Recurso<T>;
  mensajeAusente?: string;
  children: (datos: T) => ReactNode;
};

/**
 * Tarjeta de KPI reutilizable. Recibe un `Recurso` y no asume que el
 * backend exista: cargando, ausente, error y listo son estados de
 * primer nivel. Los paneles de #40 y #42 pueden usarla tal cual.
 */
export function Tarjeta<T>({
  titulo,
  descripcion,
  to,
  etiquetaEnlace,
  recurso,
  mensajeAusente = "Sin datos todavía",
  children,
}: Props<T>) {
  return (
    <article className="tarjeta">
      <h2 className="tarjeta-etiqueta">{titulo}</h2>
      {descripcion && <p className="tarjeta-descripcion">{descripcion}</p>}
      <div className="tarjeta-cuerpo">
        {cuerpo(recurso, mensajeAusente, children)}
      </div>
      {to && (
        <Link className="tarjeta-enlace" to={to}>
          {etiquetaEnlace ?? `Ver ${titulo.toLowerCase()}`}
        </Link>
      )}
    </article>
  );
}

function cuerpo<T>(
  recurso: Recurso<T>,
  mensajeAusente: string,
  children: (datos: T) => ReactNode,
): ReactNode {
  switch (recurso.tipo) {
    case "cargando":
      return (
        <p className="muted" role="status">
          Cargando…
        </p>
      );
    case "ausente":
    case "inactivo":
      return <p className="muted">{mensajeAusente}</p>;
    case "error":
      return (
        <p className="tarjeta-error" role="alert">
          {recurso.mensaje}
        </p>
      );
    case "listo":
      return children(recurso.datos);
  }
}
