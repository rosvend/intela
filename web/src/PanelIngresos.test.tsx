import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FilaIngreso, PanelExplicacion, PanelIngresos } from "./PanelIngresos";
import type { Explicacion, Ingreso } from "./ingresos";
import { api } from "./api";

vi.mock("./api", () => ({
  api: vi.fn(),
}));

const anaCasa: Ingreso = {
  ref: "proc-2026-01:obra-completa:tit-ana",
  obra_id: "obra-completa",
  obra: "La Casa de las Dos Palmas",
  fuente: "caracol",
  periodo: "2026-01",
  neto: "3600.00",
};

const anaSegundo: Ingreso = {
  ref: "proc-2026-01:obra-ana-2:tit-ana",
  obra_id: "obra-ana-2",
  obra: "El Segundo Guion",
  fuente: "caracol",
  periodo: "2026-01",
  neto: "750.00",
};

const linaje: Explicacion = {
  ref: anaCasa.ref,
  neto: "3600.00",
  bruto: "4800.00",
  corrida: {
    proceso_id: "proc-2026-01",
    periodo: "2026-01",
    circuito: "nacional",
  },
  reporte: {
    id: "rpt-caracol-2026-01",
    fuente: "caracol",
    sha256: "aaaa",
  },
  obra: {
    id: "obra-completa",
    titulo: "La Casa de las Dos Palmas",
    escalon: "alias",
    puntaje: "1.00000",
  },
  regla: { snapshot_id: "snap-2026-01", reglamento: "RD-IX" },
  split: {
    titular_id: "tit-ana",
    ipi: "IPI-00000001",
    porcentaje: "60.0000",
    version: 1,
  },
  deducciones: [
    {
      concepto: "gastos administrativos",
      porcentaje: "10.00",
      monto: "480.00",
    },
    { concepto: "bienestar social", porcentaje: "5.00", monto: "240.00" },
    { concepto: "reserva", porcentaje: "10.00", monto: "480.00" },
  ],
};

afterEach(() => {
  cleanup();
  vi.mocked(api).mockReset();
});

describe("PanelExplicacion", () => {
  it("pinta el linaje de ExplicarCifra tal cual llega", () => {
    render(<PanelExplicacion cifra={linaje} />);

    const panel = screen.getByRole("region", {
      name: "Explicacion de la cifra",
    });
    expect(panel.textContent).toContain("3600.00");
    expect(panel.textContent).toContain("4800.00");
    expect(panel.textContent).toContain("proc-2026-01");
    expect(panel.textContent).toContain("rpt-caracol-2026-01");
    expect(panel.textContent).toContain("caracol");
    expect(panel.textContent).toContain("escalon alias");
    expect(panel.textContent).toContain("puntaje 1.00000");
    expect(panel.textContent).toContain("RD-IX");
    expect(panel.textContent).toContain("snap-2026-01");
    expect(panel.textContent).toContain("60.0000%");
    expect(panel.textContent).toContain("declaracion v1");
    expect(panel.textContent).toContain("gastos administrativos");
    expect(panel.textContent).toContain("bienestar social");
    expect(panel.textContent).toContain("reserva");
  });
});

describe("FilaIngreso", () => {
  it("enseña el neto como cifra de cabecera, nunca el bruto", () => {
    render(
      <table>
        <tbody>
          <FilaIngreso
            fila={anaCasa}
            abierta={false}
            explicacion={null}
            error=""
            cargando={false}
            onExplicar={() => undefined}
          />
        </tbody>
      </table>,
    );
    expect(screen.getByText("$ 3600.00")).toBeTruthy();
    expect(screen.queryByText("$ 4800.00")).toBeNull();
    expect(screen.queryByRole("columnheader", { name: "Bruto" })).toBeNull();
  });

  it("expande a la explicacion con un clic", () => {
    const onExplicar = vi.fn();
    render(
      <table>
        <tbody>
          <FilaIngreso
            fila={anaCasa}
            abierta={false}
            explicacion={null}
            error=""
            cargando={false}
            onExplicar={onExplicar}
          />
        </tbody>
      </table>,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Explicar esta cifra" }),
    );
    expect(onExplicar).toHaveBeenCalledOnce();
  });
});

describe("PanelIngresos", () => {
  beforeEach(() => {
    vi.mocked(api).mockImplementation(async (path: string) => {
      if (path.startsWith("/api/mis-ingresos")) {
        return { ingresos: [anaCasa, anaSegundo] };
      }
      if (path.includes(encodeURIComponent(anaCasa.ref))) {
        return linaje;
      }
      if (path.includes("tit-beto")) {
        throw new Error('{"error":"no autorizado"}');
      }
      throw new Error("ruta no mockeada: " + path);
    });
  });

  it("carga como titular A y solo pinta las obras de A", async () => {
    render(<PanelIngresos />);

    expect(
      await screen.findByRole("cell", { name: "La Casa de las Dos Palmas" }),
    ).toBeTruthy();
    expect(screen.getByRole("cell", { name: "El Segundo Guion" })).toBeTruthy();
    expect(screen.queryByText("Solo de Beto")).toBeNull();
    expect(screen.queryByText("tit-beto")).toBeNull();

    const llamada = vi.mocked(api).mock.calls[0][0];
    expect(llamada).toBe("/api/mis-ingresos");
    expect(llamada).not.toContain("titular");
  });

  it("los tres filtros recortan la tabla", async () => {
    render(<PanelIngresos />);
    await screen.findByRole("cell", { name: "El Segundo Guion" });

    fireEvent.change(screen.getByLabelText("Filtrar por obra"), {
      target: { value: "obra-completa" },
    });
    expect(
      screen.getByRole("cell", { name: "La Casa de las Dos Palmas" }),
    ).toBeTruthy();
    expect(screen.queryByRole("cell", { name: "El Segundo Guion" })).toBeNull();

    fireEvent.change(screen.getByLabelText("Filtrar por obra"), {
      target: { value: "" },
    });
    fireEvent.change(screen.getByLabelText("Filtrar por fuente"), {
      target: { value: "caracol" },
    });
    expect(screen.getByRole("cell", { name: "El Segundo Guion" })).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Filtrar por periodo"), {
      target: { value: "2026-01" },
    });
    expect(screen.getAllByText("2026-01").length).toBeGreaterThan(0);
  });

  it("al explicar, pinta el linaje exacto de ExplicarCifra", async () => {
    render(<PanelIngresos />);
    await screen.findByRole("cell", { name: "La Casa de las Dos Palmas" });

    fireEvent.click(
      screen.getAllByRole("button", { name: "Explicar esta cifra" })[0],
    );

    await waitFor(() => {
      expect(
        screen.getByRole("region", { name: "Explicacion de la cifra" }),
      ).toBeTruthy();
    });
    const panel = screen.getByRole("region", {
      name: "Explicacion de la cifra",
    });
    expect(panel.textContent).toContain("snap-2026-01");
    expect(panel.textContent).toContain("escalon alias");
    expect(panel.textContent).toContain("4800.00");
  });

  it("pedir la ref de otro titular muestra 403, no sus datos", async () => {
    render(<PanelIngresos />);
    await screen.findByRole("cell", { name: "La Casa de las Dos Palmas" });

    vi.mocked(api).mockRejectedValueOnce(
      new Error('{"error":"no autorizado"}'),
    );
    fireEvent.click(
      screen.getAllByRole("button", { name: "Explicar esta cifra" })[0],
    );

    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.queryByText("Solo de Beto")).toBeNull();
    expect(screen.queryByText("tit-beto")).toBeNull();
  });
});
