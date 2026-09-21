import { useRef, type ChangeEvent, type DragEvent } from "react";

// Clic en la zona abre el selector; el input real va oculto pero enfocable.
export default function ZonaSubida({
  idArchivo,
  arrastrando,
  alArrastrar,
  alSalir,
  alSoltar,
  alElegir,
}: {
  idArchivo: string;
  arrastrando: boolean;
  alArrastrar: (evento: DragEvent<HTMLDivElement>) => void;
  alSalir: () => void;
  alSoltar: (evento: DragEvent<HTMLDivElement>) => void;
  alElegir: (archivo: File | null) => void;
}) {
  const entrada = useRef<HTMLInputElement>(null);

  function alCambiar(evento: ChangeEvent<HTMLInputElement>) {
    alElegir(evento.target.files?.[0] ?? null);
  }

  return (
    <div
      className={
        arrastrando ? "ingesta-soltar ingesta-soltar-activa" : "ingesta-soltar"
      }
      onClick={() => entrada.current?.click()}
      onDragOver={alArrastrar}
      onDragLeave={alSalir}
      onDrop={alSoltar}
    >
      {/* Ilustracion en linea: pila de hojas con acento vinotinto. */}
      <svg
        className="ingesta-soltar-ilustracion"
        viewBox="0 0 96 96"
        aria-hidden="true"
        focusable="false"
      >
        <g className="ingesta-soltar-hoja ingesta-soltar-hoja-atras">
          <rect x="18" y="22" width="48" height="58" rx="6" />
        </g>
        <g className="ingesta-soltar-hoja ingesta-soltar-hoja-atras">
          <rect x="26" y="16" width="48" height="58" rx="6" />
        </g>
        <g className="ingesta-soltar-hoja ingesta-soltar-hoja-frente">
          <rect x="34" y="10" width="48" height="58" rx="6" />
          <path d="M68 10v8a6 6 0 0 0 6 6h8z" />
          <rect x="42" y="30" width="20" height="4" rx="2" />
          <rect x="42" y="38" width="32" height="4" rx="2" />
          <rect x="42" y="54" width="32" height="14" rx="3" />
        </g>
      </svg>
      <p className="ingesta-soltar-titulo">Subir archivos</p>
      <p className="ingesta-soltar-texto">
        Arrastra el archivo hasta aquí o{" "}
        <span className="ingesta-soltar-enlace">haz clic para elegirlo</span>.
      </p>
      <p className="ingesta-ayuda">Formatos: .csv, .xlsx, .json</p>
      {/* El label real va oculto: conserva el nombre accesible "Archivo". */}
      <label htmlFor={idArchivo} className="ingesta-soltar-oculto">
        Archivo
      </label>
      {/* `accept` solo filtra el selector; lo suelto lo juzga el backend. */}
      <input
        id={idArchivo}
        ref={entrada}
        type="file"
        accept=".csv,.xlsx,.json"
        onChange={alCambiar}
        onClick={(e) => e.stopPropagation()}
        className="ingesta-soltar-entrada"
      />
    </div>
  );
}
