/**
 * Quien tiene el foco, para que el juego no le quite las teclas al formulario.
 *
 * Vive aparte de `SnakeCanvas` porque es la regla que mas facil se rompe y la
 * que menos se puede probar desde el componente: jsdom no implementa el
 * contexto 2D del lienzo, asi que en las pruebas el efecto se retira antes de
 * registrar el `keydown` y no hay forma de comprobar la guarda a traves de el.
 * Como funcion suelta si se puede.
 */

/**
 * Cierto si el elemento es un sitio donde se escribe.
 *
 * Lo editable se comprueba antes que el tipo porque un `div` editable no es
 * ninguno de los tres campos y aun asi recibe texto -- y ahi las flechas mueven
 * el cursor, que es justo lo que no se puede robar.
 *
 * Y se comprueba de DOS formas, que cubren huecos distintas:
 *
 *  - `isContentEditable` tiene en cuenta la herencia: un `<span>` dentro de un
 *    div editable es editable y no lleva atributo propio.
 *  - `closest` mira el atributo. Hace falta porque jsdom no implementa
 *    `isContentEditable` -- devuelve false incluso con el atributo puesto --,
 *    asi que sin esto la regla no se podria probar, que es como se cuelan las
 *    regresiones en la guarda mas importante de esta pantalla.
 *
 * `:not([contenteditable="false"])` porque el atributo admite apagarse, y un
 * subarbol marcado como no editable no recibe texto.
 */
export function esCampoDeTexto(el: Element | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  if (el.isContentEditable) return true;
  if (el.closest('[contenteditable]:not([contenteditable="false"])')) {
    return true;
  }
  return (
    el instanceof HTMLInputElement ||
    el instanceof HTMLTextAreaElement ||
    el instanceof HTMLSelectElement
  );
}
