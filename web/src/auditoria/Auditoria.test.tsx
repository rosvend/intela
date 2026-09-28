import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Auditoria from "./Auditoria";
import type { Asiento } from "./tipos";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// Bitacora con un asiento por familia, en orden de timeline: lo mas reciente
// primero, como la sirve la API.
const BITACORA: Asiento[] = [
  {
    id: "a-3",
    hecho: "recaudo.registrado",
    ref_tipo: "bolsa",
    ref_id: "bolsa-1",
    actor: "usr-contable",
    payload: {
      periodo: "2026-01",
      circuito: "tv_abierta",
      bruto: "1000000",
      convenio: "conv-1",
      tarifa: "tar-1",
      factura: "fac-1",
    },
    cuando: "2026-03-07T09:00:00Z",
  },
  {
    id: "a-2",
    hecho: "declaracion.guardada",
    ref_tipo: "obra",
    ref_id: "obra-9",
    actor: "usr-admin",
    payload: {
      version: 2,
      estado: "completa",
      partes: [{ TitularID: "t1", IPI: "IPI-00000001", Porcentaje: 100 }],
    },
    cuando: "2026-02-06T09:00:00Z",
  },
  {
    id: "a-1",
    hecho: "obra.registrada",
    ref_tipo: "obra",
    ref_id: "obra-9",
    actor: "usr-admin",
    payload: { titulo: "Dos Palmas" },
    cuando: "2026-01-05T09:00:00Z",
  },
];

function montar() {
  render(
    <MemoryRouter>
      <Auditoria />
    </MemoryRouter>,
  );
}

describe("pantalla de auditoria", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("muestra la linea con los asientos y la nota de inmutabilidad", async () => {
    vi.mocked(fetch).mockResolvedValue(json(BITACORA));
    montar();

    expect(await screen.findAllByRole("listitem")).toHaveLength(3);
    expect(screen.queryByText(/append-only/i)).not.toBeNull();
  });

  it("el filtro por tipo recorta a la familia pedida", async () => {
    vi.mocked(fetch).mockResolvedValue(json(BITACORA));
    montar();
    await screen.findAllByRole("listitem");

    fireEvent.change(screen.getByLabelText("Tipo"), {
      target: { value: "recaudo" },
    });

    const elementos = screen.getAllByRole("listitem");
    expect(elementos).toHaveLength(1);
    expect(elementos[0]?.textContent).toContain("recaudo.registrado");
  });

  it("el filtro por actor recorta a quien actuo", async () => {
    vi.mocked(fetch).mockResolvedValue(json(BITACORA));
    montar();
    await screen.findAllByRole("listitem");

    fireEvent.change(screen.getByLabelText("Actor"), {
      target: { value: "contable" },
    });

    const elementos = screen.getAllByRole("listitem");
    expect(elementos).toHaveLength(1);
    expect(elementos[0]?.textContent).toContain("recaudo.registrado");
  });

  it("el filtro por fecha recorta al rango pedido", async () => {
    vi.mocked(fetch).mockResolvedValue(json(BITACORA));
    montar();
    await screen.findAllByRole("listitem");

    fireEvent.change(screen.getByLabelText("Desde"), {
      target: { value: "2026-03-01" },
    });

    expect(screen.getAllByRole("listitem")).toHaveLength(1);
  });

  it("la evidencia del recaudo muestra la procedencia del dinero", async () => {
    vi.mocked(fetch).mockResolvedValue(json(BITACORA));
    montar();
    await screen.findAllByRole("listitem");

    fireEvent.change(screen.getByLabelText("Tipo"), {
      target: { value: "recaudo" },
    });
    fireEvent.click(screen.getByText("Evidencia de origen"));

    const texto = document.body.textContent ?? "";
    expect(texto).toContain("2026-01");
    expect(texto).toContain("fac-1");
  });

  it("el buscador de historia enlaza a la obra pedida", async () => {
    vi.mocked(fetch).mockResolvedValue(json(BITACORA));
    montar();
    await screen.findAllByRole("listitem");

    fireEvent.change(screen.getByLabelText("Historia de una obra concreta"), {
      target: { value: "obra-9" },
    });

    expect(
      screen.getByRole("link", { name: "Ver historia" }).getAttribute("href"),
    ).toBe("/auditoria/obra/obra-9");
  });

  it("con la bitacora vacia lo dice en vez de pintar una linea vacia", async () => {
    vi.mocked(fetch).mockResolvedValue(json([]));
    montar();

    expect(await screen.findByText(/sin asientos todavía/i)).not.toBeNull();
    expect(screen.queryByRole("list")).toBeNull();
  });

  it("un 500 pinta el error del backend", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json({ error: "la base esta caida" }, 500),
    );
    montar();

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain("la base esta caida");
  });

  it("no expone ningun control de mutacion", async () => {
    vi.mocked(fetch).mockResolvedValue(json(BITACORA));
    montar();
    await screen.findAllByRole("listitem");

    const botones = screen.getAllByRole("button");
    for (const boton of botones) {
      expect(
        /editar|eliminar|borrar|guardar/i.test(boton.textContent ?? ""),
      ).toBe(false);
    }
    expect(
      within(document.body as HTMLElement).queryByRole("link", {
        name: /editar|eliminar|borrar/i,
      }),
    ).toBeNull();
  });
});
