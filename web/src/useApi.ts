import { useEffect, useState } from "react";
import { api, esErrorDeApi } from "./api";

export type EstadoDeApi<T> =
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
 *
 * Pide en el mismo tick del efecto cada vez que cambia `path`. Los buscadores
 * que piden mientras se teclea no esperan aqui: difieren el texto ANTES de
 * armar el `path` (ver `useValorDiferido`), porque el hook no sabe cual parte
 * del `path` es tecleo y cual es un clic.
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

    // El aborto va ADEMAS de la bandera, no en su lugar. La bandera sigue
    // haciendo lo suyo -que una respuesta vieja no pinte encima de la nueva-,
    // pero por si sola dejaba la peticion vieja EN VUELO: salia igual, gastaba
    // servidor y podia llegar tarde. Con el `signal`, cambiar de recurso corta
    // la que ya no sirve.
    const controlador = new AbortController();

    (api(path, { signal: controlador.signal }) as Promise<T>)
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
        // Los errores que `api()` sabe lanzar llegan con su mensaje: sin el
        // tercero -`ErrorDeCuerpoIlegible`-, un 2xx con el cuerpo ilegible
        // caia aqui como "error desconocido", y se perdia lo unico que ese
        // tipo explica: el servidor contesto bien y lo que no llego fue el
        // cuerpo. Quien decide cuales son esos errores es `esErrorDeApi`, en
        // el modulo que los define, y no una lista copiada aqui (D-016).
        const errorTipado = esErrorDeApi(error)
          ? error
          : new Error("error desconocido");
        setEstado({ datos: null, cargando: false, error: errorTipado });
      });

    // Al desmontar o al cambiar de recurso: la bandera evita escribir estado y
    // la peticion en vuelo se aborta.
    return () => {
      vigente = false;
      controlador.abort();
    };
  }, [path]);

  return estado;
}
