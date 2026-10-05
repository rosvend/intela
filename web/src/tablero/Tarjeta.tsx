import { ReactNode } from "react";
import Detalle from "../ui/Detalle";
import { BotonEnlace } from "./BotonEnlace";
import { Recurso } from "./tipos";
import "../staff.css";

type Props<T> = {
  titulo: string;
  to?: string;
  etiquetaEnlace?: string;
  recurso: Recurso<T>;
  mensajeAusente?: string;
  /** Icono decorativo junto al titulo. */
  icono?: ReactNode;
  /** Explicacion en lenguaje llano, detras de un `Detalle`. */
  ayuda?: ReactNode;
  className?: string;
  children: (datos: T) => ReactNode;
};

/**
 * Tarjeta de KPI reutilizable. Recibe un `Recurso` y no asume que el
 * backend exista: cargando, ausente, error y listo son estados de
 * primer nivel. Cada tarjeta falla sola: nunca tumba la pagina.
 */
export function Tarjeta<T>({
  titulo,
  to,
  etiquetaEnlace,
  recurso,
  mensajeAusente = "Sin datos todavía",
  icono,
  ayuda,
  className,
  children,
}: Props<T>) {
  const etiqueta = etiquetaEnlace ?? `Ver ${titulo.toLowerCase()}`;

  return (
    <article className={className ? `tarjeta ${className}` : "tarjeta"}>
      <div className="tarjeta-cabecera">
        {icono && (
          <span className="tarjeta-icono" aria-hidden="true">
            {icono}
          </span>
        )}
        <h2 className="tarjeta-etiqueta">{titulo}</h2>
        {ayuda && (
          <Detalle
            titulo={titulo}
            claseDisparador="tarjeta-ayuda"
            etiquetaDisparador={`Qué significa ${titulo}`}
            disparador={<span aria-hidden="true">?</span>}
          >
            <span className="detalle-texto">{ayuda}</span>
          </Detalle>
        )}
      </div>
      <div className="tarjeta-cuerpo">
        {cuerpo(recurso, mensajeAusente, children)}
      </div>
      {to && (
        <div className="tarjeta-accion">
          <BotonEnlace to={to}>{etiqueta}</BotonEnlace>
        </div>
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
      return <Esqueleto />;
    case "ausente":
    case "inactivo":
      return <p className="muted tarjeta-vacio">{mensajeAusente}</p>;
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

/** Placeholder con brillo; el texto real queda para el lector de pantalla. */
export function Esqueleto({ lineas = 1 }: { lineas?: number }) {
  return (
    <div className="esqueleto" role="status">
      <span className="solo-lector">Cargando…</span>
      <span className="esqueleto-bloque esqueleto-cifra" aria-hidden="true" />
      {Array.from({ length: lineas - 1 }, (_, i) => (
        <span key={i} className="esqueleto-bloque" aria-hidden="true" />
      ))}
    </div>
  );
}
