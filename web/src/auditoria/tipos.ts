import type { components } from "../contrato";

/**
 * Los tipos de red de la auditoria salen del contrato generado
 * (`npm run contrato`), nunca escritos a mano: dos fuentes de verdad para la
 * misma forma es la clase de defecto que el repo ya pago (D-002). Los nombres
 * de campo se quedan en `snake_case`, como los sirve la API.
 */
export type Asiento = components["schemas"]["Asiento"];

export const RUTAS_AUDITORIA = {
  asientos: "/api/auditoria/asientos",
  historialDeObra: (id: string) =>
    `/api/auditoria/obra/${encodeURIComponent(id)}`,
} as const;

/**
 * Si un valor sin tipar tiene la forma de un `Asiento`: los campos que leen
 * estas pantallas, y solo esos.
 *
 * Vive aqui, junto a `Asiento`, como `esObra` vive junto a `Obra`: cada
 * guarda se lee al lado del tipo que comprueba. Existe porque
 * `useApi<Asiento[]>` promete una lista pero no la comprueba, y un elemento a
 * medias reventaria al pintarlo. Sin ErrorBoundary en `web/src`, eso deja la
 * pantalla en blanco.
 *
 * - `payload` tiene que ser un objeto y no `null`: el detalle del asiento lo
 *   recorre como pares clave-valor, y `Object.entries(null)` lanza.
 * - `cuando` tiene que ser texto: se ordena y se formatea como fecha, y un
 *   numero u objeto ahi pintaria basura o reventaria al `Date.parse`.
 */
export function esAsiento(valor: unknown): valor is Asiento {
  if (typeof valor !== "object" || valor === null) return false;
  const v = valor as Record<string, unknown>;
  return (
    typeof v["id"] === "string" &&
    typeof v["hecho"] === "string" &&
    typeof v["ref_tipo"] === "string" &&
    typeof v["ref_id"] === "string" &&
    typeof v["actor"] === "string" &&
    typeof v["payload"] === "object" &&
    v["payload"] !== null &&
    typeof v["cuando"] === "string"
  );
}

/**
 * La familia de un hecho, que es lo que la vista usa para agrupar y filtrar.
 *
 * Se clasifica por PREFIJO del `hecho` (`modulo.evento`) y no por lista
 * cerrada: los modulos publican sus propios asientos (ADR 0006) y manana
 * puede aterrizar `reparto.corrida_cerrada` sin que esta pantalla se entere.
 * Lo desconocido cae en `otro` y se sigue mostrando -un asiento que no se
 * entiende no se esconde-, solo que sin etiqueta de familia.
 */
export type FamiliaHecho =
  | "catalogo"
  | "splits"
  | "distribucion"
  | "recaudo"
  | "identificacion"
  | "otro";

const PREFIJOS_POR_FAMILIA: readonly (readonly [string, FamiliaHecho])[] = [
  ["obra.", "catalogo"],
  ["catalogo.", "catalogo"],
  ["declaracion.", "splits"],
  ["reparto.", "distribucion"],
  ["distribucion.", "distribucion"],
  ["liquidacion.", "distribucion"],
  ["proceso.", "distribucion"],
  ["firma.", "distribucion"],
  ["recaudo.", "recaudo"],
  ["bolsa.", "recaudo"],
  ["matching.", "identificacion"],
  ["identificacion.", "identificacion"],
  ["oni.", "identificacion"],
];

export function familiaDeHecho(hecho: string): FamiliaHecho {
  for (const [prefijo, familia] of PREFIJOS_POR_FAMILIA) {
    if (hecho.startsWith(prefijo)) return familia;
  }
  return "otro";
}

export const ETIQUETA_FAMILIA: Record<FamiliaHecho, string> = {
  catalogo: "Catálogo",
  splits: "Reparto entre autores",
  distribucion: "Distribución",
  recaudo: "Recaudo",
  identificacion: "Identificación",
  otro: "Otro",
};

/**
 * Filtros de la vista. Los cuatro se aplican EN EL CLIENTE sobre la pagina
 * que trajo el servidor: la API no filtra (decision del alcance minimo de
 * este PR). Un campo vacio no filtra.
 */
export type Filtros = {
  familia: FamiliaHecho | "todas";
  desde: string;
  hasta: string;
  actor: string;
  obra: string;
};

export const FILTROS_VACIOS: Filtros = {
  familia: "todas",
  desde: "",
  hasta: "",
  actor: "",
  obra: "",
};

/**
 * Recorta una pagina de asientos segun los filtros. Pura, para probarla sin
 * React.
 *
 * - `familia` compara contra la clasificacion por prefijo, no contra el hecho
 *   crudo: filtrar por "splits" trae `declaracion.guardada` de cualquier
 *   version futura del hecho.
 * - `desde`/`hasta` son fechas `YYYY-MM-DD` (inputs `type="date"`) y se
 *   comparan contra los diez primeros caracteres del `cuando` ISO: comparar
 *   lexicograficamente evita el `Date.parse` y sus zonas horarias, y un
 *   `cuando` mal formado simplemente no cuadra en vez de reventar.
 * - `actor` y `obra` son coincidencia parcial sin distinguir mayusculas;
 *   `obra` cruza contra el `ref_id` solo cuando la referencia es una obra.
 */
export function filtrarAsientos(
  asientos: readonly Asiento[],
  filtros: Filtros,
): Asiento[] {
  const actor = filtros.actor.trim().toLowerCase();
  const obra = filtros.obra.trim().toLowerCase();
  return asientos.filter((asiento) => {
    if (
      filtros.familia !== "todas" &&
      familiaDeHecho(asiento.hecho) !== filtros.familia
    ) {
      return false;
    }
    const dia = asiento.cuando.slice(0, 10);
    if (filtros.desde !== "" && dia < filtros.desde) return false;
    if (filtros.hasta !== "" && dia > filtros.hasta) return false;
    if (actor !== "" && !asiento.actor.toLowerCase().includes(actor)) {
      return false;
    }
    if (
      obra !== "" &&
      !(
        asiento.ref_tipo === "obra" &&
        asiento.ref_id.toLowerCase().includes(obra)
      )
    ) {
      return false;
    }
    return true;
  });
}

/**
 * Una parte tal como la trae el payload de `declaracion.guardada`.
 *
 * Las claves van en PascalCase (`TitularID`, `IPI`, `Porcentaje`) porque el
 * payload es el marshal directo del struct de Go, que no lleva etiquetas
 * json. Se aceptan tambien en `snake_case` por si el productor las normaliza
 * manana: lo que la vista necesita es el dato, no la capitalizacion.
 */
export type ParteDeclarada = {
  titularId: string;
  ipi: string;
  porcentaje: number;
};

function textoDe(valor: unknown): string {
  return typeof valor === "string" ? valor : "";
}

function numeroDe(valor: unknown): number | null {
  return typeof valor === "number" && Number.isFinite(valor) ? valor : null;
}

export function leerPartes(payload: unknown): ParteDeclarada[] | null {
  if (typeof payload !== "object" || payload === null) return null;
  const partes = (payload as Record<string, unknown>)["partes"];
  if (!Array.isArray(partes)) return null;
  const leidas: ParteDeclarada[] = [];
  for (const parte of partes) {
    if (typeof parte !== "object" || parte === null) return null;
    const p = parte as Record<string, unknown>;
    const porcentaje = numeroDe(p["Porcentaje"]) ?? numeroDe(p["porcentaje"]);
    if (porcentaje === null) return null;
    leidas.push({
      titularId: textoDe(p["TitularID"]) || textoDe(p["titular_id"]),
      ipi: textoDe(p["IPI"]) || textoDe(p["ipi"]),
      porcentaje,
    });
  }
  return leidas;
}

/**
 * La diferencia entre dos versiones consecutivas de la declaracion: que parte
 * entro, cual salio y cual cambio de porcentaje.
 *
 * `antes: null` es parte nueva, `despues: null` es parte retirada. Es lo que
 * deja a la historia de una obra decir "el reparto anterior y el nuevo" sin
 * que el auditor reste a mano. Pura, para probarla sin React.
 */
export type CambioDeParte = {
  titularId: string;
  ipi: string;
  antes: number | null;
  despues: number | null;
};

export function diferenciaDePartes(
  anteriores: readonly ParteDeclarada[],
  actuales: readonly ParteDeclarada[],
): CambioDeParte[] {
  const porTitular = new Map<string, CambioDeParte>();
  for (const parte of anteriores) {
    porTitular.set(parte.titularId, {
      titularId: parte.titularId,
      ipi: parte.ipi,
      antes: parte.porcentaje,
      despues: null,
    });
  }
  for (const parte of actuales) {
    const cambio = porTitular.get(parte.titularId);
    if (cambio === undefined) {
      porTitular.set(parte.titularId, {
        titularId: parte.titularId,
        ipi: parte.ipi,
        antes: null,
        despues: parte.porcentaje,
      });
    } else {
      cambio.despues = parte.porcentaje;
    }
  }
  const cambios: CambioDeParte[] = [];
  for (const cambio of porTitular.values()) {
    if (cambio.antes !== cambio.despues) cambios.push(cambio);
  }
  cambios.sort((a, b) => a.titularId.localeCompare(b.titularId));
  return cambios;
}
