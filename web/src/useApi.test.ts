import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useApi } from "./useApi";

describe("useApi", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("empieza cargando y termina con los datos de la respuesta", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ estado: "listo" }), {
        headers: { "content-type": "application/json" },
      }),
    );

    const { result } = renderHook(() =>
      useApi<{ estado: string }>("/api/ready"),
    );

    expect(result.current.cargando).toBe(true);

    await waitFor(() => expect(result.current.cargando).toBe(false));
    expect(result.current.datos).toEqual({ estado: "listo" });
    expect(result.current.error).toBeNull();
  });

  it("termina con un error tipado si la API responde con un estado de error", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "no autorizado" }), {
        status: 403,
      }),
    );

    const { result } = renderHook(() => useApi("/api/obras"));

    await waitFor(() => expect(result.current.cargando).toBe(false));
    expect(result.current.datos).toBeNull();
    expect(result.current.error?.message).toBe("no autorizado");
  });

  it("un 2xx que no trae JSON queda como error, nunca como datos", async () => {
    // `api()` devuelve el `Response` crudo cuando el content-type no es JSON.
    // Entregarlo como `datos` es lo que tumbaba al listado de ingesta: quien
    // pidio `Carga[]` recibia un `Response` y reventaba al pedirle `.length` o
    // `.map`. El hook es la unica frontera que puede notar la diferencia.
    vi.mocked(fetch).mockResolvedValue(
      new Response("<html><body>sin API</body></html>", {
        status: 200,
        headers: { "content-type": "text/html" },
      }),
    );

    const { result } = renderHook(() =>
      useApi<{ estado: string }[]>("/api/reportes"),
    );

    await waitFor(() => expect(result.current.cargando).toBe(false));
    expect(result.current.datos).toBeNull();
    expect(result.current.error?.message).toBe("la respuesta no vino en JSON");
  });

  it("vuelve a pedir los datos cuando cambia el path", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ id: "1" }), {
          headers: { "content-type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ id: "2" }), {
          headers: { "content-type": "application/json" },
        }),
      );

    const { result, rerender } = renderHook(
      ({ path }) => useApi<{ id: string }>(path),
      { initialProps: { path: "/api/obras/1" } },
    );

    await waitFor(() => expect(result.current.datos).toEqual({ id: "1" }));

    act(() => rerender({ path: "/api/obras/2" }));

    await waitFor(() => expect(result.current.datos).toEqual({ id: "2" }));
    expect(fetch).toHaveBeenCalledTimes(2);
  });
});
