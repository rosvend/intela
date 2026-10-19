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

  it("sin portapapeles, en un asiento selecciona solo la ref, sin la palabra", async () => {
    render(<PildoraCita clase="asiento" valor="p1:obra-7" />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });

    expect(window.getSelection()?.toString()).toBe("p1:obra-7");
  });

  it("el aviso de copiado no monta una region status propia", async () => {
    conPortapapeles(vi.fn().mockResolvedValue(undefined));
    render(<PildoraCita clase="reglamento" valor="RD 9.1.1" />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });

    expect(screen.getByText("Cita copiada")).toBeTruthy();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("el aviso de copiado se retira solo a los dos segundos", async () => {
    vi.useFakeTimers();
    conPortapapeles(vi.fn().mockResolvedValue(undefined));
    render(<PildoraCita clase="reglamento" valor="RD 9.1.1" />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(screen.getByText("Cita copiada")).toBeTruthy();

    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(screen.queryByText("Cita copiada")).toBeNull();
  });

  it("una cita de asiento se distingue de un numeral de reglamento", () => {
    render(
      <>
        <PildoraCita clase="asiento" valor="p1:obra-7" />
        <PildoraCita clase="reglamento" valor="RD 9.1.1" />
      </>,
    );
    const [asiento, numeral] = screen.getAllByRole("button");
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

  it("conserva la mayuscula de Asiento tal como la escribio el agente", () => {
    const { container } = render(<TextoConCitas texto="Asiento p1:obra-7" />);
    expect(container.textContent).toBe("Asiento p1:obra-7");
    expect(
      screen.getByRole("button", { name: "Copiar cita Asiento p1:obra-7" }),
    ).toBeTruthy();
  });

  it("el HTML que escriba el modelo se ve como texto, nunca se interpreta", () => {
    const texto = '<img src=x onerror="alert(1)"> ver RD 9.1.1';
    const { container } = render(<TextoConCitas texto={texto} />);
    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toBe(texto);
  });
});
