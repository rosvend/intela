import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import Cargando from "./Cargando";

describe("Cargando", () => {
  afterEach(cleanup);

  it("se anuncia como estado y trae texto legible: una figura girando no dice nada a un lector de pantalla", () => {
    render(<Cargando />);

    const estado = screen.getByRole("status");
    expect(estado).toBeTruthy();
    expect(estado.textContent).toContain("Cargando");
  });

  it("acepta un texto propio para decir QUE se esta cargando", () => {
    render(<Cargando texto="Consultando el backend…" />);

    expect(screen.getByRole("status").textContent).toBe(
      "Consultando el backend…",
    );
  });

  it("la marca es decorativa: se oculta del arbol de accesibilidad para no duplicar el anuncio", () => {
    const { container } = render(<Cargando />);

    const marca = container.querySelector(".logo-spinner");
    expect(marca).toBeTruthy();
    expect(marca?.getAttribute("aria-hidden")).toBe("true");
  });

  it("pinta la marca como mascara, no como imagen: el PNG es rosa y el color lo mandan los tokens", () => {
    const { container } = render(<Cargando />);

    const marca = container.querySelector(".logo-spinner") as HTMLElement;
    // Enmascarar usa solo el canal alfa; el color sale de --acento via CSS.
    expect(marca.style.maskImage || marca.style.webkitMaskImage).toContain(
      "url(",
    );
    expect(container.querySelector("img")).toBeNull();
  });
});
