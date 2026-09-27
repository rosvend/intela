import { DocumentTextIcon, TrashIcon } from "@heroicons/react/24/outline";

// KB con un decimal; debajo de 1 KB se dice en bytes.
function formatearBytes(nbytes: number): string {
  if (nbytes < 1024) return `${nbytes} B`;
  const kb = nbytes / 1024;
  if (kb < 1024) return `${kb.toFixed(1)} KB`;
  return `${(kb / 1024).toFixed(1)} MB`;
}

// Fila del archivo elegido, con estado y retiro; sin E/S propia.
export default function FichaArchivo({
  archivo,
  enviando,
  alQuitar,
}: {
  archivo: File;
  enviando: boolean;
  alQuitar: () => void;
}) {
  return (
    <div className="ingesta-ficha" aria-live="polite">
      <span className="ingesta-ficha-icono" aria-hidden="true">
        <DocumentTextIcon />
      </span>
      <div className="ingesta-ficha-datos">
        <p className="ingesta-ficha-nombre">{archivo.name}</p>
        <p className="ingesta-ficha-estado">
          {formatearBytes(archivo.size)}
          <span aria-hidden="true"> · </span>
          {enviando ? "Subiendo…" : "Listo para subir"}
        </p>
        {enviando && (
          <div
            className="ingesta-ficha-progreso"
            role="progressbar"
            aria-label={`Subiendo ${archivo.name}`}
          >
            <span className="ingesta-ficha-barra" />
          </div>
        )}
      </div>
      {!enviando && (
        <button
          type="button"
          className="ingesta-ficha-quitar"
          onClick={alQuitar}
          aria-label={`Quitar ${archivo.name}`}
        >
          <TrashIcon aria-hidden="true" />
        </button>
      )}
    </div>
  );
}
