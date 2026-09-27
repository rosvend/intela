import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";
import LineaTiempo from "./LineaTiempo";
import type { Asiento } from "./tipos";

/**
 * Una historia con un cambio de metadatos, un cambio de reparto y una
 * distribucion, en orden de cadena: es el caso que la vista por obra tiene
 * que saber contar de principio a fin.
 */
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
      version: 2,
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
    hecho: "reparto.corrida_cerrada",
    ref_tipo: "proceso",
    ref_id: "2026-01",
    actor: "",
    payload: {
      fuente: "Caracol",
      reporte: "rep-abc123",
      regla: "RD 13.5.2",
      periodo: "2026-01",
    },
    cuando: "2026-03-07T09:00:00Z",
  },
];

function renderizar(asientos: Asiento[]) {
  return render(
    <MemoryRouter>
      <LineaTiempo asientos={asientos} />
    </MemoryRouter>,
  );
}

afterEach(cleanup);

describe("LineaTiempo", () => {
  it("muestra los tres hechos en orden, cada uno con su familia", () => {
    renderizar(HISTORIA);

    const elementos = screen.getAllByRole("listitem");
    expect(elementos).toHaveLength(3);
    expect(elementos[0]?.textContent).toContain("obra.registrada");
    expect(elementos[0]?.textContent).toContain("Catálogo");
    expect(elementos[1]?.textContent).toContain("Reparto entre autores");
    expect(elementos[2]?.textContent).toContain("Distribución");
  });

  it("la evidencia del reparto muestra version y partes", () => {
    renderizar(HISTORIA);

    const items = screen.getAllByRole("listitem");
    fireEvent.click(within(items[1]!).getByText("Evidencia de origen"));
    expect(within(items[1]!).queryByText("t1")).not.toBeNull();
    expect(within(items[1]!).queryByText("60%")).not.toBeNull();
  });

  it("la evidencia de la distribucion muestra fuente, reporte y regla", () => {
    renderizar(HISTORIA);

    const items = screen.getAllByRole("listitem");
    fireEvent.click(within(items[2]!).getByText("Evidencia de origen"));
    const texto = items[2]?.textContent ?? "";
    expect(texto).toContain("Caracol");
    expect(texto).toContain("rep-abc123");
    expect(texto).toContain("RD 13.5.2");
  });

  it("un asiento sin actor dice proceso automatico, no un id vacio", () => {
    renderizar(HISTORIA);
    expect(document.body.textContent).toContain("Proceso automático");
  });

  it("la referencia a otra obra enlaza a su historia", () => {
    renderizar(HISTORIA);
    const enlaces = screen.getAllByRole("link", {
      name: /historia de la obra obra-9/i,
    });
    expect(enlaces).toHaveLength(2);
    for (const enlace of enlaces) {
      expect(enlace.getAttribute("href")).toBe("/auditoria/obra/obra-9");
    }
  });

  it("no expone ningun control de mutacion", () => {
    renderizar(HISTORIA);
    expect(screen.queryByRole("button")).toBeNull();
    expect(
      screen.queryByRole("link", { name: /editar|eliminar|borrar/i }),
    ).toBeNull();
  });
});
