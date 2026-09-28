import { describe, expect, it } from "vitest";
import CONTRATO from "../../../api/openapi.yaml?raw";
import { ApiError } from "../api";
import { RUTAS } from "../navegacion";
import { MAX_NOTA, causaDelConflicto } from "./resolucion";
import { ESTADOS_DE_CASO } from "./tipos";

/**
 * Lo que este modulo asume de `api/openapi.yaml` y que los tipos generados no
 * pueden decir: un tope, los mensajes de un 409, el orden de un enum y los roles
 * de una ruta. Si #175 cambia su contrato antes de mergear, esto falla aqui y no
 * en pantalla.
 *
 * El YAML entra como texto (`?raw` de Vite, sin `@types/node` para `fs`) y se
 * recorta por bloques: no hay parser de YAML entre las dependencias del front, y
 * para cuatro datos no se agrega uno.
 */

/** Las lineas de un bloque que empieza en `inicio` y acaba donde otra linea vuelve a su sangria. */
function bloque(inicio: string): string {
  const lineas = CONTRATO.split(/\r?\n/);
  const desde = lineas.findIndex((linea) => linea === inicio);
  expect(desde, `no esta en el contrato: ${inicio.trim()}`).toBeGreaterThan(-1);
  const sangria = inicio.length - inicio.trimStart().length;
  const hasta = lineas.findIndex(
    (linea, i) =>
      i > desde &&
      linea.trim() !== "" &&
      linea.length - linea.trimStart().length <= sangria,
  );
  return lineas.slice(desde, hasta === -1 ? undefined : hasta).join("\n");
}

const RUTA_COLA = bloque("  /identificacion/casos:");
const RUTA_RESOLUCION = bloque("  /identificacion/casos/{id}/resolucion:");

describe("contrato de identificacion (api/openapi.yaml)", () => {
  it("la nota tiene el tope que el panel cuenta", () => {
    const nota = bloque("    ResolucionDeCaso:").split("        nota:")[1];
    expect(nota).toMatch(new RegExp(`maxLength: ${MAX_NOTA}\\b`));
  });

  it("cada ejemplo del 409 cae en su causa", () => {
    const conflicto = RUTA_RESOLUCION.split('"409":')[1].split('"503":')[0];
    const ejemplos = [...conflicto.matchAll(/error: '(.+)'/g)].map((m) => m[1]);
    expect(ejemplos).toHaveLength(3);
    expect(
      ejemplos.map((mensaje) => causaDelConflicto(new ApiError(409, mensaje))),
    ).toEqual(["ya_resuelto", "no_pendiente", "alias_en_conflicto"]);
  });

  it("el filtro de estado es el enum del contrato, en su orden", () => {
    expect(RUTA_COLA).toContain(`enum: [${ESTADOS_DE_CASO.join(", ")}]`);
  });

  it("las dos rutas son solo de administrador, como las dos entradas de la navegacion", () => {
    expect(RUTA_COLA).toContain("x-required-roles: [administrador]");
    expect(RUTA_RESOLUCION).toContain("x-required-roles: [administrador]");
    for (const to of ["/identificacion", "/lista-oni"]) {
      expect(RUTAS.find((r) => r.to === to)?.roles).toEqual(["administrador"]);
    }
  });
});
