import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import Liquidaciones from "./Liquidaciones";

vi.mock("./api", () => ({
  api: vi.fn(),
  descargar: vi.fn(),
}));

import { api, descargar } from "./api";

const panel = {
  titular_id: "tit-ana",
  periodo: "2026-01",
  lineas: [
    {
      periodo: "2026-01",
      obra_id: "obra-completa",
      titulo: "La Casa de las Dos Palmas",
      bruto: "6000.00",
      admin: "1200.00",
      social: "600.00",
      reserva: "300.00",
      neto: "3900.00",
    },
  ],
  totales: {
    bruto: "6000.00",
    admin: "1200.00",
    social: "600.00",
    reserva: "300.00",
    neto: "3900.00",
  },
};

describe("Liquidaciones", () => {
  beforeEach(() => {
    vi.mocked(api).mockResolvedValue(panel);
    vi.mocked(descargar).mockResolvedValue(undefined);
  });

  afterEach(() => {
    vi.clearAllMocks();
    cleanup();
  });

  it("muestra bruto, deducciones y neto del panel", async () => {
    render(<Liquidaciones />);
    expect(await screen.findByText("La Casa de las Dos Palmas")).toBeTruthy();
    expect(screen.getAllByText("6000.00").length).toBeGreaterThan(0);
    expect(screen.getAllByText("3900.00").length).toBeGreaterThan(0);
    expect(vi.mocked(api)).toHaveBeenCalledWith("/api/mis-liquidaciones");
  });

  it("filtra por periodo antes de exportar", async () => {
    render(<Liquidaciones />);
    await screen.findByText("La Casa de las Dos Palmas");

    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "2026-01" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Filtrar" }));

    await waitFor(() => {
      expect(vi.mocked(api)).toHaveBeenCalledWith(
        "/api/mis-liquidaciones?periodo=2026-01",
      );
    });
  });

  it("exporta PDF y Excel con el periodo filtrado", async () => {
    render(<Liquidaciones />);
    await screen.findByText("La Casa de las Dos Palmas");
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "2026-01" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Filtrar" }));
    await waitFor(() =>
      expect(vi.mocked(api).mock.calls.length).toBeGreaterThan(1),
    );

    fireEvent.click(screen.getByRole("button", { name: "Exportar PDF" }));
    await waitFor(() => {
      expect(vi.mocked(descargar)).toHaveBeenCalledWith(
        "/api/mis-liquidaciones/export?formato=pdf&periodo=2026-01",
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "Exportar Excel" }));
    await waitFor(() => {
      expect(vi.mocked(descargar)).toHaveBeenCalledWith(
        "/api/mis-liquidaciones/export?formato=xlsx&periodo=2026-01",
      );
    });
  });
});
