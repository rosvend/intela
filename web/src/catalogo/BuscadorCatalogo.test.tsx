import { useState } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import BuscadorCatalogo from "./BuscadorCatalogo";
import type { CategoriaId } from "./categoriasDeBusqueda";

type Filtros = Record<CategoriaId, string>;
const SIN_FILTROS: Filtros = { titulo: "", genero: "", ipi: "", anio: "" };

/**
 * Hace de `Catalogo`: dueno de los filtros, como lo es de la URL. Los callbacks
 * espian y ademas actualizan el estado, porque el buscador se resincroniza con
 * lo que le llega en `filtros`.
 */
function montar(inicial: Partial<Filtros> = {}) {
  const onAplicar = vi.fn();
  const onQuitar = vi.fn();
  const onLimpiar = vi.fn();
  let poner: (f: Filtros) => void = () => {};
  function Anfitrion() {
    const [filtros, setFiltros] = useState<Filtros>({
      ...SIN_FILTROS,
      ...inicial,
    });
    poner = setFiltros;
    return (
      <>
        <BuscadorCatalogo
          filtros={filtros}
          onAplicar={(categoria, valor) => {
            onAplicar(categoria, valor);
            setFiltros((f) => ({ ...f, [categoria]: valor }));
          }}
          onQuitar={(categoria) => {
            onQuitar(categoria);
            setFiltros((f) => ({ ...f, [categoria]: "" }));
          }}
          onLimpiar={() => {
            onLimpiar();
            setFiltros(SIN_FILTROS);
          }}
        />
        <button type="button">Fuera</button>
      </>
    );
  }
  render(<Anfitrion />);
  return {
    onAplicar,
    onQuitar,
    onLimpiar,
    desdeFuera: (f: Partial<Filtros>) => poner({ ...SIN_FILTROS, ...f }),
  };
}

const selector = () => screen.getByRole("combobox", { name: "Buscar por" });
const campo = () => screen.getByRole("textbox", { name: "Texto de búsqueda" });
const lupa = () => screen.getByRole("button", { name: "Buscar" });
const tecla = (elemento: HTMLElement, key: string) =>
  fireEvent.keyDown(elemento, { key });

function elegir(etiqueta: string) {
  fireEvent.click(selector());
  fireEvent.click(screen.getByRole("option", { name: etiqueta }));
}

function opcionActiva(): string | null {
  const id = selector().getAttribute("aria-activedescendant");
  return id ? (document.getElementById(id)?.textContent ?? null) : null;
}

afterEach(() => cleanup());

describe("selector de categoría", () => {
  it("es un combobox cerrado que muestra la categoría por defecto", () => {
    montar();

    expect(selector().textContent).toBe("Título");
    expect(selector().getAttribute("aria-haspopup")).toBe("listbox");
    expect(selector().getAttribute("aria-expanded")).toBe("false");
    expect(selector().getAttribute("aria-activedescendant")).toBeNull();
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("al abrirlo expone un listbox con las cuatro opciones y la activa seleccionada", () => {
    montar();

    fireEvent.click(selector());

    expect(selector().getAttribute("aria-expanded")).toBe("true");
    const lista = screen.getByRole("listbox");
    expect(selector().getAttribute("aria-controls")).toBe(lista.id);
    const opciones = screen.getAllByRole("option");
    expect(opciones.map((o) => o.textContent)).toEqual([
      "Título",
      "Género",
      "IPI de coautor",
      "Año",
    ]);
    expect(opciones.map((o) => o.getAttribute("aria-selected"))).toEqual([
      "true",
      "false",
      "false",
      "false",
    ]);
    expect(opcionActiva()).toBe("Título");
  });

  it("los iconos no se anuncian", () => {
    montar();
    fireEvent.click(selector());

    for (const svg of document.querySelectorAll("svg")) {
      expect(svg.getAttribute("aria-hidden")).toBe("true");
      expect(svg.getAttribute("focusable")).toBe("false");
    }
  });

  it.each(["ArrowDown", "ArrowUp", "Enter", " "])(
    "cerrado, %j abre el menú con la opción actual resaltada",
    (key) => {
      montar();
      elegir("Género");
      // `elegir` deja el menú cerrado y Género seleccionada.
      expect(selector().getAttribute("aria-expanded")).toBe("false");

      tecla(selector(), key);

      expect(selector().getAttribute("aria-expanded")).toBe("true");
      expect(opcionActiva()).toBe("Género");
    },
  );

  it("abierto, las flechas mueven sin dar la vuelta", () => {
    montar();
    tecla(selector(), "ArrowDown");

    tecla(selector(), "ArrowUp");
    expect(opcionActiva()).toBe("Título");

    tecla(selector(), "ArrowDown");
    expect(opcionActiva()).toBe("Género");
    tecla(selector(), "ArrowDown");
    tecla(selector(), "ArrowDown");
    expect(opcionActiva()).toBe("Año");
    tecla(selector(), "ArrowDown");
    expect(opcionActiva()).toBe("Año");
  });

  it("abierto, Home y End van a los extremos", () => {
    montar();
    tecla(selector(), "ArrowDown");

    tecla(selector(), "End");
    expect(opcionActiva()).toBe("Año");
    tecla(selector(), "Home");
    expect(opcionActiva()).toBe("Título");
  });

  it("la opción resaltada por teclado se distingue de la seleccionada", () => {
    montar();
    tecla(selector(), "ArrowDown");
    tecla(selector(), "ArrowDown");

    const [titulo, genero] = screen.getAllByRole("option");
    expect(genero.className).toContain("buscador-opcion-resaltada");
    expect(titulo.className).not.toContain("buscador-opcion-resaltada");
    expect(titulo.getAttribute("aria-selected")).toBe("true");
  });

  it.each(["Enter", " "])(
    "abierto, %j elige la resaltada, cierra y pasa el foco al campo",
    (key) => {
      montar();
      tecla(selector(), "ArrowDown");
      tecla(selector(), "ArrowDown");

      tecla(selector(), key);

      expect(selector().textContent).toBe("Género");
      expect(selector().getAttribute("aria-expanded")).toBe("false");
      expect(document.activeElement).toBe(campo());
      expect(campo().getAttribute("placeholder")).toBe(
        "Género exacto, p. ej. Drama",
      );
    },
  );

  it("abierto, Escape cierra sin cambiar la categoría", () => {
    montar();
    tecla(selector(), "ArrowDown");
    tecla(selector(), "ArrowDown");

    tecla(selector(), "Escape");

    expect(selector().getAttribute("aria-expanded")).toBe("false");
    expect(selector().textContent).toBe("Título");
    expect(document.activeElement).not.toBe(campo());
  });

  it("abierto, Tab elige la resaltada y cierra", () => {
    montar();
    tecla(selector(), "ArrowDown");
    tecla(selector(), "End");

    tecla(selector(), "Tab");

    expect(selector().textContent).toBe("Año");
    expect(selector().getAttribute("aria-expanded")).toBe("false");
  });

  it("un clic en una opción la elige, cierra y pasa el foco al campo", () => {
    montar();
    fireEvent.click(selector());

    fireEvent.click(screen.getByRole("option", { name: "IPI de coautor" }));

    expect(selector().textContent).toBe("IPI de coautor");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(document.activeElement).toBe(campo());
    expect(campo().getAttribute("placeholder")).toBe("IPI exacto del coautor");
  });

  it("un clic en el botón abre y vuelve a cerrar", () => {
    montar();

    fireEvent.click(selector());
    expect(selector().getAttribute("aria-expanded")).toBe("true");
    fireEvent.click(selector());
    expect(selector().getAttribute("aria-expanded")).toBe("false");
  });

  it("un clic fuera cierra sin cambiar, y un clic dentro del menú no", () => {
    montar();
    fireEvent.click(selector());

    fireEvent.mouseDown(screen.getByRole("listbox"));
    expect(selector().getAttribute("aria-expanded")).toBe("true");

    fireEvent.mouseDown(screen.getByRole("button", { name: "Fuera" }));
    expect(selector().getAttribute("aria-expanded")).toBe("false");
    expect(selector().textContent).toBe("Título");
  });

  it("cada categoría pone su placeholder y su inputMode", () => {
    montar();

    expect(campo().getAttribute("placeholder")).toBe(
      "Título o parte, sin importar tildes…",
    );
    expect(campo().getAttribute("inputmode")).toBe("text");
    elegir("Año");
    expect(campo().getAttribute("placeholder")).toBe("Año exacto, p. ej. 1991");
    expect(campo().getAttribute("inputmode")).toBe("numeric");
  });
});

describe("campo y chips", () => {
  it("Título llama a onAplicar en cada cambio", () => {
    const { onAplicar } = montar();

    fireEvent.change(campo(), { target: { value: "C" } });
    fireEvent.change(campo(), { target: { value: "Ca" } });

    expect(onAplicar.mock.calls).toEqual([
      ["titulo", "C"],
      ["titulo", "Ca"],
    ]);
    expect(campo()).toHaveProperty("value", "Ca");
  });

  it.each([
    ["Género", "genero", "  Drama  ", "Drama"],
    ["IPI de coautor", "ipi", " IPI-00000001 ", "IPI-00000001"],
    ["Año", "anio", " 2024 ", "2024"],
  ])(
    "%s no aplica al escribir; sí con Enter y con la lupa, recortado",
    (etiqueta, id, escrito, esperado) => {
      const { onAplicar } = montar();
      elegir(etiqueta);

      fireEvent.change(campo(), { target: { value: escrito } });
      expect(onAplicar).not.toHaveBeenCalled();

      tecla(campo(), "Enter");
      expect(onAplicar).toHaveBeenLastCalledWith(id, esperado);
      // El valor queda en su chip y el campo se vacía.
      expect(campo()).toHaveProperty("value", "");
      expect(screen.getByText(`${etiqueta}: ${esperado}`)).toBeTruthy();

      fireEvent.change(campo(), { target: { value: escrito } });
      fireEvent.click(lupa());
      expect(onAplicar).toHaveBeenCalledTimes(2);
      expect(onAplicar).toHaveBeenLastCalledWith(id, esperado);
    },
  );

  it("Enter y la lupa con el campo vacío no hacen nada, ni con solo espacios", () => {
    const { onAplicar, onQuitar } = montar({ genero: "Drama" });
    elegir("Año");

    tecla(campo(), "Enter");
    fireEvent.click(lupa());
    fireEvent.change(campo(), { target: { value: "   " } });
    tecla(campo(), "Enter");

    expect(onAplicar).not.toHaveBeenCalled();
    expect(onQuitar).not.toHaveBeenCalled();
    expect(screen.getByText("Género: Drama")).toBeTruthy();
  });

  it("Enter con el campo vacío no quita el filtro que la categoría ya tenía", () => {
    const { onAplicar, onQuitar } = montar({ genero: "Drama" });
    elegir("Género");
    fireEvent.change(campo(), { target: { value: "" } });

    tecla(campo(), "Enter");

    expect(onAplicar).not.toHaveBeenCalled();
    expect(onQuitar).not.toHaveBeenCalled();
    expect(screen.getByText("Género: Drama")).toBeTruthy();
  });

  it("un año inválido no se aplica, se explica, y el aviso se va al volver a escribir", () => {
    const { onAplicar } = montar();
    elegir("Año");

    fireEvent.change(campo(), { target: { value: "19a1" } });
    tecla(campo(), "Enter");

    expect(onAplicar).not.toHaveBeenCalled();
    const aviso = screen.getByText("Escribe un año entero positivo.");
    expect(campo().getAttribute("aria-invalid")).toBe("true");
    expect(campo().getAttribute("aria-describedby")).toBe(aviso.id);
    // El campo conserva lo tecleado.
    expect(campo()).toHaveProperty("value", "19a1");

    fireEvent.change(campo(), { target: { value: "19a" } });

    expect(screen.queryByText("Escribe un año entero positivo.")).toBeNull();
    expect(campo().getAttribute("aria-invalid")).toBeNull();
    expect(campo().getAttribute("aria-describedby")).toBeNull();
  });

  it("un año inválido también se rechaza con la lupa", () => {
    const { onAplicar } = montar();
    elegir("Año");

    fireEvent.change(campo(), { target: { value: "0" } });
    fireEvent.click(lupa());

    expect(onAplicar).not.toHaveBeenCalled();
    expect(screen.getByText("Escribe un año entero positivo.")).toBeTruthy();
  });

  it("no hay chips sin filtros; con filtros hay uno por categoría, en orden fijo", () => {
    montar();
    expect(
      screen.queryByRole("list", { name: "Filtros aplicados" }),
    ).toBeNull();
    cleanup();

    montar({ anio: "2024", titulo: "casa", genero: "Drama" });

    const lista = screen.getByRole("list", { name: "Filtros aplicados" });
    const chips = lista.querySelectorAll("li");
    expect(
      [...chips].map((li) => li.querySelector("span")?.textContent),
    ).toEqual(["Título: casa", "Género: Drama", "Año: 2024"]);
    expect(
      screen.getByRole("button", { name: "Quitar el filtro Género" }),
    ).toBeTruthy();
  });

  it("aplicar otro valor en la misma categoría reemplaza su chip, no lo duplica", () => {
    montar({ genero: "Drama" });
    elegir("Género");
    fireEvent.change(campo(), { target: { value: "Comedia" } });

    tecla(campo(), "Enter");

    expect(screen.getByText("Género: Comedia")).toBeTruthy();
    expect(screen.queryByText("Género: Drama")).toBeNull();
    expect(screen.getAllByRole("listitem")).toHaveLength(1);
  });

  it("la x de un chip llama a onQuitar y el foco pasa al chip siguiente", () => {
    const { onQuitar } = montar({
      titulo: "casa",
      genero: "Drama",
      anio: "2024",
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Quitar el filtro Título" }),
    );

    expect(onQuitar).toHaveBeenCalledWith("titulo");
    expect(screen.queryByText("Título: casa")).toBeNull();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Quitar el filtro Género" }),
    );
  });

  it("quitar el último chip deja el foco en el campo", () => {
    const { onQuitar } = montar({ genero: "Drama", anio: "2024" });

    fireEvent.click(
      screen.getByRole("button", { name: "Quitar el filtro Año" }),
    );

    expect(onQuitar).toHaveBeenCalledWith("anio");
    expect(document.activeElement).toBe(campo());
  });

  it("Limpiar filtros solo aparece con chips y llama a onLimpiar", () => {
    montar();
    expect(
      screen.queryByRole("button", { name: "Limpiar filtros" }),
    ).toBeNull();
    cleanup();

    const { onLimpiar } = montar({ genero: "Drama" });
    fireEvent.click(screen.getByRole("button", { name: "Limpiar filtros" }));

    expect(onLimpiar).toHaveBeenCalledTimes(1);
    expect(
      screen.queryByRole("list", { name: "Filtros aplicados" }),
    ).toBeNull();
  });

  it("elegir una categoría con valor precarga el campo; sin valor, lo deja vacío", () => {
    montar({ genero: "Drama" });

    elegir("Género");
    expect(campo()).toHaveProperty("value", "Drama");

    elegir("Año");
    expect(campo()).toHaveProperty("value", "");
  });

  it("un cambio de filtros desde fuera actualiza el campo", () => {
    const { desdeFuera } = montar({ genero: "Drama" });
    elegir("Género");
    expect(campo()).toHaveProperty("value", "Drama");

    // El botón atrás, un enlace o "Limpiar filtros" cambian la URL.
    act(() => desdeFuera({ genero: "Comedia" }));
    expect(campo()).toHaveProperty("value", "Comedia");

    act(() => desdeFuera({}));
    expect(campo()).toHaveProperty("value", "");
  });

  it("en Título el campo es reflejo directo de los filtros", () => {
    const { desdeFuera } = montar({ titulo: "casa" });
    expect(campo()).toHaveProperty("value", "casa");

    act(() => desdeFuera({ titulo: "noche" }));

    expect(campo()).toHaveProperty("value", "noche");
  });

  it("el texto a medias de una categoría exacta no se pierde al cambiar otro filtro", () => {
    const { desdeFuera } = montar();
    elegir("Género");
    fireEvent.change(campo(), { target: { value: "Dra" } });

    act(() => desdeFuera({ anio: "2024" }));

    expect(campo()).toHaveProperty("value", "Dra");
  });
});
