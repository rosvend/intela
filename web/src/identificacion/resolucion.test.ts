import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, ErrorDeRed } from "../api";
import {
  causaDelConflicto,
  clasificarError,
  debeVolverALaLista,
  resolverCaso,
  type ResolucionDeCaso,
} from "./resolucion";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

describe("resolverCaso", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("hace POST a la ruta del caso con la decision, la obra y la nota", async () => {
    vi.mocked(fetch).mockResolvedValue(json({}));
    const cuerpo = {
      decision: "asignar",
      obra_id: "obra-12",
      nota: "coincide la ficha tecnica",
    } satisfies ResolucionDeCaso;

    await resolverCaso("uso-1", cuerpo);

    expect(fetch).toHaveBeenCalledTimes(1);
    const [ruta, init] = vi.mocked(fetch).mock.calls[0];
    expect(ruta).toBe("/api/identificacion/casos/uso-1/resolucion");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual(cuerpo);
  });

  it("un 2xx con el cuerpo ilegible es exito: el servidor ya resolvio", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response("{trunc", {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );

    await expect(
      resolverCaso("uso-1", {
        decision: "descartar",
        nota: "no es del repertorio",
      }),
    ).resolves.toBeUndefined();
  });

  it("un error de la API sube tal cual, con su status", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json({ error: "la nota es obligatoria para resolver un caso" }, 400),
    );

    const error = await resolverCaso("uso-1", {
      decision: "descartar",
      nota: " ",
    }).catch((e: unknown) => e);

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(400);
    expect((error as ApiError).message).toBe(
      "la nota es obligatoria para resolver un caso",
    );
  });
});

describe("causaDelConflicto", () => {
  // Los mensajes de los ejemplos del 409 en api/openapi.yaml (contrato
  // provisional de #175) y el que Go escribe de verdad para el alias, que
  // antepone el uso con `resolver el uso %q: %w`.
  it.each([
    [
      'otra persona ya resolvio este caso: el caso esta en "manual"',
      "ya_resuelto",
    ],
    ['el caso ya no esta pendiente: el caso esta en "difuso"', "no_pendiente"],
    [
      'ese identificador ya apunta a otra obra: caracol id_ficha=871732 ya apunta a "obra-40"',
      "alias_en_conflicto",
    ],
    [
      'resolver el uso "uso-1": ese identificador ya apunta a otra obra: canal-prueba id_ficha=1 ya apunta a "obra-40"',
      "alias_en_conflicto",
    ],
  ] as const)("%s -> %s", (mensaje, causa) => {
    expect(causaDelConflicto(new ApiError(409, mensaje))).toBe(causa);
  });

  it("un 409 que no se reconoce es un conflicto desconocido, no un error comun", () => {
    expect(causaDelConflicto(new ApiError(409, "otra cosa"))).toBe(
      "desconocida",
    );
  });

  it("no es un conflicto si no es un 409 de la API", () => {
    expect(
      causaDelConflicto(
        new ApiError(400, "otra persona ya resolvio este caso"),
      ),
    ).toBeNull();
    expect(causaDelConflicto(new ErrorDeRed(new TypeError("x")))).toBeNull();
    expect(causaDelConflicto("409")).toBeNull();
  });
});

describe("clasificarError", () => {
  it("separa los cuatro 409, un 4xx con mensaje, y todo lo demas como conexion", () => {
    expect(
      clasificarError(new ApiError(409, "otra persona ya resolvio este caso")),
    ).toEqual({ tipo: "ya_resuelto" });
    expect(
      clasificarError(new ApiError(409, "el caso ya no esta pendiente: x")),
    ).toEqual({ tipo: "no_pendiente" });
    expect(
      clasificarError(
        new ApiError(409, "ese identificador ya apunta a otra obra"),
      ),
    ).toEqual({
      tipo: "alias",
      mensaje: "ese identificador ya apunta a otra obra",
    });
    expect(clasificarError(new ApiError(409, "raro"))).toEqual({
      tipo: "desconocido",
      mensaje: "raro",
    });
    expect(clasificarError(new ApiError(400, "nota vacia"))).toEqual({
      tipo: "api",
      mensaje: "nota vacia",
    });
    expect(clasificarError(new ApiError(503, "caido"))).toEqual({
      tipo: "conexion",
    });
    expect(clasificarError(new TypeError("Failed to fetch"))).toEqual({
      tipo: "conexion",
    });
  });
});

describe("debeVolverALaLista", () => {
  it("solo los dos 409 que dicen que el caso salio de la cola lo dejan fuera", () => {
    expect(debeVolverALaLista({ tipo: "ya_resuelto" })).toBe(false);
    expect(debeVolverALaLista({ tipo: "no_pendiente" })).toBe(false);
    expect(debeVolverALaLista({ tipo: "conexion" })).toBe(true);
    expect(debeVolverALaLista({ tipo: "alias", mensaje: "x" })).toBe(true);
  });
});
