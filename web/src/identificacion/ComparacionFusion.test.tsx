import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import ComparacionFusion from "./ComparacionFusion";
import { sugerenciaNinguna, type CasoIdentificacion } from "./tipos";

const caso = {
  id: "uso-1",
  titulo: "La Niña T3 E12",
  titulo_original: "La niña — Capítulo 12",
  fuente: "caracol",
  modalidad: "tv",
  reporte_id: "ING-1",
  periodo: "2024-11",
  ids_fuente: "ID_Ficha=48213",
  evidencia: "",
  estado: "pendiente",
  candidatos: [],
  obra_asignada: null,
  resuelto_por: null,
  resuelto_en: null,
  ultima_actualizacion: "",
  nota: null,
  sugerencia: sugerenciaNinguna(),
} satisfies CasoIdentificacion;

const obra = { id: "obra-1", titulo: "La Niña", anio: 2016, genero: "Drama" };

afterEach(cleanup);

function celdas(lado: "reportado" | "catalogo"): HTMLElement[] {
  return Array.from(
    screen
      .getByRole("group", { name: "Comparación campo por campo" })
      .querySelectorAll<HTMLElement>(`.fusion-celda-${lado}`),
  );
}

describe("ComparacionFusion", () => {
  it("cabeceras de las dos tarjetas: la fuente que reporto y el catalogo", () => {
    render(<ComparacionFusion caso={caso} obra={null} unida={false} />);
    expect(screen.getByText("Reportado por Caracol TV")).not.toBeNull();
    expect(screen.getByText("Obra del catálogo")).not.toBeNull();
  });

  it("sin obra elegida, el catalogo es un estado vacio y lo reportado se lee completo", () => {
    render(<ComparacionFusion caso={caso} obra={null} unida={false} />);
    expect(
      screen.getByRole("heading", { level: 2, name: "La Niña T3 E12" }),
    ).not.toBeNull();
    expect(screen.getByText("La niña — Capítulo 12")).not.toBeNull();
    expect(screen.getByText("TV")).not.toBeNull();
    expect(screen.getByText("nov 2024")).not.toBeNull();
    expect(screen.getByText("ID_Ficha: 48213")).not.toBeNull();
    expect(
      screen.getByText("Elige una candidata o busca la obra para compararla."),
    ).not.toBeNull();
    expect(celdas("catalogo")).toHaveLength(0);
  });

  it("con obra, las filas se alinean: mismo numero y orden en los dos lados", () => {
    render(<ComparacionFusion caso={caso} obra={obra} unida={false} />);
    const izquierda = celdas("reportado");
    const derecha = celdas("catalogo");
    expect(izquierda).toHaveLength(6);
    expect(derecha).toHaveLength(6);
    expect(izquierda.map((c) => c.dataset.campo)).toEqual(
      derecha.map((c) => c.dataset.campo),
    );
  });

  it("el titulo distinto se resalta del lado reportado y marca las palabras que no estan", () => {
    render(<ComparacionFusion caso={caso} obra={obra} unida={false} />);
    const [titulo] = celdas("reportado");
    expect(titulo?.dataset.estado).toBe("distinto");
    expect(titulo?.classList.contains("fusion-distinta")).toBe(true);
    const marcas = Array.from(titulo!.querySelectorAll("mark")).map(
      (m) => m.textContent,
    );
    expect(marcas).toEqual(["T3 E12", "Capítulo 12"]);
    expect(within(titulo!).getByText(/Distinto/)).not.toBeNull();
  });

  it("lo que un lado no trae se pinta con una raya", () => {
    render(<ComparacionFusion caso={caso} obra={obra} unida={false} />);
    const anio = celdas("reportado").find((c) => c.dataset.campo === "anio");
    expect(anio?.textContent).toContain("—");
    const periodo = celdas("catalogo").find(
      (c) => c.dataset.campo === "periodo",
    );
    expect(periodo?.textContent).toContain("—");
    expect(
      celdas("catalogo").find((c) => c.dataset.campo === "anio")?.textContent,
    ).toContain("2016");
  });

  it("un titulo que coincide lleva un check discreto", () => {
    render(
      <ComparacionFusion
        caso={{ ...caso, titulo: "La Niña" }}
        obra={obra}
        unida={false}
      />,
    );
    const titulo = celdas("catalogo")[0]!;
    expect(titulo.dataset.estado).toBe("coincide");
    expect(within(titulo).getByText("Coincide")).not.toBeNull();
  });

  it("unida: la comparacion entra en el estado de union", () => {
    render(<ComparacionFusion caso={caso} obra={obra} unida />);
    const grupo = screen.getByRole("group", {
      name: "Comparación campo por campo",
    });
    expect(grupo.classList.contains("fusion-unida")).toBe(true);
  });
});
