import { useEffect, useState } from "react";
import { ApiError, ErrorDeCuerpoIlegible, ErrorDeRed, api } from "./api";

type EstadoDeApi<T> =
  | { datos: null; cargando: true; error: null }
  | { datos: T; cargando: false; error: null }
  | { datos: null; cargando: false; error: Error };

// Lo que se dice cuando un 2xx no trae JSON. `api()` devuelve entonces el
// `Response` crudo, y eso no es el dato: es la respuesta sin leer.
const MENSAJE_SIN_JSON = "la respuesta no vino en JSON";

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
        if (!vigente) return;
        // Un `Response` aqui significa que el 2xx no traia JSON: `api()`
        // devuelve la respuesta cruda cuando el content-type no es JSON, y ese
        // objeto no es el dato prometido. Entregarlo como `datos` es lo que
        // tumbaba al listado: quien pidio `Carga[]` recibia un `Response` y
        // reventaba al pedirle `.length` o `.map` -una pagina de error servida
        // con un 2xx basta-. `T` es una promesa, no una comprobacion, asi que
        // la unica frontera donde se puede notar es esta, que es la que conoce
        // la diferencia. Se trata como lo que es: un fallo, no un dato.
        if (datos instanceof Response) {
          setEstado({
            datos: null,
            cargando: false,
            error: new Error(MENSAJE_SIN_JSON),
          });
          return;
        }
        setEstado({ datos, cargando: false, error: null });
      })
      .catch((error: unknown) => {
        if (!vigente) return;
        // Los tres errores que `api()` sabe lanzar llegan con su mensaje. Sin
        // el tercero, un 2xx con el cuerpo ilegible -`ErrorDeCuerpoIlegible`-
        // caia aqui como "error desconocido", y se perdia lo unico que ese tipo
        // explica: el servidor contesto bien y lo que no llego fue el cuerpo.
        const errorTipado =
          error instanceof ApiError ||
          error instanceof ErrorDeRed ||
          error instanceof ErrorDeCuerpoIlegible
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
