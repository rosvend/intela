import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import SnakeCanvas from "./SnakeCanvas";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

/**
 * jsdom no implementa el contexto 2D, asi que sin doble `getContext` devuelve
 * null y el efecto se retira sin montar nada -- que es un camino que tambien
 * hay que probar, pero no deja ver el bucle.
 *
 * El doble es lo minimo que el pintado llama. Si el componente empieza a usar
 * otra operacion del contexto, esta prueba falla con un TypeError que dice
 * cual: es una forma de que el doble no se quede atras en silencio.
 */
function contexto2DFalso() {
  const ctx = {
    setTransform: vi.fn(),
    clearRect: vi.fn(),
    beginPath: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(),
    translate: vi.fn(),
    rotate: vi.fn(),
    drawImage: vi.fn(),
    fillStyle: "",
  };
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(
    ctx as unknown as CanvasRenderingContext2D,
  );
  return ctx;
}

describe("SnakeCanvas", () => {
  it("se anuncia una vez y se puede ignorar", () => {
    render(<SnakeCanvas />);
    const lienzo = screen.getByRole("img");
    // El nombre dice que hay un juego, como se dirige, y que es decorativo.
    // Quien usa lector de pantalla no necesita el movimiento narrado.
    expect(lienzo.getAttribute("aria-label")).toContain("decorativo");
    expect(lienzo.getAttribute("aria-label")).toContain("flechas");
  });

  it("es alcanzable con el tabulador, para poder dirigirlo con el teclado", () => {
    render(<SnakeCanvas />);
    expect(screen.getByRole("img").getAttribute("tabindex")).toBe("0");
  });

  it("no revienta sin contexto 2D", () => {
    // El navegador con el lienzo deshabilitado, y jsdom por defecto.
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
    expect(() => render(<SnakeCanvas />)).not.toThrow();
    expect(screen.getByRole("img")).toBeTruthy();
  });

  it("pide frames al montar y los cancela al desmontar", () => {
    contexto2DFalso();
    const pedir = vi.spyOn(window, "requestAnimationFrame");
    const cancelar = vi.spyOn(window, "cancelAnimationFrame");

    const { unmount } = render(<SnakeCanvas />);
    expect(pedir).toHaveBeenCalled();

    unmount();
    // Sin esto el bucle sigue pintando sobre un lienzo que ya no esta en el
    // documento, que es la fuga que el requisito nombra.
    expect(cancelar).toHaveBeenCalled();
  });

  it("quita del window el escuchador de teclas al desmontar", () => {
    contexto2DFalso();
    const quitar = vi.spyOn(window, "removeEventListener");

    const { unmount } = render(<SnakeCanvas />);
    unmount();

    // El `keydown` vive en window, no en el lienzo: si se quedara, seguiria
    // robando flechas en la pantalla siguiente.
    expect(quitar.mock.calls.some(([tipo]) => tipo === "keydown")).toBe(true);
  });

  it("deja de observar el tamano al desmontar", () => {
    contexto2DFalso();
    const desconectar = vi.fn();
    vi.stubGlobal(
      "ResizeObserver",
      class {
        observe = vi.fn();
        unobserve = vi.fn();
        disconnect = desconectar;
      },
    );

    const { unmount } = render(<SnakeCanvas />);
    unmount();
    expect(desconectar).toHaveBeenCalled();
  });

  it("se apana sin ResizeObserver, cayendo al evento resize", () => {
    // jsdom no lo trae, y algun navegador viejo tampoco. La pantalla de acceso
    // no puede quedarse en blanco por eso.
    contexto2DFalso();
    vi.stubGlobal("ResizeObserver", undefined);
    const poner = vi.spyOn(window, "addEventListener");
    const quitar = vi.spyOn(window, "removeEventListener");

    const { unmount } = render(<SnakeCanvas />);
    expect(poner.mock.calls.some(([t]) => t === "resize")).toBe(true);

    unmount();
    expect(quitar.mock.calls.some(([t]) => t === "resize")).toBe(true);
  });

  it("empieza en cero puntos", () => {
    render(<SnakeCanvas />);
    expect(screen.getByText("0 puntos")).toBeTruthy();
  });
});
