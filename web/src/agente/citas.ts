/** Trozo de la respuesta del agente: texto plano o una cita que se pinta como pildora. */
export type Fragmento =
  | { tipo: "texto"; texto: string }
  | { tipo: "cita"; clase: "reglamento"; valor: string }
  | { tipo: "cita"; clase: "asiento"; valor: string; palabra: string };

// Segmento de una ref de asiento. Las refs que acepta ExplicarCifra (explicar.go) son
// proceso:obra[:titular] (ParsearRef) y las accesorias reserva|rendimiento:origen:destino:obra:titular
// (parsearRefAccesoria, bolsas_asientos.go).
const SEG = String.raw`[\w-]+(?:\.[\w-]+)*`;
const REF = String.raw`(?:(?:reserva|rendimiento)(?::${SEG}){4}|${SEG}(?::${SEG}){1,2})`;
// Numeral de RD/RT/RS/RA (convencion de CLAUDE.md) o "asiento <ref>". La ref solo termina en fin
// de texto, espacio o puntuacion de cierre: si sigue cualquier otra cosa (letra con tilde, -, :, /)
// la ref seria otra, y se deja como texto antes que copiar una ref cortada.
const CITA = new RegExp(
  String.raw`\b(?:R[DTSA]\s\d+(?:\.\d+)*)|\b([Aa]siento)\s+(${REF})(?=$|[\s.,;!?)\]}"'»…])`,
  "gu",
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
      m[2] === undefined
        ? { tipo: "cita", clase: "reglamento", valor: m[0] }
        : { tipo: "cita", clase: "asiento", valor: m[2], palabra: m[1] },
    );
    desde = inicio + m[0].length;
  }
  if (desde < texto.length) {
    fragmentos.push({ tipo: "texto", texto: texto.slice(desde) });
  }
  return fragmentos;
}
