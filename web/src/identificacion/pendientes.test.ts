import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  useFijarPendientes,
  usePendientesDeIdentificacion,
} from "./pendientes";
import { RUTA_CONTEO_PENDIENTES } from "./tipos";

function json(cuerpo: unknown, status = 200) {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

describe("usePendientesDeIdentificacion", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("deshabilitado no pide el conteo y no hay pendientes", () => {
    const { result } = renderHook(() => usePendientesDeIdentificacion(false));

    expect(result.current.pendientes).toBeUndefined();
    expect(result.current.contexto.pendientes).toBeUndefined();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("habilitado pide el conteo de pendientes en la ruta del badge", async () => {
    vi.mocked(fetch).mockResolvedValue(json({ pendientes: 5, casos: [] }));

    const { result } = renderHook(() => usePendientesDeIdentificacion(true));

    await waitFor(() => expect(result.current.pendientes).toBe(5));
    expect(result.current.contexto.pendientes).toBe(5);
    expect(vi.mocked(fetch).mock.calls[0][0]).toBe(RUTA_CONTEO_PENDIENTES);
  });

  it("con pendientes: 0 el hook expone 0, no undefined (quien pinta el badge decide no mostrarlo)", async () => {
    vi.mocked(fetch).mockResolvedValue(json({ pendientes: 0, casos: [] }));

    const { result } = renderHook(() => usePendientesDeIdentificacion(true));

    await waitFor(() => expect(result.current.pendientes).toBe(0));
  });

  it("un 404 no deja pendientes (sin badge, no un error visible)", async () => {
    vi.mocked(fetch).mockResolvedValue(json({ error: "no encontrado" }, 404));

    const { result } = renderHook(() => usePendientesDeIdentificacion(true));

    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(result.current.pendientes).toBeUndefined();
  });

  it("un 503 no deja pendientes", async () => {
    vi.mocked(fetch).mockResolvedValue(json({ error: "no disponible" }, 503));

    const { result } = renderHook(() => usePendientesDeIdentificacion(true));

    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(result.current.pendientes).toBeUndefined();
  });

  it("un fallo de red no deja pendientes", async () => {
    vi.mocked(fetch).mockRejectedValue(new TypeError("Failed to fetch"));

    const { result } = renderHook(() => usePendientesDeIdentificacion(true));

    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(result.current.pendientes).toBeUndefined();
  });

  it("una forma invalida (pendientes no es un entero) no deja pendientes", async () => {
    vi.mocked(fetch).mockResolvedValue(json({ pendientes: "cinco" }));

    const { result } = renderHook(() => usePendientesDeIdentificacion(true));

    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(result.current.pendientes).toBeUndefined();
  });

  it("una forma invalida (pendientes negativo) no deja pendientes", async () => {
    vi.mocked(fetch).mockResolvedValue(json({ pendientes: -1 }));

    const { result } = renderHook(() => usePendientesDeIdentificacion(true));

    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(result.current.pendientes).toBeUndefined();
  });

  it("fijarPendientes manda sobre el valor leido del servidor", async () => {
    vi.mocked(fetch).mockResolvedValue(json({ pendientes: 5, casos: [] }));

    const { result } = renderHook(() => usePendientesDeIdentificacion(true));

    await waitFor(() => expect(result.current.pendientes).toBe(5));

    act(() => {
      result.current.contexto.fijarPendientes(0);
    });

    expect(result.current.pendientes).toBe(0);
    expect(result.current.contexto.pendientes).toBe(0);
  });

  it("el contexto es estable: la misma identidad si sus valores no cambian entre renders", async () => {
    vi.mocked(fetch).mockResolvedValue(json({ pendientes: 5, casos: [] }));

    const { result, rerender } = renderHook(() =>
      usePendientesDeIdentificacion(true),
    );

    await waitFor(() => expect(result.current.pendientes).toBe(5));
    const contextoAntes = result.current.contexto;

    rerender();

    expect(result.current.contexto).toBe(contextoAntes);
  });
});

describe("useFijarPendientes", () => {
  it("sin Outlet (sin contexto) es seguro llamarlo: no hace nada", () => {
    const { result } = renderHook(() => useFijarPendientes());

    expect(() => result.current(9)).not.toThrow();
  });
});
