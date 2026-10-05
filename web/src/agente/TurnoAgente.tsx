import {
  CheckIcon,
  ExclamationTriangleIcon,
  LockClosedIcon,
  NoSymbolIcon,
} from "@heroicons/react/24/outline";
import { etiquetaHerramienta } from "./herramientas";
import TextoConCitas from "./TextoConCitas";
import {
  enCurso,
  type RespuestaAgente,
  type TurnoAgente as Turno,
} from "./turno";

/** Respuesta del asistente en un turno: rastro de herramientas y luego respuesta, aviso o fallo. */
export default function TurnoAgente({ turno }: { turno: Turno }) {
  const vivo = enCurso(turno);
  return (
    <div className="turno-agente" aria-live="polite">
      <Pasos herramientas={turno.herramientas} vivo={vivo} />
      {turno.fallo && <Fallo />}
      {turno.respuesta && <Respuesta respuesta={turno.respuesta} />}
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

function Respuesta({ respuesta }: { respuesta: RespuestaAgente }) {
  if (respuesta.restringida) {
    return (
      <div className="agente-aviso agente-aviso--restringida" role="note">
        <LockClosedIcon className="agente-aviso-icono" aria-hidden="true" />
        <div>
          <strong>Fuera del alcance de tu rol</strong>
          <p>{respuesta.texto}</p>
        </div>
      </div>
    );
  }
  return (
    <>
      {respuesta.parcial && (
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
          respuesta.parcial
            ? "agente-respuesta agente-respuesta--parcial"
            : "agente-respuesta"
        }
      >
        <TextoConCitas texto={respuesta.texto} />
      </p>
    </>
  );
}

function Fallo() {
  return (
    <div className="agente-aviso agente-aviso--fallo" role="alert">
      <NoSymbolIcon className="agente-aviso-icono" aria-hidden="true" />
      <div>
        <strong>El asistente no está disponible.</strong>
        <p>No se obtuvo respuesta. Vuelve a preguntar en unos minutos.</p>
      </div>
    </div>
  );
}
