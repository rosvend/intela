/** Trozo de la respuesta del agente: texto plano o una cita que se pinta como pildora. */
export type Fragmento =
  | { tipo: "texto"; texto: string }
  | { tipo: "cita"; clase: "reglamento" | "asiento"; valor: string };

// Segmento de una ref de asiento; la ref real es proceso:obra[:titular] (ParsearRef en explicar.go).
const SEG = String.raw`[\w-]+(?:\.[\w-]+)*`;
// Numeral de RD/RT/RS/RA (convencion de CLAUDE.md) o "asiento <ref>" con 2 o 3 segmentos.
const CITA = new RegExp(
  String.raw`\b(?:R[DTSA]\s\d+(?:\.\d+)*)|\b[Aa]siento\s+(${SEG}(?::${SEG}){1,2})(?![\w:])`,
  "g",
);

export function partirCitas(texto: string): Fragmento[] {
  const fragmentos: Fragmento[] = [];
  let desde = 0;
  for (const m of texto.matchAll(CITA)) {
    const inicio = m.index ?? 0;
    if (inicio > desde) {
      fragmentos.push({ tipo: "texto", texto: texto.slice(desde, inicio) });
    }
    fragmentos.push(
      m[1] === undefined
        ? { tipo: "cita", clase: "reglamento", valor: m[0] }
        : { tipo: "cita", clase: "asiento", valor: m[1] },
    );
    desde = inicio + m[0].length;
  }
  if (desde < texto.length) {
    fragmentos.push({ tipo: "texto", texto: texto.slice(desde) });
  }
  return fragmentos;
}
