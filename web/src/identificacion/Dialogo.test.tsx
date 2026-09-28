import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import Dialogo from "./Dialogo";

afterEach(() => {
  cleanup();
});

/**
 * Envuelve `Dialogo` con un boton fuera que lo abre, para poder afirmar "el
 * foco vuelve a quien lo abrio" sin inventar un `document.activeElement` de
 * mentira.
 */
function Envoltura({
  bloqueado = false,
  onCerrar,
  sinEnfocables = false,
}: {
  bloqueado?: boolean;
  onCerrar: () => void;
  sinEnfocables?: boolean;
}) {
  const [abierto, setAbierto] = useState(false);
  return (
    <div>
      <button type="button" onClick={() => setAbierto(true)}>
        Abrir
      </button>
      <Dialogo
        abierto={abierto}
        variante="lateral"
        idTitulo="titulo-dialogo"
        bloqueado={bloqueado}
        onCerrar={() => {
          setAbierto(false);
          onCerrar();
        }}
      >
        <h2 id="titulo-dialogo">Panel de prueba</h2>
        {!sinEnfocables && (
          <>
            <button type="button">Primero</button>
            <button type="button">Segundo</button>
          </>
        )}
      </Dialogo>
    </div>
  );
}

describe("Dialogo", () => {
  it("tiene role=dialog, aria-modal y aria-labelledby apuntando al titulo", () => {
    render(<Envoltura onCerrar={vi.fn()} />);
    fireEvent.click(screen.getByText("Abrir"));

    const dialogo = screen.getByRole("dialog");
    expect(dialogo.getAttribute("aria-modal")).toBe("true");
    expect(dialogo.getAttribute("aria-labelledby")).toBe("titulo-dialogo");
  });

  it("pone el foco en el primer elemento enfocable al abrir", () => {
    render(<Envoltura onCerrar={vi.fn()} />);
    fireEvent.click(screen.getByText("Abrir"));

    expect(document.activeElement).toBe(screen.getByText("Primero"));
  });

  it("sin elementos enfocables dentro, el foco va al contenedor", () => {
    render(<Envoltura onCerrar={vi.fn()} sinEnfocables />);
    fireEvent.click(screen.getByText("Abrir"));

    expect(document.activeElement).toBe(screen.getByRole("dialog"));
  });

  it("Tab avanza y envuelve al final; Shift+Tab retrocede y envuelve al principio", () => {
    render(<Envoltura onCerrar={vi.fn()} />);
    fireEvent.click(screen.getByText("Abrir"));

    const primero = screen.getByText("Primero");
    const segundo = screen.getByText("Segundo");
    expect(document.activeElement).toBe(primero);

    fireEvent.keyDown(document, { key: "Tab" });
    expect(document.activeElement).toBe(segundo);

    // Desde el ultimo, Tab hacia adelante envuelve al primero: el foco nunca
    // sale del dialogo hacia el resto de la pagina.
    fireEvent.keyDown(document, { key: "Tab" });
    expect(document.activeElement).toBe(primero);

    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(segundo);
  });

  it("Escape cierra el dialogo", () => {
    const onCerrar = vi.fn();
    render(<Envoltura onCerrar={onCerrar} />);
    fireEvent.click(screen.getByText("Abrir"));

    fireEvent.keyDown(document, { key: "Escape" });

    expect(onCerrar).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("bloqueado: Escape no cierra", () => {
    const onCerrar = vi.fn();
    render(<Envoltura bloqueado onCerrar={onCerrar} />);
    fireEvent.click(screen.getByText("Abrir"));

    fireEvent.keyDown(document, { key: "Escape" });

    expect(onCerrar).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeNull();
  });

  it("el clic en el fondo (fuera del dialogo) cierra", () => {
    const onCerrar = vi.fn();
    const { container } = render(<Envoltura onCerrar={onCerrar} />);
    fireEvent.click(screen.getByText("Abrir"));

    const fondo = container.querySelector(".dialogo-fondo");
    if (!fondo) throw new Error("no se encontro el fondo del dialogo");
    fireEvent.mouseDown(fondo);

    expect(onCerrar).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("un clic DENTRO del dialogo no cierra (no es el fondo)", () => {
    const onCerrar = vi.fn();
    render(<Envoltura onCerrar={onCerrar} />);
    fireEvent.click(screen.getByText("Abrir"));

    fireEvent.mouseDown(screen.getByRole("dialog"));

    expect(onCerrar).not.toHaveBeenCalled();
  });

  it("bloqueado: el clic en el fondo no cierra", () => {
    const onCerrar = vi.fn();
    const { container } = render(<Envoltura bloqueado onCerrar={onCerrar} />);
    fireEvent.click(screen.getByText("Abrir"));

    const fondo = container.querySelector(".dialogo-fondo");
    if (!fondo) throw new Error("no se encontro el fondo del dialogo");
    fireEvent.mouseDown(fondo);

    expect(onCerrar).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeNull();
  });

  it("al cerrar, el foco vuelve a quien tenia el foco antes de abrir", () => {
    render(<Envoltura onCerrar={vi.fn()} />);
    const boton = screen.getByText("Abrir");
    boton.focus();
    expect(document.activeElement).toBe(boton);

    fireEvent.click(boton);
    expect(document.activeElement).toBe(screen.getByText("Primero"));

    fireEvent.keyDown(document, { key: "Escape" });

    expect(document.activeElement).toBe(boton);
  });
});
