import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import HistoriaObra from "./HistoriaObra";
import type { Asiento } from "./tipos";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// Historia de una obra con un cambio de metadatos, un cambio de reparto y
// una distribucion, en orden de cadena: el caso de integracion que pide el
// task.
const HISTORIA: Asiento[] = [
  {
    id: "a-1",
    hecho: "obra.registrada",
    ref_tipo: "obra",
    ref_id: "obra-9",
    actor: "usr-admin",
    payload: { titulo: "Dos Palmas", genero: "Drama" },
    cuando: "2026-01-05T09:00:00Z",
  },
  {
    id: "a-2",
    hecho: "declaracion.guardada",
    ref_tipo: "obra",
    ref_id: "obra-9",
    actor: "usr-admin",
    payload: {
      version: 1,
      estado: "completa",
      partes: [
        { TitularID: "t1", IPI: "IPI-00000001", Porcentaje: 60 },
        { TitularID: "t2", IPI: "IPI-00000002", Porcentaje: 40 },
      ],
    },
    cuando: "2026-02-06T09:00:00Z",
  },
  {
    id: "a-3",
    hecho: "declaracion.guardada",
    ref_tipo: "obra",
    ref_id: "obra-9",
    actor: "usr-admin",
    payload: {
      version: 2,
      estado: "completa",
      partes: [
        { TitularID: "t1", IPI: "IPI-00000001", Porcentaje: 50 },
        { TitularID: "t3", IPI: "IPI-00000003", Porcentaje: 50 },
      ],
    },
    cuando: "2026-03-07T09:00:00Z",
  },
  {
    id: "a-4",
    hecho: "reparto.corrida_cerrada",
    ref_tipo: "proceso",
    ref_id: "2026-01",
    actor: "",
    payload: {
      fuente: "Caracol",
      reporte: "rep-abc123",
      regla: "RD 13.5.2",
    },
    cuando: "2026-04-08T09:00:00Z",
  },
];

function montar() {
  render(
    <MemoryRouter initialEntries={["/auditoria/obra/obra-9"]}>
      <Routes>
        <Route path="/auditoria/obra/:id" element={<HistoriaObra />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("historia de una obra", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("pide la historia de la obra de la ruta y muestra los cuatro en orden", async () => {
    vi.mocked(fetch).mockImplementation((url) => {
      if (String(url).includes("/api/auditoria/obra/obra-9")) {
        return Promise.resolve(json(HISTORIA));
      }
      return Promise.resolve(json({ error: "ruta no encontrada" }, 404));
    });
    montar();

    const elementos = await screen.findAllByRole("listitem");
    expect(elementos).toHaveLength(4);
    expect(elementos[0]?.textContent).toContain("obra.registrada");
    expect(elementos[3]?.textContent).toContain("reparto.corrida_cerrada");

    const llamadas = vi.mocked(fetch).mock.calls.map((args) => String(args[0]));
    expect(
      llamadas.some((url) => url.includes("/api/auditoria/obra/obra-9")),
    ).toBe(true);
  });

  it("la segunda version muestra el reparto anterior y el nuevo", async () => {
    vi.mocked(fetch).mockResolvedValue(json(HISTORIA));
    montar();

    const elementos = await screen.findAllByRole("listitem");
    fireEvent.click(elementos[2]?.querySelector("summary") ?? elementos[2]!);

    const texto = elementos[2]?.textContent ?? "";
    expect(texto).toContain("Contra la versión anterior");
    expect(texto).toContain("t2");
    expect(texto).toContain("t3");
  });

  it("la distribucion enlaza su fuente, reporte y regla", async () => {
    vi.mocked(fetch).mockResolvedValue(json(HISTORIA));
    montar();

    const elementos = await screen.findAllByRole("listitem");
    fireEvent.click(elementos[3]?.querySelector("summary") ?? elementos[3]!);

    const texto = elementos[3]?.textContent ?? "";
    expect(texto).toContain("Caracol");
    expect(texto).toContain("rep-abc123");
    expect(texto).toContain("RD 13.5.2");
  });

  it("sin asientos lo dice y no pinta linea vacia", async () => {
    vi.mocked(fetch).mockResolvedValue(json([]));
    montar();

    expect(await screen.findByText(/no tiene asientos/i)).not.toBeNull();
    expect(screen.queryByRole("list")).toBeNull();
  });

  it("vuelve a la linea global", async () => {
    vi.mocked(fetch).mockResolvedValue(json(HISTORIA));
    montar();
    await screen.findAllByRole("listitem");

    expect(
      screen.getByRole("link", { name: /auditoría/i }).getAttribute("href"),
    ).toBe("/auditoria");
  });
});
