import "../titular.css";

/** Filas con brillo mientras carga; el lector de pantalla oye "Cargando". */
export default function EsqueletoFilas({ filas = 3 }: { filas?: number }) {
  return (
    <div role="status" aria-label="Cargando" className="esqueleto-lista">
      {Array.from({ length: filas }, (_, i) => (
        <span key={i} className="esqueleto esqueleto-fila" />
      ))}
    </div>
  );
}
