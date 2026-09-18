import type { Obra } from "./tipos";

/**
 * La etiqueta de estado de la declaracion de una obra: el UNICO sitio donde se
 * deciden su texto y su clase.
 *
 * Vive en un archivo propio, y no dentro de `Catalogo.tsx` donde nacio (paso 5),
 * porque ya no es del listado solo: el detalle de obra (paso 6) pinta el mismo
 * estado del mismo dato, y dejar la funcion ahi obligaria a la pantalla de
 * detalle a importar de la del listado -una pantalla dependiendo de otra- para
 * un badge que ninguna de las dos posee. Lo que dos pantallas hermanas
 * comparten va al lado de lo que ya comparten: `tipos.ts` y `declaracion.ts`.
 * En `declaracion.ts` no cabe: ese archivo dice de si mismo que es logica pura
 * "sin React y sin red", y esto es un componente. Las clases si siguen
 * compartidas en `styles.css`, que es de donde las leen las dos.
 *
 * `estado_declaracion` viene del backend y se pinta tal cual, sin re-derivarlo.
 * Lo unico que decide el cliente es SI HAY declaracion, y eso lo dice
 * `version_vigente`: `null` quiere decir que la obra no tiene ninguna, y un
 * numero, la version que sostiene el estado. Hace falta porque las dos
 * situaciones -"nadie la declaro" y "declarada y no suma 100"- llegan con el
 * MISMO estado, `incompleta`, y con la suma en 0 la una y menor que 100 la otra:
 * `version_vigente` es lo unico que las separa (D-008), y pintar "Incompleta"
 * sobre una obra que nadie declaro afirmaria una declaracion que no existe. Es
 * ademas el dato que un administrador necesita para saber si la obra esta
 * bloqueada porque falta declararla o porque declara de menos.
 *
 * Que esto sea un solo componente es parte del asunto: mientras el texto y la
 * clase se decidieron en dos funciones distintas, con el payload validado por
 * los guards y nada que atara las dos pantallas, cambiar una sola de las dos
 * dejaba al listado y al detalle afirmando cosas distintas del MISMO hecho sin
 * que ningun test lo notara. Los guards validan el cuerpo, no el rotulo que cada
 * pantalla pinta de el.
 *
 * "Incompleta" es un estado valido del negocio, no un error: se pinta en ambar
 * -`badge-estado-incompleta`-, nunca en rojo.
 */
export function EtiquetaDeDeclaracion({ obra }: { obra: Obra }) {
  const sinDeclaracion = obra.version_vigente === null;
  const clase = sinDeclaracion
    ? "badge-estado-sin-declaracion"
    : obra.estado_declaracion === "completa"
      ? "badge-estado-completa"
      : "badge-estado-incompleta";
  const texto = sinDeclaracion
    ? "Sin declaración"
    : obra.estado_declaracion === "completa"
      ? "Completa"
      : "Incompleta";

  return (
    <span className={`badge-estado ${clase}`}>
      <span className="badge-estado-punto" aria-hidden="true" />
      {texto}
    </span>
  );
}
