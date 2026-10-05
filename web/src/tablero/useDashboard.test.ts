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

  it("un 2xx que no trae JSON queda en error, nunca en listo", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response("<html>sin API</html>", {
        status: 200,
        headers: { "content-type": "text/html" },
      }),
    );

    const { result } = renderHook(() => useRecurso("/api/x"));

    await waitFor(() => expect(result.current.tipo).toBe("error"));
    expect(result.current).toEqual({
      tipo: "error",
      mensaje: "la respuesta no vino en JSON",
    });
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

  it("un 403 termina en error, no en ausente", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json({ error: "el reglamento no te deja ver esto" }, 403),
    );

    const { result } = renderHook(() => useRecurso("/api/x"));

    await waitFor(() => expect(result.current.tipo).toBe("error"));
    expect(result.current).toEqual({
      tipo: "error",
      mensaje: "el reglamento no te deja ver esto",
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

  it("una recarga del mismo path no pinta Cargando otra vez", async () => {
    vi.mocked(fetch).mockResolvedValue(json({ total: 1 }));
    const { result, rerender } = renderHook(
      ({ recarga }: { recarga: number }) => useRecurso("/api/x", true, recarga),
      { initialProps: { recarga: 0 } },
    );

    await waitFor(() => expect(result.current.tipo).toBe("listo"));

    vi.mocked(fetch).mockResolvedValue(json({ total: 2 }));
    rerender({ recarga: 1 });

    expect(result.current).toEqual({ tipo: "listo", datos: { total: 1 } });
    await waitFor(() =>
      expect(result.current).toEqual({ tipo: "listo", datos: { total: 2 } }),
    );
  });

  it("una recarga no borra el vacio: sin backend el sondeo no parpadea", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json({ error: "ruta no encontrada" }, 404),
    );
    const { result, rerender } = renderHook(
      ({ recarga }: { recarga: number }) => useRecurso("/api/x", true, recarga),
      { initialProps: { recarga: 0 } },
    );

    await waitFor(() => expect(result.current.tipo).toBe("ausente"));

    rerender({ recarga: 1 });

    expect(result.current.tipo).toBe("ausente");
  });

  it("una recarga no borra el error: el role=alert no se re-anuncia cada ciclo", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json({ error: "la base esta caida" }, 500),
    );
    const { result, rerender } = renderHook(
      ({ recarga }: { recarga: number }) => useRecurso("/api/x", true, recarga),
      { initialProps: { recarga: 0 } },
    );

    await waitFor(() => expect(result.current.tipo).toBe("error"));

    rerender({ recarga: 1 });

    expect(result.current).toEqual({
      tipo: "error",
      mensaje: "la base esta caida",
    });
  });

  it("cambiar de path si pinta la carga, aunque el anterior estuviera resuelto", async () => {
    vi.mocked(fetch).mockResolvedValue(json({ total: 1 }));
    const { result, rerender } = renderHook(
      ({ path }: { path: string }) => useRecurso(path),
      { initialProps: { path: "/api/x" } },
    );

    await waitFor(() => expect(result.current.tipo).toBe("listo"));

    rerender({ path: "/api/y" });

    expect(result.current.tipo).toBe("cargando");
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

  it("el administrador pide procesos, bolsas y la bitacora; el resumen de alertas espera al periodo", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/procesos") {
        return json([
          {
            id: "p1",
            circuito: "nacional",
            etapa: "verificacion",
            periodo: "2025",
            revision: 1,
          },
        ]);
      }
      if (path.startsWith("/api/alertas/resumen")) return json({ abiertas: 3 });
      return json([]);
    });

    const { result } = renderHook(() => useDashboard("administrador"));

    await waitFor(() =>
      expect(result.current.resumenAlertas.tipo).toBe("listo"),
    );
    const paths = vi.mocked(fetch).mock.calls.map(([path]) => String(path));
    expect(paths).toEqual(
      expect.arrayContaining([
        "/api/procesos",
        "/api/bolsas",
        "/api/auditoria/asientos?limite=5",
        "/api/alertas/resumen?periodo=2025",
      ]),
    );
    expect(result.current.periodo).toBe("2025");
  });

  it("contabilidad no pide la bitacora: /auditoria no es suya", async () => {
    vi.mocked(fetch).mockImplementation(async () => json([]));

    const { result } = renderHook(() => useDashboard("contabilidad"));

    await waitFor(() => expect(result.current.procesos.tipo).toBe("listo"));
    const paths = vi.mocked(fetch).mock.calls.map(([path]) => String(path));
    expect(paths.some((p) => p.startsWith("/api/auditoria"))).toBe(false);
    expect(result.current.asientos.tipo).toBe("inactivo");
  });

  it("el titular no pide nada del panel de staff", async () => {
    vi.mocked(fetch).mockImplementation(async () => json({}));

    const { result } = renderHook(() => useDashboard("titular"));

    await waitFor(() => expect(result.current.misObras.tipo).toBe("listo"));
    expect(result.current.procesos.tipo).toBe("inactivo");
    expect(result.current.bolsas.tipo).toBe("inactivo");
    expect(result.current.resumenAlertas.tipo).toBe("inactivo");
  });
});
