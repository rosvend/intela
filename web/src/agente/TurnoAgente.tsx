import {
  CheckIcon,
  ExclamationTriangleIcon,
  LockClosedIcon,
  NoSymbolIcon,
} from "@heroicons/react/24/outline";
import marca from "../marca-intela.png";
import { NO_DISPONIBLE, type Turno } from "./conversacion";
import { etiquetaHerramienta } from "./herramientas";
import TextoConCitas from "./TextoConCitas";

/** Turno del asistente: rastro de herramientas y luego respuesta, aviso o fallo. Mismo DOM en vivo o en buffer. */
export default function TurnoAgente({ turno }: { turno: Turno }) {
  return (
    <div className="turno-agente">
      <Pasos
        herramientas={turno.herramientas}
        estado={turno.estado}
        denegada={turno.restringida === true}
      />
      {turno.estado === "error" && <Fallo texto={turno.texto} />}
      {turno.estado === "listo" && <Respuesta turno={turno} />}
    </div>
  );
}

function Pasos({
  herramientas,
  estado,
  denegada,
}: {
  herramientas: string[];
  estado: Turno["estado"];
  denegada: boolean;
}) {
  const vivo = estado === "pendiente";
  const etiquetas = herramientas.map(etiquetaHerramienta);
  if (vivo && etiquetas.length === 0) return <Pensando />;
  if (etiquetas.length === 0) return null;
  return (
    <ol className="agente-pasos" aria-label="Pasos del asistente">
      {etiquetas.map((etiqueta, i) => {
        const ultimo = i === etiquetas.length - 1;
        const activo = vivo && ultimo;
        // Sin check en la ultima herramienta si el turno fallo (no se sabe terminada) o si
        // fue restringida: el servidor la denego sin ejecutarla (agente.go, ErrNoAutorizado).
        const inconcluso = (estado === "error" || denegada) && ultimo;
        const clase = activo
          ? "agente-paso agente-paso--activo"
          : inconcluso
            ? "agente-paso agente-paso--inconcluso"
            : "agente-paso";
        return (
          <li
            key={i}
            className={clase}
            aria-current={activo ? "step" : undefined}
          >
            {activo && (
              <span className="agente-paso-pulso" aria-hidden="true" />
            )}
            {!activo && !inconcluso && (
              <CheckIcon className="agente-paso-icono" aria-hidden="true" />
            )}
            {etiqueta}
          </li>
        );
      })}
    </ol>
  );
}

/** Mismo spinner de marca que `Cargando`, en linea: el panel es chico y no cabe su bloque centrado. */
function Pensando() {
  return (
    <div className="agente-pensando" role="status">
      <span
        className="logo-spinner agente-pensando-logo"
        style={{ maskImage: `url(${marca})`, WebkitMaskImage: `url(${marca})` }}
        aria-hidden="true"
      />
      Pensando…
    </div>
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
// Sin role="alert": el contenedor aria-live del panel ya lo anuncia y una region dentro de otra lo repetiria.
function Fallo({ texto }: { texto: string }) {
  return (
    <p className="agente-aviso agente-aviso--fallo">
      <NoSymbolIcon className="agente-aviso-icono" aria-hidden="true" />
      <span>{texto.trim() || NO_DISPONIBLE}</span>
    </p>
  );
}
