import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import SnakeCanvas from "./SnakeCanvas";
import { DEFINICIONES } from "./snake";

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
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    fillStyle: "",
    strokeStyle: "",
    lineWidth: 0,
    lineCap: "butt" as CanvasLineCap,
    lineJoin: "miter" as CanvasLineJoin,
    globalAlpha: 1,
  };
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(
    ctx as unknown as CanvasRenderingContext2D,
  );
  return ctx;
}

describe("SnakeCanvas", () => {
  it("se esconde del lector de pantalla: no se puede hacer nada con el", () => {
    const { container } = render(<SnakeCanvas />);
    const lienzo = container.querySelector("canvas");
    expect(lienzo?.getAttribute("aria-hidden")).toBe("true");
    // Sin rol, porque no hay nada que anunciar.
    expect(screen.queryByRole("img")).toBeNull();
  });

  it("no es una parada de tabulador", () => {
    // Una parada que no lleva a ninguna accion es una trampa para quien
    // navega con teclado. Antes era enfocable porque se dirigia; ya no.
    const { container } = render(<SnakeCanvas />);
    expect(container.querySelector("canvas")?.hasAttribute("tabindex")).toBe(
      false,
    );
  });

  it("no escucha el teclado, asi que no puede robarle teclas al formulario", () => {
    contexto2DFalso();
    const poner = vi.spyOn(window, "addEventListener");
    render(<SnakeCanvas />);
    // La guarda de foco existia porque habia un keydown en window. Al dejar de
    // ser interactivo el riesgo desaparece por construccion: no hay escuchador.
    expect(poner.mock.calls.some(([t]) => t === "keydown")).toBe(false);
  });

  it("no revienta sin contexto 2D", () => {
    // El navegador con el lienzo deshabilitado, y jsdom por defecto.
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
    const { container } = render(<SnakeCanvas />);
    expect(container.querySelector("canvas")).toBeTruthy();
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

  it("pinta todas las serpientes en cada frame", () => {
    const ctx = contexto2DFalso();
    // En un array y no en un `let`: asignar dentro del callback no lo ve el
    // analisis de flujo de TypeScript, que despues estrecha la variable a
    // `never` y no la deja llamar.
    const pedidos: FrameRequestCallback[] = [];
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((cb) => {
      pedidos.push(cb);
      return 1;
    });

    render(<SnakeCanvas />);
    ctx.stroke.mockClear();
    ctx.drawImage.mockClear();
    pedidos[0]?.(16);

    // Contra DEFINICIONES.length y no contra un numero escrito: la primera
    // version fijaba 3, y al anadir dos serpientes la prueba fallo por el
    // numero, no por el comportamiento.
    expect(ctx.stroke.mock.calls.length).toBeGreaterThanOrEqual(
      DEFINICIONES.length,
    );
    expect(ctx.restore.mock.calls.length).toBeGreaterThanOrEqual(
      DEFINICIONES.length,
    );
  });

  it("usa trazos y no un circulo por nodo", () => {
    const ctx = contexto2DFalso();
    // En un array y no en un `let`: asignar dentro del callback no lo ve el
    // analisis de flujo de TypeScript, que despues estrecha la variable a
    // `never` y no la deja llamar.
    const pedidos: FrameRequestCallback[] = [];
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((cb) => {
      pedidos.push(cb);
      return 1;
    });

    render(<SnakeCanvas />);
    ctx.arc.mockClear();
    ctx.stroke.mockClear();
    pedidos[0]?.(16);

    // Con circulos el cuerpo se veia como un collar de cuentas. Ahora el cuerpo
    // es `stroke`; los `arc` que queden son solo las cabezas sin imagen, uno
    // por serpiente como maximo.
    expect(ctx.stroke.mock.calls.length).toBeGreaterThan(0);
    expect(ctx.arc.mock.calls.length).toBeLessThanOrEqual(DEFINICIONES.length);
  });
});
