import type { EstadoDeclaracion, Obra } from "./tipos";

/**
 * La etiqueta de estado de UNA declaracion: el estado que el backend calculo
 * para ella, con el texto y la clase que le corresponden.
 *
 * Es el UNICO sitio donde se deciden el texto y la clase de `completa` y
 * `incompleta`, y existe separada de `EtiquetaDeDeclaracion` porque hay dos
 * sitios que conocen un estado y NO conocen una obra: el historial de versiones
 * (paso 7), donde cada version trae su propio `estado` y no hay ninguna obra de
 * la que leerlo. Antes de esto los dos rotulos se decidieron en una funcion que
 * pedia una `Obra` entera, asi que el historial habria tenido que fabricarse una
 * obra falsa -o repetir las dos lineas-, y las dos pantallas habrian podido
 * discrepar del MISMO estado sin que nada lo notara: los guards validan el
 * cuerpo, no el rotulo que cada pantalla pinta de el.
 *
 * `estado` viene del backend y se pinta tal cual, sin re-derivarlo de las
 * partes. "Incompleta" es un estado valido del negocio, no un error: se pinta
 * en ambar -`badge-estado-incompleta`-, nunca en rojo.
 */
export function EtiquetaDeEstado({ estado }: { estado: EstadoDeclaracion }) {
  const completa = estado === "completa";
  const clase = completa ? "badge-estado-completa" : "badge-estado-incompleta";

  return (
    <span className={`badge-estado ${clase}`}>
      <span className="badge-estado-punto" aria-hidden="true" />
      {completa ? "Completa" : "Incompleta"}
    </span>
  );
}

/**
 * La etiqueta de estado de la declaracion de una obra: el estado que el
 * backend calculo para su version vigente, o el aviso de que no hay ninguna.
 *
 * El estado lo pinta `EtiquetaDeEstado`, que es donde vive su texto; esta
 * funcion solo anade la distincion que necesita una OBRA y no una version.
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
 */
export function EtiquetaDeDeclaracion({ obra }: { obra: Obra }) {
  if (obra.version_vigente === null) return <EtiquetaSinDeclaracion />;
  return <EtiquetaDeEstado estado={obra.estado_declaracion} />;
}

/**
 * Lo que se pinta para una obra sin ninguna declaracion: ni `Completa` ni
 * `Incompleta`, porque no hay ninguna declaracion de la que decir un estado.
 */
function EtiquetaSinDeclaracion() {
  return (
    <span className="badge-estado badge-estado-sin-declaracion">
      <span className="badge-estado-punto" aria-hidden="true" />
      Sin declaración
    </span>
  );
}
