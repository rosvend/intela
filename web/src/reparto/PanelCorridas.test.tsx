import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setToken } from "../api";
import { ProveedorDeSesion, Rol } from "../sesion";
import PanelCorridas from "./PanelCorridas";
import { Proceso, RUTAS_REPARTO } from "./tipos";

function json(cuerpo: unknown, status = 200) {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function usuario(rol: Rol) {
  return {
    id: "usr-1",
    email: "x@redes.co",
    nombre: "Persona de Prueba",
    rol,
    titular_id: "",
  };
}

const nacional: Proceso = {
  id: "proc-nac",
  circuito: "nacional",
  etapa: "verificacion",
  periodo: "2025",
  revision: 1,
  firmas: [],
};

const internacional: Proceso = {
  id: "proc-int",
  circuito: "internacional",
  etapa: "recaudo",
  periodo: "2025-06",
  revision: 1,
  firmas: [],
};

function montar(ruta = "/distribucion") {
  return render(
    <MemoryRouter initialEntries={[ruta]}>
      <ProveedorDeSesion>
        <Routes>
          <Route path="/distribucion" element={<PanelCorridas />} />
          <Route path="/distribucion/:id" element={<PanelCorridas />} />
        </Routes>
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

describe("PanelCorridas", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("lista cada proceso con su etapa actual", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json(usuario("administrador"));
      }
      if (path === RUTAS_REPARTO.procesos) {
        return json([nacional, internacional]);
      }
      if (path.startsWith("/api/alertas")) {
        return json([]);
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() => expect(screen.getByText("2025")).toBeTruthy());
    expect(screen.getByText("2025-06")).toBeTruthy();
    expect(screen.getAllByText("Verificación").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Recaudo").length).toBeGreaterThan(0);
    expect(screen.getByText("Nacional")).toBeTruthy();
    expect(screen.getByText("Internacional")).toBeTruthy();
  });

  it("una compuerta muestra firmados y pendientes", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json(usuario("administrador"));
      }
      if (path === RUTAS_REPARTO.procesos) {
        return json([
          {
            ...nacional,
            firmas: [{ rol: "distribucion", actor_id: "usr-d", sobre_rev: 1 }],
          },
        ]);
      }
      if (path.startsWith("/api/alertas")) return json([]);
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() => expect(screen.getByText("Firmado")).toBeTruthy());
    expect(screen.getByText("Pendiente")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Firmar" })).toBeNull();
  });

  it("firmar como distribucion recarga y la etapa avanza", async () => {
    let etapa: Proceso["etapa"] = "verificacion";
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json(usuario("distribucion"));
      }
      if (
        path === RUTAS_REPARTO.firmar("proc-nac") &&
        init?.method === "POST"
      ) {
        etapa = "liquidacion_final";
        return new Response(null, { status: 204 });
      }
      if (path === RUTAS_REPARTO.procesos) {
        return json([{ ...nacional, etapa }]);
      }
      if (path.startsWith("/api/alertas")) return json([]);
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Firmar" })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Firmar" }));

    await waitFor(() =>
      expect(screen.getAllByText("Liquidación final").length).toBeGreaterThan(
        0,
      ),
    );
    const firma = vi
      .mocked(fetch)
      .mock.calls.find(
        ([url, init]) =>
          String(url) === RUTAS_REPARTO.firmar("proc-nac") &&
          init?.method === "POST",
      );
    expect(firma?.[1]?.body).toBe(JSON.stringify({ accion: "firmar" }));
  });

  it("un 403 al firmar se muestra y no oculta el pipeline", async () => {
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json(usuario("distribucion"));
      }
      if (
        path === RUTAS_REPARTO.firmar("proc-nac") &&
        init?.method === "POST"
      ) {
        return json({ error: "no autorizado" }, 403);
      }
      if (path === RUTAS_REPARTO.procesos) {
        return json([nacional]);
      }
      if (path.startsWith("/api/alertas")) return json([]);
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Firmar" })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Firmar" }));

    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe("no autorizado"),
    );
    expect(screen.getByText("Importe de la obra")).toBeTruthy();
  });

  it("un id inexistente nunca ofrece firmar otra corrida", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("distribucion"));
      if (path === RUTAS_REPARTO.procesos) return json([nacional]);
      return json([]);
    });
    montar("/distribucion/no-existe");
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain("no encontrado"),
    );
    expect(screen.queryByRole("button", { name: "Firmar" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Rechazar" })).toBeNull();
  });

  it("navegar a otra corrida descarta el motivo y actúa sobre el id visible", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("distribucion"));
      if (path === RUTAS_REPARTO.procesos)
        return json([nacional, { ...internacional, etapa: "verificacion" }]);
      return json([]);
    });
    montar("/distribucion/proc-nac");
    await screen.findByRole("button", { name: "Rechazar" });
    fireEvent.click(screen.getByRole("button", { name: "Rechazar" }));
    fireEvent.change(screen.getByLabelText("Motivo del rechazo"), {
      target: { value: "Problema nacional" },
    });
    fireEvent.click(screen.getByRole("link", { name: "2025-06" }));
    expect(screen.queryByLabelText("Motivo del rechazo")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Rechazar" }));
    expect(
      (screen.getByLabelText("Motivo del rechazo") as HTMLTextAreaElement)
        .value,
    ).toBe("");
    fireEvent.change(screen.getByLabelText("Motivo del rechazo"), {
      target: { value: "Problema internacional" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Confirmar rechazo" }));
    await waitFor(() =>
      expect(
        vi.mocked(fetch).mock.calls.some(
          ([url, init]) =>
            String(url) === RUTAS_REPARTO.firmar("proc-int") &&
            init?.body ===
              JSON.stringify({
                accion: "rechazar",
                motivo: "Problema internacional",
              }),
        ),
      ).toBe(true),
    );
    expect(
      vi
        .mocked(fetch)
        .mock.calls.some(
          ([url]) => String(url) === RUTAS_REPARTO.firmar("proc-nac"),
        ),
    ).toBe(false);
  });

  it("una firma tardía no muestra su error en otra corrida", async () => {
    let responder!: (response: Response) => void;
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("distribucion"));
      if (path === RUTAS_REPARTO.procesos)
        return json([nacional, { ...internacional, etapa: "verificacion" }]);
      if (path === RUTAS_REPARTO.firmar("proc-nac"))
        return new Promise<Response>((resolve) => {
          responder = resolve;
        });
      return json([]);
    });
    montar("/distribucion/proc-nac");
    fireEvent.click(await screen.findByRole("button", { name: "Firmar" }));
    fireEvent.click(screen.getByRole("link", { name: "2025-06" }));
    await act(async () => {
      responder(json({ error: "Error nacional" }, 409));
    });
    expect(screen.queryByText("Error nacional")).toBeNull();
    expect(
      (screen.getByRole("button", { name: "Firmar" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
  });

  it("contabilidad pide las alertas y ve el aviso antes de firmar", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("contabilidad"));
      if (path === RUTAS_REPARTO.procesos) return json([nacional]);
      if (path.startsWith("/api/alertas"))
        return json([
          {
            id: "al-1",
            tipo: "oni",
            detalle: "Sin identificar",
            periodo: "2025",
          },
        ]);
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await screen.findByText(
      "Revisa las 1 alertas abiertas del periodo antes de firmar.",
    );
    expect(
      vi
        .mocked(fetch)
        .mock.calls.some(([url]) => String(url).startsWith("/api/alertas")),
    ).toBe(true);
    expect(screen.getByText("Alertas abiertas del periodo")).toBeTruthy();
  });

  it("firmar con alertas abiertas avisa, pero no bloquea la compuerta", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("distribucion"));
      if (path === RUTAS_REPARTO.procesos) return json([nacional]);
      if (path.startsWith("/api/alertas"))
        return json([
          {
            id: "al-1",
            tipo: "oni",
            detalle: "Sin identificar",
            periodo: "2025",
          },
          {
            id: "al-2",
            tipo: "reserva_declaracion_incompleta",
            detalle: "80% declarado",
            periodo: "2025",
          },
        ]);
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await screen.findByText(
      "Revisa las 2 alertas abiertas del periodo antes de firmar.",
    );
    expect(
      (screen.getByRole("button", { name: "Firmar" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
  });

  it("volver a una corrida no revive el error de firma que quedo atras", async () => {
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("distribucion"));
      if (path === RUTAS_REPARTO.firmar("proc-nac") && init?.method === "POST")
        return json({ error: "Error nacional" }, 409);
      if (path === RUTAS_REPARTO.procesos)
        return json([nacional, { ...internacional, etapa: "verificacion" }]);
      return json([]);
    });

    montar("/distribucion/proc-nac");

    fireEvent.click(await screen.findByRole("button", { name: "Firmar" }));
    await waitFor(() =>
      expect(screen.getByText("Error nacional")).toBeTruthy(),
    );

    fireEvent.click(screen.getByRole("link", { name: "2025-06" }));
    expect(screen.queryByText("Error nacional")).toBeNull();

    fireEvent.click(screen.getByRole("link", { name: "2025" }));
    expect(screen.queryByText("Error nacional")).toBeNull();
  });

  it("sin backend de procesos muestra el vacio, no un crash", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      if (String(input) === "/api/auth/session") {
        return json(usuario("administrador"));
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() =>
      expect(screen.getByText(/Sin datos todavía/)).toBeTruthy(),
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
