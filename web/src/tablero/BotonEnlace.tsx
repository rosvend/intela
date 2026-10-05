import { ReactNode } from "react";
import { Link, LinkProps } from "react-router-dom";
import { ArrowRightIcon } from "@heroicons/react/16/solid";
import "../staff.css";

/** CTA de navegacion con forma de boton; la flecha es decorativa. */
export function BotonEnlace({
  to,
  state,
  primario = false,
  children,
}: {
  to: string;
  state?: LinkProps["state"];
  primario?: boolean;
  children: ReactNode;
}) {
  const clase = `${primario ? "boton-primario" : "boton-secundario"} boton-enlace`;
  const contenido = (
    <>
      {children}
      <ArrowRightIcon className="boton-enlace-flecha" aria-hidden="true" />
    </>
  );
  // Un fragmento es un ancla de la misma pagina: no pasa por el router.
  return to.startsWith("#") ? (
    <a className={clase} href={to}>
      {contenido}
    </a>
  ) : (
    <Link className={clase} to={to} state={state}>
      {contenido}
    </Link>
  );
}
