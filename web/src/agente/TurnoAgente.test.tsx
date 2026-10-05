import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import AvisoNoSeGuarda from "./AvisoNoSeGuarda";
import TurnoAgente from "./TurnoAgente";
import {
  aplicarEvento,
  cerrarTurno,
  turnoVacio,
  type EventoAgente,
  type TurnoAgente as Turno,
} from "./turno";

const turno = (...eventos: EventoAgente[]): Turno =>
  eventos.reduce(aplicarEvento, turnoVacio);
const herramienta = (h: string): EventoAgente => ({
  evento: "tool_call",
  datos: { herramienta: h },
});
const respuesta = (
  texto: string,
  extra: Partial<{ parcial: boolean; restringida: boolean }> = {},
): EventoAgente => ({
  evento: "answer",
  datos: { texto, parcial: false, restringida: false, ...extra },
});

describe("TurnoAgente", () => {
  afterEach(cleanup);

  it("N eventos tool_call producen N chips en el orden recibido, antes de la respuesta", () => {
    render(
      <TurnoAgente
        turno={turno(
          herramienta("buscar_obra"),
          herramienta("buscar_reglamento"),
          herramienta("estado_corrida"),
        )}
      />,
    );
    const pasos = within(
      screen.getByRole("list", { name: "Pasos del asistente" }),
    ).getAllByRole("listitem");
    expect(pasos.map((p) => p.textContent)).toEqual([
      "Buscando la obra en el catálogo…",
      "Buscando en el Reglamento…",
      "Consultando el estado de la corrida…",
    ]);
    expect(pasos[2].getAttribute("aria-current")).toBe("step");
    expect(pasos[0].getAttribute("aria-current")).toBeNull();
  });

  it("mientras no llega nada, dice que esta trabajando en vez de quedar en blanco", () => {
    render(<TurnoAgente turno={turnoVacio} />);
    expect(screen.getByRole("listitem").textContent).toBe(
      "Leyendo tu pregunta…",
    );
  });

  it("la respuesta final deja los chips como rastro, sin paso activo, y pinta las citas", () => {
    const { container } = render(
      <TurnoAgente
        turno={turno(
          herramienta("buscar_reglamento"),
          respuesta("Se retiene el total (RD 13.1.3); ver asiento p1:obra-7."),
        )}
      />,
    );
    expect(screen.getAllByRole("listitem")).toHaveLength(1);
    expect(container.querySelector("[aria-current]")).toBeNull();
    expect(screen.getAllByRole("button")).toHaveLength(2);
    expect(screen.queryByRole("note")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("el evento error es un mensaje en linea claro, sin el texto crudo del servidor", () => {
    render(
      <TurnoAgente
        turno={turno(herramienta("buscar_obra"), {
          evento: "error",
          datos: { mensaje: "panic: runtime error at agente.go:150" },
        })}
      />,
    );
    const alerta = screen.getByRole("alert");
    expect(alerta.textContent).toContain("El asistente no está disponible");
    expect(document.body.textContent).not.toContain("panic");
  });

  it("un flujo cortado sin respuesta se ve igual que un error", () => {
    render(
      <TurnoAgente turno={cerrarTurno(turno(herramienta("buscar_obra")))} />,
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "El asistente no está disponible",
    );
  });

  it("una respuesta parcial lleva un aviso que una completa no lleva", () => {
    const { container } = render(
      <TurnoAgente
        turno={turno(respuesta("Quiza RD 9.1.1.", { parcial: true }))}
      />,
    );
    expect(screen.getByRole("note").textContent).toContain("Respuesta parcial");
    expect(container.querySelector(".agente-respuesta--parcial")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Copiar cita RD 9.1.1" }));
  });

  it("una consulta restringida por rol es su propio estado, no una respuesta", () => {
    const { container } = render(
      <TurnoAgente
        turno={turno(
          herramienta("estado_corrida"),
          respuesta("Esa consulta está fuera de lo que tu rol puede ver.", {
            restringida: true,
          }),
        )}
      />,
    );
    const nota = screen.getByRole("note");
    expect(nota.textContent).toContain("Fuera del alcance de tu rol");
    expect(nota.textContent).toContain("tu rol puede ver");
    expect(container.querySelector(".agente-respuesta")).toBeNull();
  });

  it("los eventos en buffer (todos de una vez) pintan lo mismo que en streaming", () => {
    const eventos: EventoAgente[] = [
      herramienta("buscar_obra"),
      herramienta("buscar_reglamento"),
      respuesta("Ver RD 9.1.1 y asiento p1:obra-7."),
    ];

    const enVivo = render(<TurnoAgente turno={turnoVacio} />);
    let t = turnoVacio;
    for (const e of eventos) {
      t = aplicarEvento(t, e);
      enVivo.rerender(<TurnoAgente turno={t} />);
    }
    const htmlEnVivo = enVivo.container.innerHTML;
    cleanup();

    const enBuffer = render(<TurnoAgente turno={turno(...eventos)} />);
    expect(enBuffer.container.innerHTML).toBe(htmlEnVivo);
  });

  it("anuncia lo nuevo a lectores de pantalla", () => {
    const { container } = render(<TurnoAgente turno={turnoVacio} />);
    expect(container.firstElementChild?.getAttribute("aria-live")).toBe(
      "polite",
    );
  });
});

describe("AvisoNoSeGuarda", () => {
  afterEach(cleanup);

  it("dice en una linea que la conversacion no se guarda", () => {
    render(<AvisoNoSeGuarda />);
    expect(screen.getByText("Esta conversación no se guarda.")).toBeTruthy();
  });
});
