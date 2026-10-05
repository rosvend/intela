import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import Cifra from "./Cifra";

describe("Cifra", () => {
  afterEach(cleanup);

  it("expone el importe final exacto a lectores de pantalla desde el primer render", () => {
    render(<Cifra valor="2450000.00" />);
    expect(screen.getByLabelText("$ 2.450.000")).toBeTruthy();
  });

  it("muestra la pastilla de variacion con su signo", () => {
    render(<Cifra valor="100" variacion={5.4} />);
    expect(screen.getByText("+5,4 %")).toBeTruthy();
  });
});
