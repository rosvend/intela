/**
 * Las categorias por las que se busca en el catalogo: la unica lista. El menu
 * del selector, los chips y el campo la leen de aqui.
 *
 * Es configuracion pura y sin JSX -por eso los iconos no viven aqui: el
 * buscador los asocia por `id`-, para probarla sin renderizar nada.
 *
 * El `id` es el nombre EXACTO del parametro de `GET /obras` y tambien el de la
 * URL (`titulo` parcial sin distinguir mayusculas, `genero` exacto, `ipi`
 * exacto, `anio` entero positivo). Una sola forma de nombrarlos evita la
 * traduccion que se desvia.
 */

export type CategoriaId = "titulo" | "genero" | "ipi" | "anio";

export interface CategoriaDeBusqueda {
  id: CategoriaId;
  etiqueta: string;
  placeholder: string;
  inputMode: "text" | "numeric";
  /**
   * `vivo`: cada tecla filtra (la peticion se difiere con el debounce de
   * `Catalogo`). `confirmar`: el valor se aplica con Enter o con la lupa.
   */
  modo: "vivo" | "confirmar";
  /** El mensaje de por que el texto no se aplica, o `null` si es aplicable. */
  validar?: (texto: string) => string | null;
}

/**
 * La forma de un anio que el backend acepta: un entero positivo. Es la misma
 * regla de `buscarObras` (`strconv.Atoi` y `anio <= 0` = 400).
 *
 * Existe solo para no mandar una consulta que se sabe rechazada, no para
 * sustituir al backend: la autoridad para aceptar o rechazar sigue siendo el
 * servidor, y si su regla cambia, su mensaje es el que se ve en pantalla.
 */
const FORMA_ANIO = /^\d+$/;

export function esAnioAplicable(texto: string): boolean {
  return FORMA_ANIO.test(texto) && Number(texto) > 0;
}

export const CATEGORIAS: readonly CategoriaDeBusqueda[] = [
  {
    id: "titulo",
    etiqueta: "Título",
    placeholder: "Parte del título…",
    inputMode: "text",
    modo: "vivo",
  },
  {
    id: "genero",
    etiqueta: "Género",
    placeholder: "Género exacto, p. ej. Drama",
    inputMode: "text",
    modo: "confirmar",
  },
  {
    id: "ipi",
    etiqueta: "IPI de coautor",
    placeholder: "IPI exacto del coautor",
    inputMode: "text",
    modo: "confirmar",
  },
  {
    id: "anio",
    etiqueta: "Año",
    placeholder: "Año exacto, p. ej. 1991",
    // Campo de texto y no `type="number"`: un `number` descarta lo que no
    // parsea sin decirlo y su valor llega vacio, con lo que la pantalla no
    // podria explicar por que no esta filtrando.
    inputMode: "numeric",
    modo: "confirmar",
    validar: (texto) =>
      esAnioAplicable(texto) ? null : "Escribe un año entero positivo.",
  },
];

export function categoriaPorId(id: CategoriaId): CategoriaDeBusqueda {
  const categoria = CATEGORIAS.find((c) => c.id === id);
  if (categoria === undefined) throw new Error(`categoria desconocida: ${id}`);
  return categoria;
}
