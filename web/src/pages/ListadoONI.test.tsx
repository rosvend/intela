import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ListadoONI from "./ListadoONI";

const listado = {
  periodo: "2026-01",
  fecha_proceso: "2026-08-31T12:00:00Z",
  direccion_fisica: "Calle 74 #7-35, Bogota D.C.",
  direccion_electronica: "oni@redescritores.com",
  explicacion: "Se publican titulos e informacion identificatoria.",
  obras: [
    {
      id: "uso-1",
      titulo: "Serie Desconocida",
      fuente: "caracol",
      ids_fuente: "ID-99",
      modalidad: "tv",
    },
  ],
};

describe("ListadoONI", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("muestra titulos e identificadores sin montos y sin pedir sesion", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify(listado), {
        headers: { "content-type": "application/json" },
      }),
    );

    render(<ListadoONI />);

    expect(await screen.findByText("Serie Desconocida")).toBeTruthy();
    expect(screen.getByText("caracol")).toBeTruthy();
    expect(screen.getByText("ID-99")).toBeTruthy();
    expect(screen.getByText("Calle 74 #7-35, Bogota D.C.")).toBeTruthy();
    expect(screen.getByText("oni@redescritores.com")).toBeTruthy();
    expect(screen.getByText(/2026-01/)).toBeTruthy();

    expect(screen.queryByRole("columnheader", { name: /monto/i })).toBeNull();
    expect(screen.queryByRole("columnheader", { name: /importe/i })).toBeNull();

    const [, init] = vi.mocked(fetch).mock.calls[0];
    expect(new Headers(init?.headers).get("Authorization")).toBeNull();
  });

  it("es alcanzable cuando no hay listado publicado", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response("no hay listado ONI publicado", { status: 404 }),
    );

    render(<ListadoONI />);

    expect(
      await screen.findByText(/no hay un listado ONI publicado/i),
    ).toBeTruthy();
  });

  it("no redirige al login si el backend falla", async () => {
    const hrefSpy = vi.fn();
    vi.stubGlobal("location", { href: "", assign: hrefSpy });
    vi.mocked(fetch).mockResolvedValue(new Response("caido", { status: 500 }));

    render(<ListadoONI />);

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeTruthy();
    });
    expect(hrefSpy).not.toHaveBeenCalled();
  });
});
