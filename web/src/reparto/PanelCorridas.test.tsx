import {
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
