/**
 * El pie de una pagina de resultados: el tramo que se ve, y Anterior / Siguiente.
 *
 * Sin `total` en la respuesta no se puede decir "pagina 2 de 7": "Siguiente" se
 * ofrece cuando la pagina vino entera -si el servidor devolvio menos de `limite`,
 * no hay mas filas- y una pagina vacia despues lo dice sin afirmar que la lista
 * este vacia.
 *
 * `etiqueta` es lo que se pagina en plural ("Obras", "Titulares"): el texto del
 * rango es el unico que cambia entre las dos pantallas que lo usan.
 */
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
          disabled={desplazamiento === 0}
          onClick={() => onIrA(Math.max(0, desplazamiento - limite))}
        >
          Anterior
        </button>
        <button
          type="button"
          className="catalogo-pagina"
          disabled={cuantas < limite}
          onClick={() => onIrA(desplazamiento + limite)}
        >
          Siguiente
        </button>
      </div>
    </div>
  );
}
