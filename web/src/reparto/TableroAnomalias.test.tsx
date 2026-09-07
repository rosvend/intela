import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setToken } from "../api";
import { ProveedorDeSesion } from "../sesion";
import TableroAnomalias from "./TableroAnomalias";
import { Alerta, RUTAS_REPARTO } from "./tipos";

function json(cuerpo: unknown, status = 200) {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const alertas: Alerta[] = [
  {
    id: "al-1",
    tipo: "oni",
    detalle: "Título no identificado",
    periodo: "2025",
    referencia: "uso-9",
  },
  {
    id: "al-2",
    tipo: "oni",
    detalle: "Otra ONI",
    periodo: "2025",
  },
  {
    id: "al-3",
    tipo: "duplicado_archivo",
    detalle: "Mismo SHA-256",
    periodo: "2025",
    referencia: "rep-2",
  },
  {
    id: "al-4",
    tipo: "titular_sin_porcentaje",
    detalle: "tit-ana sin split",
    periodo: "2025",
    resuelta: true,
  },
];

function montar(ruta = "/anomalias?periodo=2025") {
  setToken("tok");
  return render(
    <MemoryRouter initialEntries={[ruta]}>
      <ProveedorDeSesion>
        <Routes>
          <Route path="/anomalias" element={<TableroAnomalias />} />
        </Routes>
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

describe("TableroAnomalias", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("muestra los conteos por tipo del periodo y enlaza a resolucion", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json({
          id: "usr-1",
          email: "admin@redes.co",
          nombre: "Admin Intela",
          rol: "administrador",
          titular_id: "",
        });
      }
      if (path === RUTAS_REPARTO.procesos) {
        return json([
          {
            id: "proc-1",
            circuito: "nacional",
            etapa: "verificacion",
            periodo: "2025",
            revision: 1,
            firmas: [],
          },
        ]);
      }
      if (path === RUTAS_REPARTO.alertas("2025")) {
        return json(alertas);
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() => expect(screen.getByText("2")).toBeTruthy());
    expect(screen.getAllByText("ONI").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Duplicado de archivo").length).toBeGreaterThan(
      0,
    );
    expect(screen.getByText("Título no identificado")).toBeTruthy();
    expect(screen.queryByText("tit-ana sin split")).toBeNull();
    const enlaces = screen.getAllByRole("link", { name: "Ir a resolución" });
    expect(enlaces.length).toBeGreaterThan(0);
    expect(enlaces[0]?.getAttribute("href")).toBe("#bandeja");
    expect(
      screen.getAllByRole("link", { name: "Resolver" }).length,
    ).toBeGreaterThan(0);
  });

  it("sin backend de alertas muestra el vacio", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      if (String(input) === "/api/auth/session") {
        return json({
          id: "usr-1",
          email: "admin@redes.co",
          nombre: "Admin Intela",
          rol: "administrador",
          titular_id: "",
        });
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar("/anomalias");

    await waitFor(() =>
      expect(screen.getAllByText("Sin datos todavía").length).toBeGreaterThan(
        0,
      ),
    );
  });
});
