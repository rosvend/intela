import { useEffect, useState } from "react";
import { ApiError, ErrorDeRed, api } from "./api";

type EstadoDeApi<T> =
  | { datos: null; cargando: true; error: null }
  | { datos: T; cargando: false; error: null }
  | { datos: null; cargando: false; error: Error };

/**
 * Hook minimo sobre `api()` para un GET por pantalla.
 *
 * React-query entra cuando aparezca un segundo consumidor del mismo recurso
 * o la necesidad real de revalidar -bandeja ONI, alertas, Sprint 3- no
 * antes: para una sola pantalla con un solo fetch esto es lo mismo con mas
 * ceremonia alrededor (issue #19, DEC-1).
 */
export function useApi<T>(path: string): EstadoDeApi<T> {
  const [estado, setEstado] = useState<EstadoDeApi<T>>({
    datos: null,
    cargando: true,
    error: null,
  });

  useEffect(() => {
    let vigente = true;
    setEstado({ datos: null, cargando: true, error: null });

    (api(path) as Promise<T>)
      .then((datos) => {
        if (vigente) setEstado({ datos, cargando: false, error: null });
      })
      .catch((error: unknown) => {
        if (!vigente) return;
        const errorTipado =
          error instanceof ApiError || error instanceof ErrorDeRed
            ? error
            : new Error("error desconocido");
        setEstado({ datos: null, cargando: false, error: errorTipado });
      });

    // Evita escribir estado si la pantalla cambio de recurso o se desmonto
    // mientras la peticion seguia en vuelo.
    return () => {
      vigente = false;
    };
  }, [path]);

  return estado;
}
