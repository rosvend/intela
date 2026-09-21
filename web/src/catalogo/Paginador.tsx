/**
 * El pie de una pagina de resultados: el tramo que se ve, y los botones redondos de pagina anterior y siguiente.
 *
 * Sin `total` en la respuesta no se puede decir "pagina 2 de 7": "Siguiente" se
 * ofrece cuando la pagina vino entera -si el servidor devolvio menos de `limite`,
 * no hay mas filas- y una pagina vacia despues lo dice sin afirmar que la lista
 * este vacia.
 *
 * `etiqueta` es lo que se pagina en plural ("Obras", "Titulares"): el texto del
 * rango es el unico que cambia entre las dos pantallas que lo usan.
 *
 * Con un titulo en el filtro el catalogo se ordena por PARECIDO y el paginado es
 * por desplazamiento: si el catalogo cambia mientras se navega, una obra nueva
 * mas parecida empuja a las demas hacia abajo y una fila puede repetirse o
 * saltarse entre paginas. No se promete estabilidad, igual que no se prometen
 * totales: es lo que el servidor puede dar sin `total` ni cursor.
 */
function Chevron({ haciaLaDerecha }: { haciaLaDerecha: boolean }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d={haciaLaDerecha ? "M6 3l5 5-5 5" : "M10 3L5 8l5 5"} />
    </svg>
  );
}

export default function Paginador({
  etiqueta,
  desplazamiento,
  cuantas,
  limite,
  onIrA,
}: {
  etiqueta: string;
  desplazamiento: number;
  cuantas: number;
  limite: number;
  onIrA: (nuevoDesplazamiento: number) => void;
}) {
  return (
    <div className="catalogo-pie">
      <span>{`${etiqueta} ${desplazamiento + 1} a ${desplazamiento + cuantas}`}</span>
      <div className="catalogo-pie-botones">
        <button
          type="button"
          className="catalogo-pagina"
          aria-label="Página anterior"
          title="Página anterior"
          disabled={desplazamiento === 0}
          onClick={() => onIrA(Math.max(0, desplazamiento - limite))}
        >
          <Chevron haciaLaDerecha={false} />
        </button>
        <button
          type="button"
          className="catalogo-pagina"
          aria-label="Página siguiente"
          title="Página siguiente"
          disabled={cuantas < limite}
          onClick={() => onIrA(desplazamiento + limite)}
        >
          <Chevron haciaLaDerecha />
        </button>
      </div>
    </div>
  );
}
