import {
  CheckIcon,
  ExclamationTriangleIcon,
  LockClosedIcon,
  NoSymbolIcon,
} from "@heroicons/react/24/outline";
import type { Turno } from "./conversacion";
import { etiquetaHerramienta } from "./herramientas";
import TextoConCitas from "./TextoConCitas";

/** Turno del asistente: rastro de herramientas y luego respuesta, aviso o fallo. Mismo DOM en vivo o en buffer. */
export default function TurnoAgente({ turno }: { turno: Turno }) {
  return (
    <div className="turno-agente">
      <Pasos
        herramientas={turno.herramientas}
        vivo={turno.estado === "pendiente"}
      />
      {turno.estado === "error" && <Fallo texto={turno.texto} />}
      {turno.estado === "listo" && <Respuesta turno={turno} />}
    </div>
  );
}

function Pasos({
  herramientas,
  vivo,
}: {
  herramientas: string[];
  vivo: boolean;
}) {
  const etiquetas = herramientas.map(etiquetaHerramienta);
  if (vivo && etiquetas.length === 0) etiquetas.push("Leyendo tu pregunta…");
  if (etiquetas.length === 0) return null;
  return (
    <ol className="agente-pasos" aria-label="Pasos del asistente">
      {etiquetas.map((etiqueta, i) => {
        const activo = vivo && i === etiquetas.length - 1;
        return (
          <li
            key={i}
            className={
              activo ? "agente-paso agente-paso--activo" : "agente-paso"
            }
            aria-current={activo ? "step" : undefined}
          >
            {activo ? (
              <span className="agente-paso-pulso" aria-hidden="true" />
            ) : (
              <CheckIcon className="agente-paso-icono" aria-hidden="true" />
            )}
            {etiqueta}
          </li>
        );
      })}
    </ol>
  );
}

function Respuesta({ turno }: { turno: Turno }) {
  if (turno.restringida) {
    return (
      <div className="agente-aviso agente-aviso--restringida" role="note">
        <LockClosedIcon className="agente-aviso-icono" aria-hidden="true" />
        <div>
          <strong>Fuera del alcance de tu rol</strong>
          <p>{turno.texto}</p>
        </div>
      </div>
    );
  }
  return (
    <>
      {turno.parcial && (
        <p className="agente-aviso agente-aviso--parcial" role="note">
          <ExclamationTriangleIcon
            className="agente-aviso-icono"
            aria-hidden="true"
          />
          <span>
            <strong>Respuesta parcial.</strong> El asistente se detuvo antes de
            verificarla: no la tomes como confirmada.
          </span>
        </p>
      )}
      <p
        className={
          turno.parcial
            ? "agente-respuesta agente-respuesta--parcial"
            : "agente-respuesta"
        }
      >
        <TextoConCitas texto={turno.texto} />
      </p>
    </>
  );
}

// El texto llega curado (evento error o mensaje JSON de la API); vacio, se dice igual que no hay asistente.
function Fallo({ texto }: { texto: string }) {
  return (
    <p className="agente-aviso agente-aviso--fallo" role="alert">
      <NoSymbolIcon className="agente-aviso-icono" aria-hidden="true" />
      <span>{texto.trim() || "El asistente no está disponible."}</span>
    </p>
  );
}
