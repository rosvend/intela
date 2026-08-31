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
