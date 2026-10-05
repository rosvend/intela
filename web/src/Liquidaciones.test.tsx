import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
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

  it("muestra cada obra con su neto, periodo en palabras y descuentos en pesos", async () => {
    render(<Liquidaciones />);
    const fila = (await screen.findByText("La Casa de las Dos Palmas")).closest(
      "li",
    );
    expect(fila?.textContent).toContain("enero 2026");
    expect(fila?.textContent).toContain("$ 3.900");
    expect(fila?.textContent).toContain("$ 6.000");
    expect(fila?.textContent).toContain("Gastos administrativos $ 1.200");
    expect(fila?.textContent).toContain("Bienestar social $ 600");
    expect(fila?.textContent).toContain("Reserva para errores técnicos $ 300");
    expect(screen.getByLabelText("$ 3.900")).toBeTruthy();
    expect(vi.mocked(api)).toHaveBeenCalledWith("/api/mis-liquidaciones/obras");
  });

  it("no usa clases que no existen", async () => {
    const { container } = render(<Liquidaciones />);
    await screen.findByText("La Casa de las Dos Palmas");
    expect(container.querySelector(".card")).toBeNull();
    expect(container.querySelector(".panel")).toBeTruthy();
  });

  it("filtra por periodo", async () => {
    render(<Liquidaciones />);
    await screen.findByText("La Casa de las Dos Palmas");
    fireEvent.change(screen.getByLabelText("Periodo"), {
      target: { value: "2026-01" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Filtrar" }));
    await waitFor(() =>
      expect(vi.mocked(api)).toHaveBeenCalledWith(
        "/api/mis-liquidaciones/obras?periodo=2026-01",
      ),
    );
  });

  it("exporta PDF y Excel con el periodo filtrado", async () => {
    render(<Liquidaciones />);
    await screen.findByText("La Casa de las Dos Palmas");
    fireEvent.change(screen.getByLabelText("Periodo"), {
      target: { value: "2026-01" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Filtrar" }));
    await waitFor(() =>
      expect(vi.mocked(api).mock.calls.length).toBeGreaterThan(1),
    );

    fireEvent.click(screen.getByRole("button", { name: "Descargar PDF" }));
    await waitFor(() =>
      expect(vi.mocked(descargar)).toHaveBeenCalledWith(
        "/api/mis-liquidaciones/export?formato=pdf&periodo=2026-01",
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "Descargar Excel" }));
    await waitFor(() =>
      expect(vi.mocked(descargar)).toHaveBeenCalledWith(
        "/api/mis-liquidaciones/export?formato=xlsx&periodo=2026-01",
      ),
    );
  });

  it("sin liquidaciones muestra un vacio amable y no ofrece exportar", async () => {
    vi.mocked(api).mockResolvedValue({
      ...panel,
      lineas: [],
      totales: { ...panel.totales, neto: "0.00" },
    });
    render(<Liquidaciones />);
    expect(
      await screen.findByText("Todavía no tienes liquidaciones"),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Descargar PDF" })).toBeNull();
  });

  it("un error de la API se anuncia", async () => {
    vi.mocked(api).mockRejectedValue(new Error("se cayo la base"));
    render(<Liquidaciones />);
    expect((await screen.findByRole("alert")).textContent).toContain(
      "se cayo la base",
    );
  });
});
