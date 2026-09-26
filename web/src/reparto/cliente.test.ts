import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { firmarProceso } from "./cliente";
import { RUTAS_REPARTO } from "./tipos";

describe("cliente de procesos", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    localStorage.setItem("intela.token", "tok");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("POST /procesos/{id}/firmar con accion firmar", async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(null, { status: 204 }));

    await firmarProceso("proc-1", { accion: "firmar" });

    expect(fetch).toHaveBeenCalledWith(
      RUTAS_REPARTO.firmar("proc-1"),
      expect.objectContaining({ method: "POST" }),
    );
    const [, init] = vi.mocked(fetch).mock.calls[0];
    expect(init?.body).toBe(JSON.stringify({ accion: "firmar" }));
  });

  it("POST /procesos/{id}/firmar con accion rechazar y motivo", async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(null, { status: 204 }));

    await firmarProceso("proc-1", {
      accion: "rechazar",
      motivo: "cifras no cuadran",
    });

    const [, init] = vi.mocked(fetch).mock.calls[0];
    expect(init?.body).toBe(
      JSON.stringify({
        accion: "rechazar",
        motivo: "cifras no cuadran",
      }),
    );
  });

  it("las rutas de lectura cuadran con el contrato de #34 y #37", () => {
    expect(RUTAS_REPARTO.procesos).toBe("/api/procesos");
    expect(RUTAS_REPARTO.proceso("p/1")).toBe("/api/procesos/p%2F1");
    expect(RUTAS_REPARTO.alertas("2025")).toBe("/api/alertas?periodo=2025");
    expect(RUTAS_REPARTO.alertas()).toBe("/api/alertas");
  });
});
