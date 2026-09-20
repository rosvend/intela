import { useEffect, useRef, useState } from "react";
import { api, esErrorDeApi } from "./api";

type EstadoDeApi<T> =
  | { datos: null; cargando: true; error: null }
  | { datos: T; cargando: false; error: null }
  | { datos: null; cargando: false; error: Error };

// Lo que se dice cuando un 2xx no trae JSON. `api()` devuelve entonces el
// `Response` crudo, y eso no es el dato: es la respuesta sin leer.
const MENSAJE_SIN_JSON = "la respuesta no vino en JSON";

/**
 * Cuanto espera `useApi` antes de pedir, cuando el consumidor le pasa
 * `debounceMs`: los buscadores que piden mientras se teclea.
 *
 * 250 ms y no otro numero. Por encima del hueco entre dos teclas de alguien que
 * escribe seguido -una palabra entera no llega a esa pausa, asi que cabe en UNA
 * peticion- y por debajo de la espera que ya se lee como que la pantalla no
 * responde. Ni 0, que es la peticion por tecla que este valor existe para
 * evitar, ni 1000, que convierte cada busqueda en una pausa perceptible.
 *
 * Vive aqui, declarada una vez, y los dos buscadores la importan: copiada en
 * cada pantalla serian dos oportunidades de divergir -y la que se quedara
 * atras no tendria sintoma hasta que alguien contara las peticiones-.
 */
export const DEBOUNCE_TECLEO_MS = 250;

/**
 * Hook minimo sobre `api()` para un GET por pantalla.
 *
 * React-query entra cuando aparezca un segundo consumidor del mismo recurso
 * o la necesidad real de revalidar -bandeja ONI, alertas, Sprint 3- no
 * antes: para una sola pantalla con un solo fetch esto es lo mismo con mas
 * ceremonia alrededor (issue #19, DEC-1).
 *
 * `debounceMs` es opcional y por defecto 0: sin el, el hook pide en el mismo
 * tick del efecto, que es como se comportaba antes de que existiera. Con el,
 * un cambio de `path` -lo que produce teclear- espera a que el tecleo pare; la
 * carga inicial de la pantalla no espera, porque ahi no hay tecleo que agrupar.
 */
export function useApi<T>(path: string, debounceMs = 0): EstadoDeApi<T> {
  const [estado, setEstado] = useState<EstadoDeApi<T>>({
    datos: null,
    cargando: true,
    error: null,
  });

  // La PRIMERA peticion de este montaje sale ya -no hay rafaga que agrupar, y
  // hacer esperar la carga inicial de la pantalla por un debounce que no sirve
  // para nada es subirle la latencia a todas las pantallas que lo pidan-. A
  // partir de ahi el `path` cambia porque alguien teclea, y ahi si espera.
  const esLaPrimeraCarga = useRef(true);

  useEffect(() => {
    let vigente = true;
    const primeraCarga = esLaPrimeraCarga.current;
    esLaPrimeraCarga.current = false;
    setEstado({ datos: null, cargando: true, error: null });

    // El aborto va ADEMAS de la bandera, no en su lugar. La bandera sigue
    // haciendo lo suyo -que una respuesta vieja no pinte encima de la nueva-,
    // pero por si sola dejaba la peticion vieja EN VUELO: salia igual, gastaba
    // servidor y podia llegar tarde. Con el `signal`, cambiar de recurso corta
    // la que ya no sirve.
    const controlador = new AbortController();

    const consultar = () =>
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

    // Sin `debounceMs` la peticion sale ya -un consumidor que no lo pase no
    // cambia de comportamiento, y eso es lo que hace retrocompatible el
    // parametro-. Con el, un cambio de `path` espera a que el tecleo pare: cada
    // `path` nuevo vuelve a montar el efecto y con el se va el temporizador
    // anterior, asi que diez teclas seguidas son UNA peticion. No es
    // `setTimeout(consultar, 0)` para el caso por defecto: eso meteria un tick
    // de espera en TODAS las pantallas de lectura, y esto es para dos.
    const temporizador =
      debounceMs > 0 && !primeraCarga
        ? setTimeout(consultar, debounceMs)
        : null;
    if (temporizador === null) consultar();

    // Al desmontar o al cambiar de recurso: la bandera evita escribir estado,
    // el temporizador que aun no vencio se cancela -no tiene sentido pedir lo
    // que ya nadie mira- y la peticion en vuelo se aborta.
    return () => {
      vigente = false;
      if (temporizador !== null) clearTimeout(temporizador);
      controlador.abort();
    };
  }, [path, debounceMs]);

  return estado;
}
