import { Alerta, TipoDeAlerta } from "./tipos";

/** Lo que se dibuja junto al chip en vez de la frase del servidor. */
export type Visual =
  | { forma: "progreso"; valor: number }
  | { forma: "titular"; porcentaje: string | null; ipi: string | null }
  | { forma: "ninguna" };

export type Descripcion = {
  etiqueta: string;
  visual: Visual;
  /** Nombre legible del registro, si la frase lo trae (titulo reportado de una ONI). */
  nombre: string | null;
};

export type Problema = Descripcion & {
  alerta: Alerta;
  bloquea: boolean;
  cerrado: boolean;
};

/** Un registro afectado (obra, archivo o uso) con todos sus problemas. */
export type Afectado = {
  clave: string;
  refTipo: Alerta["ref_tipo"];
  refId: string;
  nombre: string | null;
  problemas: Problema[];
  bloquea: boolean;
  cerrado: boolean;
};

export type Accion = { etiqueta: string; ruta: string };

const ETIQUETA_CORTA: Record<TipoDeAlerta, string> = {
  oni: "Sin obra identificada",
  duplicado_archivo: "Archivo duplicado",
  duplicado_registro: "Uso duplicado",
  titular_sin_porcentaje: "Coautor sin porcentaje",
  reserva_declaracion_incompleta: "Declaración incompleta",
  tipo_obra_sin_mapear: "Sin tipo de obra",
};

const NUMERO = String.raw`(\d+(?:\.\d+)?)`;
// Un %q de Go: comillas dobles con \" y \\ escapados.
const CITADO = String.raw`"((?:[^"\\]|\\.)*)"`;

function citado(texto: string, prefijo: string): string | null {
  const m = new RegExp(prefijo + CITADO).exec(texto);
  if (!m) return null;
  try {
    return JSON.parse(`"${m[1]}"`) as string;
  } catch {
    return m[1];
  }
}

const conComa = (n: string) => n.replace(".", ",");

function sinPrefijo(refTitular: string | undefined): string | null {
  if (!refTitular) return null;
  return refTitular.replace(/^(ipi|titular):/, "") || null;
}

/** Etiqueta corta y visual de una alerta. Solo se lee lo que la frase dice sin ambiguedad. */
export function describirAlerta(alerta: Alerta): Descripcion {
  const texto = alerta.detalle ?? "";
  const base: Descripcion = {
    etiqueta: ETIQUETA_CORTA[alerta.tipo] ?? alerta.tipo,
    visual: { forma: "ninguna" },
    nombre: null,
  };

  switch (alerta.tipo) {
    case "reserva_declaracion_incompleta": {
      if (/no tiene ninguna Declaracion de Obra/i.test(texto)) {
        return {
          ...base,
          etiqueta: "Sin declaración",
          visual: { forma: "progreso", valor: 0 },
        };
      }
      const suma = new RegExp(`suma ${NUMERO}%`).exec(texto);
      if (!suma) return base;
      return {
        ...base,
        etiqueta: `Declaración al ${conComa(suma[1])} %`,
        visual: {
          forma: "progreso",
          valor: Math.min(100, Math.max(0, Number(suma[1]))),
        },
      };
    }
    case "titular_sin_porcentaje": {
      const parte = new RegExp(`declara ${NUMERO}% y no trae IPI`).exec(texto);
      if (parte) {
        return {
          ...base,
          etiqueta: "Parte sin IPI",
          visual: { forma: "titular", porcentaje: parte[1], ipi: null },
        };
      }
      const ipi = /coautor con IPI (\S+) figura/.exec(texto)?.[1];
      return {
        ...base,
        visual: {
          forma: "titular",
          porcentaje: null,
          ipi: ipi ?? sinPrefijo(alerta.ref_titular),
        },
      };
    }
    case "oni":
      return { ...base, nombre: citado(texto, "no reconocio ") };
    default:
      return base;
  }
}

const estaCerrada = (a: Alerta) => Boolean(a.resuelta || a.autocerrada);

function rango(x: { bloquea: boolean; cerrado: boolean }): number {
  if (x.bloquea) return 0;
  return x.cerrado ? 2 : 1;
}

/** Una tarjeta por registro afectado; lo que bloquea primero, lo cerrado al final. */
export function agruparPorAfectado(alertas: readonly Alerta[]): Afectado[] {
  const grupos = new Map<string, Afectado>();
  for (const alerta of alertas) {
    const clave = `${alerta.ref_tipo}:${alerta.ref_id}`;
    const cerrado = estaCerrada(alerta);
    const problema: Problema = {
      ...describirAlerta(alerta),
      alerta,
      cerrado,
      bloquea: alerta.critica && !cerrado,
    };
    let grupo = grupos.get(clave);
    if (!grupo) {
      grupo = {
        clave,
        refTipo: alerta.ref_tipo,
        refId: alerta.ref_id,
        nombre: null,
        problemas: [],
        bloquea: false,
        cerrado: true,
      };
      grupos.set(clave, grupo);
    }
    grupo.problemas.push(problema);
    grupo.nombre ??= problema.nombre;
    grupo.bloquea ||= problema.bloquea;
    grupo.cerrado &&= cerrado;
  }
  const ordenados = [...grupos.values()];
  for (const g of ordenados) g.problemas.sort((a, b) => rango(a) - rango(b));
  // `sort` es estable: dentro de cada rango se respeta el orden del servidor.
  return ordenados.sort((a, b) => rango(a) - rango(b));
}

/** El unico arreglo que ofrece la tarjeta, decidido por su problema mas grave. */
export function accionDe(afectado: Afectado): Accion | null {
  const primero = afectado.problemas.find((p) => !p.cerrado);
  if (!primero) return null;
  switch (primero.alerta.tipo) {
    case "oni":
      return { etiqueta: "Identificar", ruta: "/identificacion" };
    case "reserva_declaracion_incompleta":
    case "titular_sin_porcentaje":
      return afectado.refTipo === "obra"
        ? {
            etiqueta: "Abrir declaración",
            ruta: `/catalogo/${encodeURIComponent(afectado.refId)}/declaracion`,
          }
        : null;
    default:
      return { etiqueta: "Revisar en Ingesta", ruta: "/ingesta" };
  }
}
