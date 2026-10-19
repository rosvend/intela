// Herramientas del catalogo del agente (spec 2026-08-30, seccion 2); una nueva sin fila cae al fallback.
const ETIQUETAS: Record<string, string> = {
  buscar_reglamento: "Buscando en el Reglamento…",
  explicar_cifra: "Rastreando el origen de la cifra…",
  buscar_obra: "Buscando la obra en el catálogo…",
  estado_declaracion: "Revisando la declaración de obra…",
  listar_oni: "Revisando las obras no identificadas…",
  estado_corrida: "Consultando el estado de la corrida…",
};

export function etiquetaHerramienta(nombre: string): string {
  if (Object.hasOwn(ETIQUETAS, nombre)) return ETIQUETAS[nombre];
  const legible = nombre.replaceAll("_", " ").trim();
  return legible ? `Consultando: ${legible}…` : "Consultando una fuente…";
}
