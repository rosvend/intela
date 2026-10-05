import type { ReactNode } from "react";
import type { Recurso } from "../tablero/tipos";
import "../titular.css";

type Props<T> = {
  recurso: Recurso<T>;
  esqueleto: ReactNode;
  vacio: { titulo: string; texto: string };
  children: (datos: T) => ReactNode;
};

/** Los cuatro estados de un Recurso del tablero, con esqueleto y vacio amable. */
export default function EstadoRecurso<T>({
  recurso,
  esqueleto,
  vacio,
  children,
}: Props<T>) {
  switch (recurso.tipo) {
    case "cargando":
      return (
        <div role="status" aria-label="Cargando">
          {esqueleto}
        </div>
      );
    case "ausente":
    case "inactivo":
      return (
        <div className="vacio">
          <p className="vacio-titulo">{vacio.titulo}</p>
          <p className="muted">{vacio.texto}</p>
        </div>
      );
    case "error":
      return (
        <p role="alert" className="titular-error">
          {recurso.mensaje}
        </p>
      );
    case "listo":
      return <>{children(recurso.datos)}</>;
  }
}
