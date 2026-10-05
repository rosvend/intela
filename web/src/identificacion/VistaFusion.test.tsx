import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Obra } from "../catalogo/tipos";
import { DEBOUNCE_TECLEO_MS } from "../useValorDiferido";
import { MAX_NOTA } from "./resolucion";
import type { CandidatoIdentificacion, CasoIdentificacion } from "./tipos";
import VistaFusion from "./VistaFusion";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const laNina = {
  obra_id: "obra-1",
  titulo: "La Niña",
  anio: 2016,
  genero: "Drama",
  puntaje: 0.71,
  titulo_consultado: "La Niña T3 E12",
} satisfies CandidatoIdentificacion;

const laPromesa = {
  obra_id: "obra-2",
  titulo: "La Promesa",
  anio: 2019,
  genero: "Comedia",
  puntaje: 0.43,
  titulo_consultado: "La niña — Capítulo 12",
} satisfies CandidatoIdentificacion;

const caso = {
  id: "caso-1",
  titulo: "La Niña T3 E12",
  titulo_original: "La niña — Capítulo 12",
  fuente: "caracol",
  modalidad: "tv",
  reporte_id: "ING-2024-0890",
  periodo: "2024-11",
  ids_fuente: "ID_Ficha=48213",
  evidencia: "",
  estado: "pendiente",
  candidatos: [laNina, laPromesa],
  obra_asignada: null,
  resuelto_por: null,
  resuelto_en: null,
  ultima_actualizacion: "",
  nota: null,
  sugerencia: {
    decision: "asignar",
    obra_id: "obra-1",
    titulo: "La Niña",
    confianza: 0.5,
    motivo: "",
    orden: ["obra-1", "obra-2"],
    aceptada: null,
    sello: "sello-mostrado",
  },
} satisfies CasoIdentificacion;

const obraDeBusqueda = {
  id: "obra-9",
  titulo: "Otra Novela",
  genero: "Drama",
  anio: 2020,
  tipo: "serie",
  coautores: [],
  estado_declaracion: "completa",
  suma_porcentajes: 100,
  version_vigente: 1,
} satisfies Obra;

function montar(
  props: Partial<Parameters<typeof VistaFusion>[0]> = {},
  casoMontado: CasoIdentificacion = caso,
) {
  const llamadas = {
    onEnviarInicio: vi.fn(),
    onEnviandoCambia: vi.fn(),
    onFalla: vi.fn(),
    onExito: vi.fn(),
    onRecargarTodo: vi.fn(),
  };
  render(<VistaFusion caso={casoMontado} {...llamadas} {...props} />);
  return llamadas;
}

function escribirNota(texto: string) {
  fireEvent.change(screen.getByLabelText("Nota para la bitácora"), {
    target: { value: texto },
  });
}

const unir = () => screen.getByRole("button", { name: "Unir con esta obra" });
const ninguna = () => screen.getByRole("button", { name: "No es ninguna" });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("VistaFusion", () => {
  it("ADR 0007: ninguna candidata llega elegida y el catalogo espera vacio", () => {
    montar();
    const radios = screen.getAllByRole("radio");
    expect(radios).toHaveLength(2);
    expect(radios.every((r) => !(r as HTMLInputElement).checked)).toBe(true);
    expect(
      screen.getByText("Elige una candidata o busca la obra para compararla."),
    ).not.toBeNull();
    escribirNota("algo");
    expect(unir().hasAttribute("disabled")).toBe(true);
  });

  it("cada candidata es una pastilla con su medidor decimal y su nivel", () => {
    montar();
    const medidores = screen.getAllByRole("meter");
    expect(medidores.map((m) => m.getAttribute("aria-valuetext"))).toEqual([
      "Coincidencia alta, 0,71",
      "Coincidencia baja, 0,43",
    ]);
    expect(screen.getByText("0,71")).not.toBeNull();
    expect(screen.queryByText(/71\s?%/)).toBeNull();
    expect(screen.getByText("Sugerencia: asignar a La Niña.")).not.toBeNull();
  });

  it("elegir una candidata la pinta en la tarjeta del catalogo, y cambiarla la reemplaza", () => {
    montar();
    fireEvent.click(screen.getByRole("radio", { name: /La Niña/ }));
    const comparacion = screen.getByRole("group", {
      name: "Comparación campo por campo",
    });
    expect(within(comparacion).getByText("2016")).not.toBeNull();
    expect(within(comparacion).getByText("Drama")).not.toBeNull();
    expect(screen.getByText(/se parece mucho/)).not.toBeNull();

    fireEvent.click(screen.getByRole("radio", { name: /La Promesa/ }));
    expect(within(comparacion).getByText("2019")).not.toBeNull();
    expect(within(comparacion).queryByText("2016")).toBeNull();
  });

  it("unir exige obra y nota; la nota tiene el tope del contrato", () => {
    montar();
    fireEvent.click(screen.getByRole("radio", { name: /La Niña/ }));
    expect(unir().hasAttribute("disabled")).toBe(true);
    expect(ninguna().hasAttribute("disabled")).toBe(true);
    escribirNota("   ");
    expect(unir().hasAttribute("disabled")).toBe(true);
    escribirNota("ok");
    expect(unir().hasAttribute("disabled")).toBe(false);
    expect(ninguna().hasAttribute("disabled")).toBe(false);
    const campo = screen.getByLabelText(
      "Nota para la bitácora",
    ) as HTMLInputElement;
    expect(campo.maxLength).toBe(MAX_NOTA);
    expect(screen.getByText(`2/${MAX_NOTA}`)).not.toBeNull();
  });

  it("unir: POST con decision=asignar, la obra elegida, la nota recortada y el sello", async () => {
    let cuerpo: unknown = null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init?: RequestInit) => {
        cuerpo = JSON.parse(String(init?.body));
        return json({});
      }),
    );
    const llamadas = montar();
    fireEvent.click(screen.getByRole("radio", { name: /La Niña/ }));
    escribirNota("  coincide la ficha  ");
    fireEvent.click(unir());

    expect(llamadas.onEnviarInicio).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(llamadas.onExito).toHaveBeenCalledTimes(1));
    expect(vi.mocked(fetch).mock.calls[0]?.[0]).toBe(
      "/api/identificacion/casos/caso-1/resolucion",
    );
    expect(cuerpo).toEqual({
      decision: "asignar",
      obra_id: "obra-1",
      nota: "coincide la ficha",
      sello: "sello-mostrado",
    });
    expect(llamadas.onExito).toHaveBeenCalledWith({
      obraTitulo: "La Niña",
      esDescarte: false,
    });
  });

  it("no es ninguna: POST con decision=descartar, sin obra_id", async () => {
    let cuerpo: unknown = null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init?: RequestInit) => {
        cuerpo = JSON.parse(String(init?.body));
        return json({});
      }),
    );
    const llamadas = montar();
    escribirNota(" no es del repertorio ");
    fireEvent.click(ninguna());

    await waitFor(() => expect(llamadas.onExito).toHaveBeenCalledTimes(1));
    expect(cuerpo).toEqual({
      decision: "descartar",
      nota: "no es del repertorio",
      sello: "sello-mostrado",
    });
    expect(llamadas.onExito).toHaveBeenCalledWith({
      obraTitulo: null,
      esDescarte: true,
    });
  });

  it("buscar otra obra: nada con el campo vacio, debounce, y la elegida va al catalogo y al POST", async () => {
    let cuerpo: unknown = null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        if (url.startsWith("/api/obras")) return json([obraDeBusqueda]);
        cuerpo = JSON.parse(String(init?.body));
        return json({});
      }),
    );
    const llamadas = montar();
    fireEvent.click(screen.getByRole("button", { name: "Buscar otra obra" }));
    const campo = screen.getByLabelText("Título de la obra");
    expect(fetch).not.toHaveBeenCalled();

    vi.useFakeTimers();
    fireEvent.change(campo, { target: { value: "Otra" } });
    expect(fetch).not.toHaveBeenCalled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(DEBOUNCE_TECLEO_MS);
    });
    vi.useRealTimers();
    expect(vi.mocked(fetch).mock.calls[0]?.[0]).toBe(
      "/api/obras?titulo=Otra&limite=5",
    );

    fireEvent.click(await screen.findByRole("button", { name: /Otra Novela/ }));
    expect(
      screen
        .getAllByRole("radio")
        .every((r) => !(r as HTMLInputElement).checked),
    ).toBe(true);
    const comparacion = screen.getByRole("group", {
      name: "Comparación campo por campo",
    });
    expect(within(comparacion).getByText("2020")).not.toBeNull();

    escribirNota("obra correcta");
    fireEvent.click(unir());
    await waitFor(() => expect(llamadas.onExito).toHaveBeenCalledTimes(1));
    expect(cuerpo).toMatchObject({ decision: "asignar", obra_id: "obra-9" });
  });

  it("mientras guarda: avisa, dice Guardando… y bloquea los campos", async () => {
    let liberar: (r: Response) => void = () => {};
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>((r) => (liberar = r))),
    );
    const llamadas = montar();
    escribirNota("x");
    fireEvent.click(ninguna());

    await waitFor(() =>
      expect(llamadas.onEnviandoCambia).toHaveBeenCalledWith(true),
    );
    expect(screen.getByRole("button", { name: "Guardando…" })).not.toBeNull();
    expect(
      (screen.getByLabelText("Nota para la bitácora") as HTMLInputElement)
        .disabled,
    ).toBe(true);
    liberar(json({}));
    await waitFor(() =>
      expect(llamadas.onEnviandoCambia).toHaveBeenCalledWith(false),
    );
  });

  it("un error de red avisa, conserva la nota y devuelve el caso a la cola", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new TypeError("Failed to fetch");
      }),
    );
    const llamadas = montar();
    escribirNota("mi nota");
    fireEvent.click(ninguna());

    expect(
      await screen.findByText(
        "No pudimos guardar la decisión. Revisa tu conexión e inténtalo de nuevo; tu nota sigue aquí.",
      ),
    ).not.toBeNull();
    expect(llamadas.onFalla).toHaveBeenCalledWith(true);
    expect(
      (screen.getByLabelText("Nota para la bitácora") as HTMLInputElement)
        .value,
    ).toBe("mi nota");
  });

  it('409 "ya resuelto": ofrece recargar y el caso no vuelve a la cola', async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        json({ error: "otra persona ya resolvio este caso" }, 409),
      ),
    );
    const llamadas = montar();
    escribirNota("x");
    fireEvent.click(ninguna());

    expect(
      await screen.findByText("Otra persona resolvió este caso antes"),
    ).not.toBeNull();
    expect(llamadas.onFalla).toHaveBeenCalledWith(false);
    fireEvent.click(screen.getByRole("button", { name: "Recargar caso" }));
    expect(llamadas.onRecargarTodo).toHaveBeenCalledTimes(1);
  });

  it("sin candidatas lo dice y deja buscar o descartar", () => {
    montar({}, { ...caso, candidatos: [] });
    expect(
      screen.getByText("No hay obras parecidas en el catálogo."),
    ).not.toBeNull();
    expect(screen.queryAllByRole("radio")).toHaveLength(0);
    expect(
      screen.getByRole("button", { name: "Buscar otra obra" }),
    ).not.toBeNull();
  });

  it("el pie dice, con verdad, que la decision queda en la bitacora y no se deshace", () => {
    montar();
    expect(
      screen.getByText(
        "La decisión queda en la bitácora con tu nota y no se puede deshacer.",
      ),
    ).not.toBeNull();
  });
});
