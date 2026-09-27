import { useEffect, useState } from "react";

/**
 * Cuanto espera un buscador antes de pedir, cuando lo que cambio es lo que se
 * teclea: `useValorDiferido(texto, DEBOUNCE_TECLEO_MS)`.
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
 * Devuelve `valor` tras `ms` de quietud. El PRIMER valor sale sin esperar: no
 * hay rafaga que agrupar, y hacer esperar la carga inicial por un debounce que
 * no sirve para nada es subirle la latencia a la pantalla.
 *
 * Se difiere el VALOR que se teclea y no la peticion, porque el `path` de una
 * consulta mezcla lo que se teclea con lo que no -la pagina, un boton-, y un
 * clic de paginacion no tiene por que esperar a nadie. Quien sabe cual es cual
 * es la pantalla, y `useApi` se queda sin saber de tecleo.
 */
export function useValorDiferido<T>(valor: T, ms: number): T {
  const [diferido, setDiferido] = useState(valor);

  useEffect(() => {
    // Cada valor nuevo vuelve a montar el efecto y con el se va el temporizador
    // anterior: diez teclas seguidas emiten UNA vez.
    const temporizador = setTimeout(() => setDiferido(valor), ms);
    return () => clearTimeout(temporizador);
  }, [valor, ms]);

  return diferido;
}
