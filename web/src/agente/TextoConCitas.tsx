import { partirCitas } from "./citas";
import PildoraCita from "./PildoraCita";

/** Texto del agente con cada cita (numeral de reglamento o asiento) convertida en pildora. */
export default function TextoConCitas({ texto }: { texto: string }) {
  return (
    <>
      {partirCitas(texto).map((f, i) =>
        f.tipo === "texto" ? (
          f.texto
        ) : (
          <PildoraCita key={i} clase={f.clase} valor={f.valor} />
        ),
      )}
    </>
  );
}
