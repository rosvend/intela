import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import Pipeline from "./Pipeline";
import {
  ETAPAS_INTERNACIONAL,
  ETAPAS_NACIONAL,
  ETIQUETA_ETAPA,
} from "./etapas";
import { Etapa } from "./tipos";

describe("Pipeline", () => {
  afterEach(() => {
    cleanup();
  });

  it.each(ETAPAS_NACIONAL)(
    "marca %s como paso actual en el circuito nacional",
    (etapa: Etapa) => {
      render(<Pipeline circuito="nacional" etapa={etapa} />);
      const actual = screen.getByRole("listitem", { current: "step" });
      expect(actual.textContent).toBe(ETIQUETA_ETAPA[etapa]);
      expect(screen.getByText("Importe de la obra")).toBeTruthy();
      expect(screen.queryByText("Fees in Error")).toBeNull();
      cleanup();
    },
  );

  it.each(ETAPAS_INTERNACIONAL)(
    "marca %s como paso actual en el circuito internacional",
    (etapa: Etapa) => {
      render(<Pipeline circuito="internacional" etapa={etapa} />);
      const actual = screen.getByRole("listitem", { current: "step" });
      expect(actual.textContent).toBe(ETIQUETA_ETAPA[etapa]);
      expect(screen.queryByText("Importe de la obra")).toBeNull();
      expect(screen.getByText("Fees in Error")).toBeTruthy();
      cleanup();
    },
  );

  it("cada paso dice su estado al lector: hecha, en curso o pendiente", () => {
    render(<Pipeline circuito="nacional" etapa="deducciones" />);
    expect(
      screen.getByRole("button", { name: "Recaudo: completada" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Deducciones: en curso" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Auditoría: pendiente" }),
    ).toBeTruthy();
  });

  it("el detalle de una compuerta explica la etapa y quien firma", () => {
    render(<Pipeline circuito="nacional" etapa="verificacion" />);
    fireEvent.click(
      screen.getByRole("button", { name: "Verificación: en curso" }),
    );
    const detalle = screen.getByRole("dialog", { name: "Verificación" });
    expect(detalle.textContent).toMatch(/revisan las cifras/);
    expect(detalle.textContent).toContain(
      "Firman: Distribución y Contabilidad",
    );
  });

  it("una etapa sin compuerta dice que no necesita firmas", () => {
    render(<Pipeline circuito="nacional" etapa="recaudo" />);
    fireEvent.click(screen.getByRole("button", { name: "Recaudo: en curso" }));
    expect(screen.getByRole("dialog").textContent).toContain(
      "No necesita firmas.",
    );
  });

  it("compacto no ofrece detalles: es un resumen, no un control", () => {
    render(<Pipeline circuito="nacional" etapa="verificacion" compacto />);
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByRole("listitem", { current: "step" }).textContent).toBe(
      "Verificación",
    );
  });
});
