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

  it("dice en claro cuantas firmas pide la etapa y cuantas lleva", () => {
    montar(
      "administrador",
      proceso({
        firmas: [{ rol: "distribucion", actor_id: "usr-1", sobre_rev: 1 }],
      }),
    );
    expect(
      screen.getByRole("heading", { name: "Esta etapa necesita 2 firmas" }),
    ).toBeTruthy();
    expect(screen.getByText("1 de 2")).toBeTruthy();
  });

  it("una firma de una revision anterior no cuenta", () => {
    montar(
      "distribucion",
      proceso({
        revision: 2,
        firmas: [{ rol: "distribucion", actor_id: "usr-1", sobre_rev: 1 }],
      }),
    );
    expect(screen.getByText("0 de 2")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Firmar" })).toBeTruthy();
  });

  it("los ids crudos viven en el detalle tecnico, no en la tarjeta", () => {
    montar(
      "administrador",
      proceso({
        revision: 3,
        firmas: [{ rol: "contabilidad", actor_id: "usr-c9", sobre_rev: 3 }],
      }),
    );
    const tecnico = screen
      .getByText("Detalle técnico de las firmas")
      .closest("details");
    expect(tecnico).toBeTruthy();
    expect(tecnico?.hasAttribute("open")).toBe(false);
    expect(tecnico?.textContent).toContain("proc-1");
    expect(tecnico?.textContent).toContain("usr-c9");
    expect(tecnico?.textContent).toContain("Revisión 3");
  });

  it("mientras se envia la firma los botones se bloquean", () => {
    render(
      <Compuerta
        proceso={proceso()}
        rol="distribucion"
        enviando
        onFirmar={vi.fn()}
        onRechazar={vi.fn()}
      />,
    );
    expect(
      (screen.getByRole("button", { name: /Firmando/ }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("button", { name: "Rechazar" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });
});
