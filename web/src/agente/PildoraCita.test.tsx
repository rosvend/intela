import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import PildoraCita from "./PildoraCita";
import TextoConCitas from "./TextoConCitas";

function conPortapapeles(writeText: (t: string) => Promise<void>) {
  Object.defineProperty(navigator, "clipboard", {
    value: { writeText },
    configurable: true,
  });
}

describe("PildoraCita", () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    Object.defineProperty(navigator, "clipboard", {
      value: undefined,
      configurable: true,
    });
  });

  it("es un boton con nombre accesible que dice que copia", () => {
    render(<PildoraCita clase="reglamento" valor="RD 9.1.1" />);
    const boton = screen.getByRole("button", { name: "Copiar cita RD 9.1.1" });
    expect(boton.textContent).toContain("RD 9.1.1");
  });

  it("al hacer clic copia la cita y lo anuncia", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    conPortapapeles(writeText);
    render(<PildoraCita clase="asiento" valor="p1:obra-7" />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });

    expect(writeText).toHaveBeenCalledWith("p1:obra-7");
    expect(screen.getByText("Cita copiada")).toBeTruthy();
  });

  it("sin portapapeles deja la cita seleccionada para copiarla a mano", async () => {
    render(<PildoraCita clase="reglamento" valor="RT 5" />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });

    expect(window.getSelection()?.toString()).toBe("RT 5");
    expect(screen.getByText(/Ctrl\+C/)).toBeTruthy();
  });

  it("una cita de asiento se distingue de un numeral de reglamento", () => {
    render(
      <>
        <PildoraCita clase="asiento" valor="p1:obra-7" />
        <PildoraCita clase="reglamento" valor="RD 9.1.1" />
      </>,
    );
    const [asiento, numeral] = screen.getAllByRole("button");
    expect(screen.queryByRole("status")).toBeNull();
    expect(asiento.className).toContain("cita--asiento");
    expect(asiento.textContent).toContain("asiento");
    expect(numeral.className).toContain("cita--reglamento");
  });
});

describe("TextoConCitas", () => {
  afterEach(cleanup);

  it("una respuesta con citas mixtas pinta una pildora por cita y deja el resto como texto", () => {
    const { container } = render(
      <TextoConCitas texto="Se retiene en reserva (RD 13.1.3), ver asiento p1:obra-7." />,
    );
    const pildoras = screen.getAllByRole("button");
    expect(pildoras.map((p) => p.getAttribute("aria-label"))).toEqual([
      "Copiar cita RD 13.1.3",
      "Copiar cita asiento p1:obra-7",
    ]);
    expect(container.textContent).toBe(
      "Se retiene en reserva (RD 13.1.3), ver asiento p1:obra-7.",
    );
  });
});
