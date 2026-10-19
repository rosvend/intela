import type { ReactNode } from "react";
import { partirCitas } from "./citas";
import PildoraCita from "./PildoraCita";

// Subconjunto de markdown que usa el modelo. Nunca HTML crudo ni enlaces: todo sale como nodos React.
const ENFASIS = /\*\*(\S(?:[^*]*\S)?)\*\*|\*(\S(?:[^*]*\S)?)\*|`([^`]+)`/g;
const ITEM = /^\s*(?:([-*])|\d+[.)])\s+(.*)$/;

/** Texto plano con cada cita (numeral de reglamento o asiento) convertida en pildora. */
function conCitas(texto: string): ReactNode[] {
  return partirCitas(texto).map((f, i) =>
    f.tipo === "texto" ? (
      f.texto
    ) : (
      <PildoraCita
        key={i}
        clase={f.clase}
        valor={f.valor}
        palabra={f.clase === "asiento" ? f.palabra : undefined}
      />
    ),
  );
}

function enLinea(texto: string): ReactNode[] {
  const nodos: ReactNode[] = [];
  let desde = 0;
  for (const m of texto.matchAll(ENFASIS)) {
    const inicio = m.index ?? 0;
    nodos.push(...conCitas(texto.slice(desde, inicio)));
    if (m[1] !== undefined) {
      nodos.push(<strong key={inicio}>{enLinea(m[1])}</strong>);
    } else if (m[2] !== undefined) {
      nodos.push(<em key={inicio}>{enLinea(m[2])}</em>);
    } else {
      nodos.push(<code key={inicio}>{conCitas(m[3])}</code>);
    }
    desde = inicio + m[0].length;
  }
  nodos.push(...conCitas(texto.slice(desde)));
  return nodos;
}

type Bloque =
  { tipo: "p"; lineas: string[] } | { tipo: "ul" | "ol"; items: string[] };

function partirBloques(texto: string): Bloque[] {
  const bloques: Bloque[] = [];
  let abierto = false; // el ultimo bloque admite mas lineas (no hubo linea en blanco)
  for (const linea of texto.split("\n")) {
    const ultimo = bloques.at(-1);
    const item = ITEM.exec(linea);
    if (linea.trim() === "") {
      abierto = false;
    } else if (item) {
      const tipo = item[1] ? "ul" : "ol";
      if (abierto && ultimo?.tipo === tipo) ultimo.items.push(item[2]);
      else bloques.push({ tipo, items: [item[2]] });
      abierto = true;
    } else if (abierto && ultimo?.tipo === "p") {
      ultimo.lineas.push(linea);
    } else {
      bloques.push({ tipo: "p", lineas: [linea] });
      abierto = true;
    }
  }
  return bloques;
}

/** Respuesta del agente: markdown acotado y citas como pildoras. */
export default function TextoConCitas({ texto }: { texto: string }) {
  return (
    <>
      {partirBloques(texto).map((b, i) => {
        if (b.tipo === "p") {
          return <p key={i}>{enLinea(b.lineas.join("\n"))}</p>;
        }
        const Lista = b.tipo;
        return (
          <Lista key={i}>
            {b.items.map((it, j) => (
              <li key={j}>{enLinea(it)}</li>
            ))}
          </Lista>
        );
      })}
    </>
  );
}
