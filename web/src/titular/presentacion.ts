// Traducciones de datos del backend a lenguaje de escritor. Sin aritmetica en float sobre dinero.

const MESES = [
  "enero",
  "febrero",
  "marzo",
  "abril",
  "mayo",
  "junio",
  "julio",
  "agosto",
  "septiembre",
  "octubre",
  "noviembre",
  "diciembre",
];

/** "2025-01" -> "enero 2025"; "2025-1" -> "1.er semestre 2025"; "2025" -> "año 2025". */
export function nombrePeriodo(periodo: string): string {
  const p = periodo.trim();
  let m = /^(\d{4})-(\d{2})$/.exec(p);
  if (m) {
    const mes = MESES[Number(m[2]) - 1];
    return mes ? `${mes} ${m[1]}` : p;
  }
  m = /^(\d{4})-([12])$/.exec(p);
  if (m) return `${m[2] === "1" ? "1.er" : "2.º"} semestre ${m[1]}`;
  if (/^\d{4}$/.test(p)) return `año ${p}`;
  return p;
}

const FUENTES: Record<string, string> = {
  caracol: "Caracol Televisión",
  rcn: "RCN Televisión",
  cine: "Salas de cine",
  netflix: "Netflix",
};

export function nombreFuente(fuente: string): string {
  const f = fuente.trim();
  if (FUENTES[f]) return FUENTES[f];
  const llano = f.replace(/[-_]+/g, " ");
  return llano.charAt(0).toUpperCase() + llano.slice(1);
}

/** "caracol, rcn" -> "Caracol Televisión y RCN Televisión". */
export function nombreFuentes(fuentes: string): string {
  const nombres = fuentes
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean)
    .map(nombreFuente);
  if (nombres.length <= 1) return nombres[0] ?? "";
  return `${nombres.slice(0, -1).join(", ")} y ${nombres[nombres.length - 1]}`;
}

export function primerNombre(nombre: string): string {
  return nombre.trim().split(/\s+/)[0] ?? "";
}

export type EstadoObra = {
  tipo: "lista" | "reserva" | "otro";
  etiqueta: string;
};

/** RD 13.1.3: sin el 100% declarado la obra entera queda en reserva. */
export function estadoDeObra(estado: string): EstadoObra {
  const e = estado.toLowerCase();
  if (e.includes("incompleta") || e.includes("reserva")) {
    return {
      tipo: "reserva",
      etiqueta: "En reserva: falta completar la declaración",
    };
  }
  if (e === "completa") return { tipo: "lista", etiqueta: "Lista para pagar" };
  return { tipo: "otro", etiqueta: estado };
}

function aCentavos(importe: string): bigint {
  const m = /^(-)?(\d+)(?:\.(\d+))?$/.exec(importe.trim());
  if (!m) return 0n;
  const cent =
    BigInt(m[2]) * 100n + BigInt((m[3] ?? "").padEnd(2, "0").slice(0, 2));
  return m[1] ? -cent : cent;
}

function deCentavos(c: bigint): string {
  const neg = c < 0n;
  const abs = neg ? -c : c;
  return `${neg ? "-" : ""}${abs / 100n}.${(abs % 100n).toString().padStart(2, "0")}`;
}

/** Suma exacta de importes decimales en centavos (BigInt). */
export function sumarImportes(importes: string[]): string {
  return deCentavos(importes.reduce((t, v) => t + aCentavos(v), 0n));
}

export type NetoDeObra = { obra_id: string; titulo: string; neto: string };

export function agruparPorObra(lineas: NetoDeObra[]): NetoDeObra[] {
  const grupos = new Map<string, NetoDeObra>();
  for (const l of lineas) {
    const previo = grupos.get(l.obra_id);
    grupos.set(
      l.obra_id,
      previo
        ? { ...previo, neto: sumarImportes([previo.neto, l.neto]) }
        : { obra_id: l.obra_id, titulo: l.titulo, neto: l.neto },
    );
  }
  return [...grupos.values()];
}

/** "1.3" -> "1,3"; "5616" -> "5.616". Opera sobre el texto: no pierde digitos. */
export function numeroLlano(valor: string): string {
  const m = /^(-?)(\d+)(?:\.(\d+))?$/.exec(valor.trim());
  if (!m) return valor;
  const entero = m[2].replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  const frac = (m[3] ?? "").replace(/0+$/, "");
  return `${m[1]}${entero}${frac ? `,${frac}` : ""}`;
}

/** "60.0000" -> "60 %"; a lo sumo dos decimales, solo para leer. */
export function porcentajeLlano(valor: string): string {
  const m = /^(-?\d+)(?:\.(\d+))?$/.exec(valor.trim());
  if (!m) return `${valor} %`;
  const frac = (m[2] ?? "").slice(0, 2).replace(/0+$/, "");
  return `${numeroLlano(m[1])}${frac ? `,${frac}` : ""} %`;
}

const FECHA_LARGA = new Intl.DateTimeFormat("es-CO", {
  day: "numeric",
  month: "long",
  year: "numeric",
  timeZone: "UTC",
});

export function fechaLlana(iso: string): string {
  const f = new Date(iso);
  return Number.isNaN(f.getTime()) ? iso : FECHA_LARGA.format(f);
}
