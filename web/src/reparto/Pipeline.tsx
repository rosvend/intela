import type { CSSProperties, ReactNode } from "react";
import { CheckIcon } from "@heroicons/react/20/solid";
import Detalle from "../ui/Detalle";
import { Circuito, Etapa } from "./tipos";
import { ETIQUETA_ETAPA, EstadoDePaso, estadoDePaso, etapasDe } from "./etapas";
import { ETIQUETA_ROL_FIRMA, ROLES_DE_COMPUERTA, esCompuerta } from "./firmas";
import "../staff.css";

/** Que pasa en cada etapa, dicho para quien no lee el reglamento (RD 13.5). */
export const EXPLICACION_ETAPA: Record<Etapa, string> = {
  recaudo:
    "Se confirma cuánto se cobró a cada usuario en el periodo: esa es la bolsa a repartir.",
  deducciones:
    "Se descuentan de la bolsa los gastos de administración y las reservas que fija el reglamento.",
  importe_obra:
    "La bolsa neta se reparte entre las obras según cuánto se usó cada una en los reportes.",
  importe_titular:
    "El importe de cada obra se divide entre sus escritores según la declaración. Si no suma 100%, la obra entera queda en reserva.",
  liquidacion_parcial:
    "Se prepara un borrador de liquidación para revisarlo antes de comprometer el dinero.",
  verificacion:
    "Distribución y Contabilidad revisan las cifras del borrador antes de cerrarlas.",
  liquidacion_final:
    "Las cifras quedan cerradas: ya no cambian sin abrir una nueva revisión.",
  pago_registro:
    "Se autoriza el pago a cada escritor y queda registrado en la bitácora.",
  fees_in_error:
    "Se devuelven o reasignan los importes del exterior que llegaron por error.",
  auditoria:
    "La revisoría fiscal revisa la corrida completa; cada cifra se rastrea hasta su origen.",
};

const ESTADO_LEIDO: Record<EstadoDePaso, string> = {
  hecha: "completada",
  actual: "en curso",
  pendiente: "pendiente",
};

const FIRMANTES = ROLES_DE_COMPUERTA.map((r) => ETIQUETA_ROL_FIRMA[r]).join(
  " y ",
);

/**
 * Stepper horizontal de la corrida. `compacto` (tablero de Inicio) quita los
 * detalles: alli es un resumen, y el control vive en Distribucion.
 */
export default function Pipeline({
  circuito,
  etapa,
  compacto = false,
}: {
  circuito: Circuito;
  etapa: Etapa;
  compacto?: boolean;
}) {
  const pipeline = etapasDe(circuito);
  const actual = Math.max(0, pipeline.indexOf(etapa));
  const progreso = pipeline.length > 1 ? actual / (pipeline.length - 1) : 0;

  return (
    <ol
      className={`stepper${compacto ? " stepper-compacto" : ""}`}
      aria-label="Etapas del proceso"
      style={{ "--progreso": progreso } as CSSProperties}
    >
      {pipeline.map((paso, i) => {
        const estado = estadoDePaso(pipeline, etapa, paso);
        const contenido = <Paso etapa={paso} estado={estado} />;
        return (
          <li
            key={paso}
            className={`stepper-paso stepper-paso-${estado}`}
            aria-current={estado === "actual" ? "step" : undefined}
            style={{ "--i": i } as CSSProperties}
          >
            {compacto ? (
              contenido
            ) : (
              <Detalle
                titulo={ETIQUETA_ETAPA[paso]}
                claseDisparador="stepper-disparador"
                etiquetaDisparador={`${ETIQUETA_ETAPA[paso]}: ${ESTADO_LEIDO[estado]}`}
                disparador={contenido}
              >
                <span className="detalle-texto">{EXPLICACION_ETAPA[paso]}</span>
                <span className="detalle-texto stepper-firmas">
                  {esCompuerta(paso)
                    ? `Firman: ${FIRMANTES}`
                    : "No necesita firmas."}
                </span>
              </Detalle>
            )}
          </li>
        );
      })}
    </ol>
  );
}

function Paso({ etapa, estado }: { etapa: Etapa; estado: EstadoDePaso }) {
  let marca: ReactNode = null;
  if (estado === "hecha") marca = <CheckIcon className="stepper-check" />;
  return (
    <>
      <span className="stepper-punto" aria-hidden="true">
        {marca}
      </span>
      <span className="stepper-etiqueta">{ETIQUETA_ETAPA[etapa]}</span>
    </>
  );
}
