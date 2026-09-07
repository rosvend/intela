import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Rol } from "../sesion";
import Compuerta from "./Compuerta";
import { Proceso } from "./tipos";

function proceso(parcial: Partial<Proceso> = {}): Proceso {
  return {
    id: "proc-1",
    circuito: "nacional",
    etapa: "verificacion",
    periodo: "2025",
    revision: 1,
    firmas: [],
    ...parcial,
  };
}

function montar(
  rol: Rol,
  p: Proceso = proceso(),
  extras: {
    onFirmar?: () => void;
    onRechazar?: (motivo: string) => void;
    error?: string;
  } = {},
) {
  return render(
    <Compuerta
      proceso={p}
      rol={rol}
      error={extras.error}
      onFirmar={extras.onFirmar ?? vi.fn()}
      onRechazar={extras.onRechazar ?? vi.fn()}
    />,
  );
}

describe("Compuerta", () => {
  afterEach(() => {
    cleanup();
  });

  it("fuera de una compuerta no pinta nada", () => {
    const { container } = montar("distribucion", proceso({ etapa: "recaudo" }));
    expect(container.textContent).toBe("");
  });

  it("muestra que roles firmaron y cuales faltan", () => {
    montar(
      "administrador",
      proceso({
        firmas: [{ rol: "distribucion", actor_id: "usr-1", sobre_rev: 1 }],
      }),
    );
    expect(
      screen.getByText("Distribución").parentElement?.textContent,
    ).toContain("Firmado");
    expect(
      screen.getByText("Contabilidad").parentElement?.textContent,
    ).toContain("Pendiente");
  });

  it("distribucion ve Firmar y Rechazar si su firma falta", () => {
    montar("distribucion");
    expect(screen.getByRole("button", { name: "Firmar" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Rechazar" })).toBeTruthy();
  });

  it("contabilidad ve Firmar si su firma falta", () => {
    montar("contabilidad");
    expect(screen.getByRole("button", { name: "Firmar" })).toBeTruthy();
  });

  it("quien ya firmo no vuelve a ver el control", () => {
    montar(
      "distribucion",
      proceso({
        firmas: [{ rol: "distribucion", actor_id: "usr-1", sobre_rev: 1 }],
      }),
    );
    expect(screen.queryByRole("button", { name: "Firmar" })).toBeNull();
  });

  it.each(["administrador", "auditor", "titular"] as Rol[])(
    "el rol %s no ve Firmar ni Rechazar",
    (rol) => {
      montar(rol);
      expect(screen.queryByRole("button", { name: "Firmar" })).toBeNull();
      expect(screen.queryByRole("button", { name: "Rechazar" })).toBeNull();
      expect(screen.getAllByText("Pendiente").length).toBe(2);
    },
  );

  it("Firmar dispara el callback", () => {
    const onFirmar = vi.fn();
    montar("distribucion", proceso(), { onFirmar });
    fireEvent.click(screen.getByRole("button", { name: "Firmar" }));
    expect(onFirmar).toHaveBeenCalledTimes(1);
  });

  it("Rechazar pide motivo y lo envia", () => {
    const onRechazar = vi.fn();
    montar("contabilidad", proceso(), { onRechazar });
    fireEvent.click(screen.getByRole("button", { name: "Rechazar" }));
    fireEvent.change(screen.getByLabelText("Motivo del rechazo"), {
      target: { value: "cifras no cuadran" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Confirmar rechazo" }));
    expect(onRechazar).toHaveBeenCalledWith("cifras no cuadran");
  });
});
