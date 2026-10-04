import type { components, paths } from "../contrato";

/**
 * Los tipos de red de la identificacion salen del contrato generado
 * (`npm run contrato`), nunca escritos a mano: dos fuentes de verdad para la
 * misma forma es la clase de defecto que el repo ya pago (D-002). Los nombres
 * de campo se quedan en `snake_case`, como los sirve la API.
 *
 * `GET /identificacion/casos` esta en main (#174). La escritura, el estado
 * `descartado` y la `nota` del caso vienen del contrato PROVISIONAL de #175:
 * ver `resolucion.ts`.
 */
export type CasoIdentificacion = components["schemas"]["CasoIdentificacion"];
export type CandidatoIdentificacion =
  components["schemas"]["CandidatoIdentificacion"];
export type SugerenciaIdentificacion =
  components["schemas"]["SugerenciaIdentificacion"];
export type PaginaCasosIdentificacion =
  components["schemas"]["PaginaCasosIdentificacion"];
export type EstadoDeCaso = CasoIdentificacion["estado"];

/** Los parametros de consulta de la cola, tal como los declara el contrato. */
export type FiltrosDeCasos = NonNullable<
  paths["/identificacion/casos"]["get"]["parameters"]["query"]
>;

// Las plantillas de ruta se atan a las claves de `paths`: si el contrato mueve
// una ruta, `tsc` rompe aqui y no con un 404 en pantalla.
const COLA = "/identificacion/casos" satisfies keyof paths;
const RESOLUCION =
  "/identificacion/casos/{id}/resolucion" satisfies keyof paths;

// El orden en que viajan los filtros: el del contrato. Fijo para que la misma
// consulta sea siempre la misma URL.
const ORDEN_DE_FILTROS: readonly (keyof FiltrosDeCasos)[] = [
  "estado",
  "fuente",
  "periodo",
  "limite",
  "desplazamiento",
];

function consulta(filtros: FiltrosDeCasos): string {
  const params = new URLSearchParams();
  for (const clave of ORDEN_DE_FILTROS) {
    const valor = filtros[clave];
    if (valor === undefined || valor === "") continue;
    // "Cero o ausente es la primera pagina": el cero no viaja.
    if (clave === "desplazamiento" && valor === 0) continue;
    params.set(clave, String(valor));
  }
  const texto = params.toString();
  return texto === "" ? "" : `?${texto}`;
}

export const RUTAS_IDENTIFICACION = {
  casos: (filtros: FiltrosDeCasos) => `/api${COLA}${consulta(filtros)}`,
  resolucion: (id: string) =>
    `/api${RESOLUCION.replace("{id}", encodeURIComponent(id))}`,
} as const;

/**
 * Cuantos casos pinta la bandeja de una vez. Siempre desde el principio de la
 * cola: resolver saca casos del filtro `pendiente`, asi que paginar por
 * desplazamiento sobre ella saltaria los que suben a ocupar el hueco.
 */
export const LIMITE_BANDEJA = 25;

/** Filas por pagina de la lista ONI, que si pagina: ahi no se resuelve nada. */
export const LIMITE_LISTA_ONI = 50;

/** El conteo del badge: `pendientes` no depende de la pagina, basta una fila. */
export const RUTA_CONTEO_PENDIENTES = RUTAS_IDENTIFICACION.casos({
  estado: "pendiente",
  limite: 1,
});

/**
 * Etiqueta de cada estado. En femenino, como el Make v9: lo que se lista es la
 * obra no identificada ("Asignada", "Descartada"), no el caso.
 */
export const ETIQUETA_ESTADO: Record<EstadoDeCaso, string> = {
  pendiente: "Pendiente",
  asignado: "Asignada",
  descartado: "Descartada",
};

/** Los estados del contrato, en el orden del filtro de la lista ONI. */
export const ESTADOS_DE_CASO = Object.keys(ETIQUETA_ESTADO) as EstadoDeCaso[];

/** Propuesta vacia: el contrato exige el campo aunque no haya con que sugerir. */
export function sugerenciaNinguna(): SugerenciaIdentificacion {
  return {
    decision: "ninguna",
    obra_id: null,
    titulo: null,
    confianza: 0,
    motivo: "",
    orden: [],
    aceptada: null,
    sello: null,
  };
}

/**
 * `modalidad` llega en minusculas (`tv`, `ott`, `cine`...). Las siglas se
 * muestran como siglas; lo demas, tal cual: no hay traduccion de la casa que no
 * invente.
 */
export function etiquetaDeModalidad(modalidad: string): string {
  return modalidad === "tv" || modalidad === "ott"
    ? modalidad.toUpperCase()
    : modalidad;
}

const FORMATO_DE_PUNTAJE = new Intl.NumberFormat("es-CO", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

/**
 * El puntaje de similitud como decimal es-CO (`0,71`), nunca como porcentaje:
 * es la medida del escalon difuso, no una probabilidad ni una parte de nada, y
 * un "71 %" al lado de una obra se lee como un reparto.
 */
export function formatearPuntaje(puntaje: number): string {
  return FORMATO_DE_PUNTAJE.format(puntaje);
}

/** `ids_fuente` trae una pareja `tipo=valor` por linea (ADR 0018). */
export function idsDeFuente(ids: string): string[] {
  return ids
    .split(/\r?\n/)
    .map((linea) => linea.trim())
    .filter((linea) => linea !== "");
}

function esObjeto(valor: unknown): valor is Record<string, unknown> {
  return typeof valor === "object" && valor !== null;
}

function esTexto(valor: unknown): valor is string {
  return typeof valor === "string";
}

function esTextoONulo(valor: unknown): boolean {
  return valor === null || esTexto(valor);
}

/**
 * Si un valor sin tipar tiene la forma de una candidata: los campos que pinta
 * la bandeja. `puntaje` tiene que ser un numero de [0, 1], que es lo que el
 * contrato declara: un texto ahi pintaria "NaN" junto a una obra, y el
 * puntaje es lo unico que la persona tiene para comparar candidatas.
 */
function esCandidato(valor: unknown): valor is CandidatoIdentificacion {
  if (!esObjeto(valor)) return false;
  const puntaje = valor["puntaje"];
  return (
    esTexto(valor["obra_id"]) &&
    valor["obra_id"] !== "" &&
    esTexto(valor["titulo"]) &&
    Number.isInteger(valor["anio"]) &&
    esTexto(valor["genero"]) &&
    typeof puntaje === "number" &&
    Number.isFinite(puntaje) &&
    puntaje >= 0 &&
    puntaje <= 1 &&
    esTexto(valor["titulo_consultado"])
  );
}

function esSugerencia(valor: unknown): valor is SugerenciaIdentificacion {
  if (!esObjeto(valor)) return false;
  const decision = valor["decision"];
  if (
    decision !== "asignar" &&
    decision !== "descartar" &&
    decision !== "ninguna"
  ) {
    return false;
  }
  const confianza = valor["confianza"];
  const aceptada = valor["aceptada"];
  return (
    typeof confianza === "number" &&
    Number.isFinite(confianza) &&
    confianza >= 0 &&
    confianza <= 1 &&
    esTextoONulo(valor["obra_id"]) &&
    esTextoONulo(valor["titulo"]) &&
    esTexto(valor["motivo"]) &&
    Array.isArray(valor["orden"]) &&
    valor["orden"].every(esTexto) &&
    (aceptada === null || typeof aceptada === "boolean") &&
    (valor["sello"] === null || esTexto(valor["sello"])) &&
    (decision !== "asignar" ||
      (esTexto(valor["obra_id"]) && valor["obra_id"] !== ""))
  );
}

/**
 * Si un valor sin tipar tiene la forma de un `CasoIdentificacion`.
 *
 * Vive aqui, junto al tipo, como `esObra` vive junto a `Obra`. Existe porque
 * `useApi<T>` promete y no comprueba, y un caso a medias revienta al pintarlo.
 * Todo lo que se mira lo pinta la bandeja o la lista ONI:
 *
 * - `id` no vacio: es la clave de la fila y lo que viaja en la resolucion.
 * - `estado`: uno de los tres del contrato. Otro texto pintado tal cual seria un
 *   estado que el sistema no produce.
 * - `nota`, `obra_asignada`, `resuelto_por` y `resuelto_en`: `null` es una
 *   afirmacion del servidor ("pendiente"), no la falta del dato. Un caso que no
 *   los trae -el de un backend sin #175- se rechaza, no se lee como pendiente.
 * - `sugerencia`: la propuesta del rankeador (#53). Ausente no es "ninguna":
 *   un backend que no la manda no se pinta como si no hubiera propuesto nada.
 *   `sello` viaja siempre: `null` es "no hay propuesta verificable", no la
 *   falta del campo.
 */
export function esCaso(valor: unknown): valor is CasoIdentificacion {
  if (!esObjeto(valor)) return false;
  const obra = valor["obra_asignada"];
  const quien = valor["resuelto_por"];
  return (
    esTexto(valor["id"]) &&
    valor["id"] !== "" &&
    esTexto(valor["titulo"]) &&
    esTexto(valor["titulo_original"]) &&
    esTexto(valor["fuente"]) &&
    esTexto(valor["modalidad"]) &&
    esTexto(valor["reporte_id"]) &&
    esTexto(valor["periodo"]) &&
    esTexto(valor["ids_fuente"]) &&
    esTexto(valor["evidencia"]) &&
    ESTADOS_DE_CASO.includes(valor["estado"] as EstadoDeCaso) &&
    Array.isArray(valor["candidatos"]) &&
    valor["candidatos"].every(esCandidato) &&
    (obra === null ||
      (esObjeto(obra) && esTexto(obra["id"]) && esTexto(obra["titulo"]))) &&
    (quien === null ||
      (esObjeto(quien) && esTexto(quien["id"]) && esTexto(quien["nombre"]))) &&
    esTextoONulo(valor["resuelto_en"]) &&
    esTexto(valor["ultima_actualizacion"]) &&
    esTextoONulo(valor["nota"]) &&
    esSugerencia(valor["sugerencia"])
  );
}

/**
 * Una pagina de la cola. Un caso mal formado la deja ilegible ENTERA, a
 * proposito, como en `useLista`: saltarse la fila mala esconderia un caso sin
 * decirlo, y la bandeja es justo donde no se puede perder uno.
 */
export function esPaginaDeCasos(
  valor: unknown,
): valor is PaginaCasosIdentificacion {
  if (!esObjeto(valor)) return false;
  const pendientes = valor["pendientes"];
  return (
    Number.isInteger(pendientes) &&
    (pendientes as number) >= 0 &&
    Array.isArray(valor["casos"]) &&
    valor["casos"].every(esCaso)
  );
}

/** El hecho de la bitacora que deja una asignacion manual (#175). */
export const HECHO_ASIGNADA = "identificacion.asignada";

/** Lo que el historial de una obra pinta de cada asignacion manual. */
export type ResolucionAsentada = {
  usoId: string;
  titulo: string;
  fuente: string;
  periodo: string;
  reporteId: string;
  nota: string;
  actorNombre: string;
};

/**
 * Lee el payload de un asiento `identificacion.asignada`.
 *
 * El payload no tiene esquema en el contrato -`Asiento.payload` es "cualquier
 * JSON"-, asi que sus claves salen de la descripcion del esquema `Asiento`
 * (contrato provisional de #175) y se comprueban una por una. `null` si falta
 * alguna de las que se pintan: quien llama decide como decir que no se pudo
 * leer, sin esconder el asiento.
 */
export function leerResolucionAsentada(
  payload: unknown,
): ResolucionAsentada | null {
  if (!esObjeto(payload)) return null;
  const campos = [
    "uso_id",
    "titulo",
    "fuente",
    "periodo",
    "reporte_id",
    "nota",
    "actor_nombre",
  ] as const;
  if (!campos.every((campo) => esTexto(payload[campo]))) return null;
  return {
    usoId: payload["uso_id"] as string,
    titulo: payload["titulo"] as string,
    fuente: payload["fuente"] as string,
    periodo: payload["periodo"] as string,
    reporteId: payload["reporte_id"] as string,
    nota: payload["nota"] as string,
    actorNombre: payload["actor_nombre"] as string,
  };
}
