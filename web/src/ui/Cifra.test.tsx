import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import Cifra from "./Cifra";

describe("Cifra", () => {
  afterEach(cleanup);

  it("expone el importe final exacto a lectores de pantalla desde el primer render", () => {
    render(<Cifra valor="2450000.00" />);
    expect(screen.getByLabelText("$ 2.450.000")).toBeTruthy();
  });

  it("el aria-label lleva el importe grande exacto, sin abreviar", () => {
    render(<Cifra valor="1890000000.00" />);
    expect(screen.getByLabelText("$ 1.890.000.000")).toBeTruthy();
  });

  // #224: el navegador solo puede partir la cifra tras un "." de miles. El
  // salto mismo lo mide e2e/distribucion-angosta.spec.ts en Chromium; aqui,
  // donde caen los cortes y que el texto visible sigue siendo el importe.
  it("solo deja partir la cifra tras cada punto de miles, nunca en la coma ni entre digitos", () => {
    const { container } = render(<Cifra valor="-1111111110111.10" />);
    const valor = screen.getByLabelText("-$ 1.111.111.110.111,10");
    expect(valor.textContent).toBe("-$ 1.111.111.110.111,10");
    expect(container.querySelector(".cifra-numero")?.innerHTML).toBe(
      "1.<wbr>111.<wbr>111.<wbr>110.<wbr>111,10",
    );
  });

  it("muestra la pastilla de variacion con su signo", () => {
    render(<Cifra valor="100" variacion={5.4} />);
    expect(screen.getByText("+5,4 %")).toBeTruthy();
  });
});
