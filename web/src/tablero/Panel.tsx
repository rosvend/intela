import { ReactNode } from "react";
import { Esqueleto } from "./Tarjeta";
import { Recurso } from "./tipos";

type Props<T> = {
  titulo: string;
  recurso: Recurso<T>;
  /** Chip o enlace a la derecha del titulo. */
  accion?: ReactNode;
  mensajeAusente?: string;
  className?: string;
  children: (datos: T) => ReactNode;
};

/** Panel del tablero de staff: mismos estados que `Tarjeta`, cada uno falla solo. */
export function Panel<T>({
  titulo,
  recurso,
  accion,
  mensajeAusente = "Sin datos todavía",
  className,
  children,
}: Props<T>) {
  return (
    <section className={className ? `panel ${className}` : "panel"}>
      <header className="panel-cabecera">
        <h2 className="panel-titulo">{titulo}</h2>
        {accion}
      </header>
      {recurso.tipo === "cargando" && <Esqueleto lineas={3} />}
      {(recurso.tipo === "ausente" || recurso.tipo === "inactivo") && (
        <p className="muted panel-vacio">{mensajeAusente}</p>
      )}
      {recurso.tipo === "error" && (
        <p className="tarjeta-error" role="alert">
          {recurso.mensaje}
        </p>
      )}
      {recurso.tipo === "listo" && children(recurso.datos)}
    </section>
  );
}
