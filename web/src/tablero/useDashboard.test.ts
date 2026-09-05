import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useDashboard, useRecurso } from "./useDashboard";
import { RUTAS_TABLERO } from "./tipos";

function json(cuerpo: unknown, status = 200) {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

describe("useRecurso", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("con datos termina en listo", async () => {
    vi.mocked(fetch).mockResolvedValue(json({ total: 4 }));

    const { result } = renderHook(() =>
      useRecurso<{ total: number }>("/api/x"),
    );

    await waitFor(() => expect(result.current.tipo).toBe("listo"));
    expect(result.current).toEqual({ tipo: "listo", datos: { total: 4 } });
  });

  it("un 404 del backend ausente termina en ausente, no en error", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json({ error: "ruta no encontrada" }, 404),
    );

    const { result } = renderHook(() => useRecurso("/api/x"));

    await waitFor(() => expect(result.current.tipo).toBe("ausente"));
  });

  it("un 500 termina en error con el mensaje de la API", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json({ error: "la base esta caida" }, 500),
    );

    const { result } = renderHook(() => useRecurso("/api/x"));

    await waitFor(() => expect(result.current.tipo).toBe("error"));
    expect(result.current).toEqual({
      tipo: "error",
      mensaje: "la base esta caida",
    });
  });

  it("un fallo de red se trata como backend ausente", async () => {
    vi.mocked(fetch).mockRejectedValue(new TypeError("Failed to fetch"));

    const { result } = renderHook(() => useRecurso("/api/x"));

    await waitFor(() => expect(result.current.tipo).toBe("ausente"));
  });

  it("deshabilitado no pega a la red", async () => {
    const { result } = renderHook(() => useRecurso("/api/x", false));

    expect(result.current.tipo).toBe("inactivo");
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe("useDashboard", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("como administrador pide los cuatro conteos y no los de titular", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json({ error: "ruta no encontrada" }, 404),
    );

    const { result } = renderHook(() => useDashboard("administrador"));

    await waitFor(() =>
      expect(result.current.cargasPendientes.tipo).toBe("ausente"),
    );

    const paths = vi.mocked(fetch).mock.calls.map(([path]) => path);
    expect(paths).toEqual(
      expect.arrayContaining([
        RUTAS_TABLERO.cargasPendientes,
        RUTAS_TABLERO.obrasEnReserva,
        RUTAS_TABLERO.oni,
        RUTAS_TABLERO.ultimaCorrida,
      ]),
    );
    expect(paths).not.toContain(RUTAS_TABLERO.misObras);
    expect(paths).not.toContain(RUTAS_TABLERO.ultimaLiquidacion);
    expect(result.current.misObras.tipo).toBe("inactivo");
  });

  it("como titular pide obras y liquidacion, no los KPIs de administrador", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json({ error: "ruta no encontrada" }, 404),
    );

    const { result } = renderHook(() => useDashboard("titular"));

    await waitFor(() => expect(result.current.misObras.tipo).toBe("ausente"));

    const paths = vi.mocked(fetch).mock.calls.map(([path]) => path);
    expect(paths).toEqual(
      expect.arrayContaining([
        RUTAS_TABLERO.misObras,
        RUTAS_TABLERO.ultimaLiquidacion,
      ]),
    );
    expect(paths).not.toContain(RUTAS_TABLERO.oni);
    expect(result.current.oni.tipo).toBe("inactivo");
  });
});
