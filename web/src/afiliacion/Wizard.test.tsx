import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import WizardAfiliacion from "./Wizard";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

function pdf() {
  return new File(["%PDF-1.4\nprueba"], "doc.pdf", { type: "application/pdf" });
}

async function completarHasta(pasoObjetivo: number) {
  fireEvent.change(screen.getByLabelText(/^Nombre$/i), {
    target: { value: "Ana Escritora" },
  });
  fireEvent.change(screen.getByLabelText(/^Correo$/i), {
    target: { value: "ana@redes.co" },
  });
  fireEvent.change(screen.getByLabelText(/Documento de identidad/i), {
    target: { value: "12345678" },
  });
  fireEvent.click(screen.getByRole("button", { name: /siguiente/i }));

  if (pasoObjetivo < 1) return;
  fireEvent.click(screen.getByRole("radio", { name: /^Socio / }));
  fireEvent.click(screen.getByRole("button", { name: /siguiente/i }));

  if (pasoObjetivo < 2) return;
  fireEvent.change(screen.getByLabelText(/RUT actualizado/i), {
    target: { files: [pdf()] },
  });
  fireEvent.change(screen.getByLabelText(/Certificación bancaria/i), {
    target: { files: [pdf()] },
  });
  fireEvent.click(screen.getByRole("button", { name: /siguiente/i }));

  if (pasoObjetivo < 3) return;
}

describe("WizardAfiliacion", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  it("bloquea el envio si declara otra SGC sin renuncia", async () => {
    render(<WizardAfiliacion />);
    await completarHasta(3);

    fireEvent.click(screen.getByRole("radio", { name: /Sí pertenezco/i }));
    fireEvent.click(screen.getByRole("button", { name: /siguiente/i }));

    expect(screen.getByRole("alert").textContent).toMatch(/R-28/i);
    expect(
      screen.queryByRole("button", { name: /enviar solicitud/i }),
    ).toBeNull();
  });

  it("completa el alta y deja la solicitud pendiente de admision", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "afil-1",
          estado: "pendiente",
          subtipo: "socio",
          elegible_anticipo: false,
        }),
        { status: 201, headers: { "content-type": "application/json" } },
      ),
    );

    render(<WizardAfiliacion />);
    await completarHasta(3);
    fireEvent.click(
      screen.getByRole("radio", { name: /no pertenezco a otra SGC/i }),
    );
    fireEvent.click(screen.getByRole("button", { name: /siguiente/i }));
    fireEvent.click(screen.getByRole("button", { name: /enviar solicitud/i }));

    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toMatch(
        /pendiente de admisión/i,
      );
    });

    const [, init] = vi.mocked(fetch).mock.calls[0];
    expect(init?.method).toBe("POST");
    expect(init?.body).toBeInstanceOf(FormData);
    const fd = init?.body as FormData;
    expect(fd.get("nombre")).toBe("Ana Escritora");
    expect(fd.get("subtipo")).toBe("socio");
    expect(fd.get("rut")).toBeInstanceOf(File);
    expect(fd.get("certificacion_bancaria")).toBeInstanceOf(File);
  });

  it("muestra la explicacion del servidor ante un 409 de exclusividad", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          error:
            "r-28: no se acepta como afiliado a quien pertenezca a otra sociedad de gestion colectiva del mismo genero sin renuncia previa y expresa",
        }),
        { status: 409, headers: { "content-type": "application/json" } },
      ),
    );

    render(<WizardAfiliacion />);
    await completarHasta(3);
    fireEvent.click(screen.getByRole("radio", { name: /Sí pertenezco/i }));
    fireEvent.change(screen.getByLabelText(/Documento de renuncia/i), {
      target: { files: [pdf()] },
    });
    fireEvent.click(screen.getByRole("button", { name: /siguiente/i }));
    fireEvent.click(screen.getByRole("button", { name: /enviar solicitud/i }));

    await waitFor(() => {
      expect(screen.getByRole("alert").textContent).toMatch(/r-28/i);
    });
  });
});
