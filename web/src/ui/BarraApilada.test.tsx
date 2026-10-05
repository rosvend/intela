import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import BarraApilada from "./BarraApilada";

const segmentos = [
  { id: "a", etiqueta: "Tú recibes", valor: "650.00", detalle: "Tu neto" },
  { id: "b", etiqueta: "Gastos administrativos", valor: "200.00" },
  { id: "c", etiqueta: "Reserva", valor: "150.00" },
];

describe("BarraApilada", () => {
  afterEach(cleanup);

  it("pinta un segmento por importe con ancho proporcional", () => {
    render(<BarraApilada etiqueta="Reparto" segmentos={segmentos} />);
    const botones = screen.getAllByRole("button");
    expect(botones).toHaveLength(3);
    expect((botones[0] as HTMLElement).style.getPropertyValue("--ancho")).toBe(
      "65%",
    );
  });

  it("cada segmento se nombra con su etiqueta e importe", () => {
    render(<BarraApilada etiqueta="Reparto" segmentos={segmentos} />);
    expect(
      screen.getByRole("button", { name: /Tú recibes.*\$ 650/ }),
    ).toBeTruthy();
  });

  it("al pulsar un segmento abre su detalle y Escape lo cierra", () => {
    render(<BarraApilada etiqueta="Reparto" segmentos={segmentos} />);
    fireEvent.click(screen.getByRole("button", { name: /Tú recibes/ }));
    expect(screen.getByRole("dialog").textContent).toContain("Tu neto");
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("omite segmentos en cero", () => {
    render(
      <BarraApilada
        etiqueta="Reparto"
        segmentos={[...segmentos, { id: "z", etiqueta: "Nada", valor: "0" }]}
      />,
    );
    expect(screen.getAllByRole("button")).toHaveLength(3);
  });
});
