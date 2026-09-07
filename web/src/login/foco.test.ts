import { describe, expect, it } from "vitest";
import { esCampoDeTexto } from "./foco";

describe("esCampoDeTexto", () => {
  it("reconoce los tres campos de formulario", () => {
    for (const etiqueta of ["input", "textarea", "select"] as const) {
      expect(esCampoDeTexto(document.createElement(etiqueta))).toBe(true);
    }
  });

  it("reconoce un elemento editable que no es un campo", () => {
    // Un div con contenteditable recibe texto y ahi las flechas mueven el
    // cursor: robarlas seria el mismo defecto que sobre un <input>.
    const div = document.createElement("div");
    div.setAttribute("contenteditable", "true");
    document.body.append(div);
    expect(esCampoDeTexto(div)).toBe(true);
    div.remove();
  });

  it("deja pasar lo que no recibe texto", () => {
    for (const etiqueta of ["div", "button", "canvas", "a"] as const) {
      expect(esCampoDeTexto(document.createElement(etiqueta))).toBe(false);
    }
  });

  it("deja pasar null", () => {
    // `document.activeElement` es null si el documento no tiene foco.
    expect(esCampoDeTexto(null)).toBe(false);
  });

  it("reconoce un hijo dentro de un elemento editable", () => {
    // Herencia: el span no lleva atributo propio y aun asi recibe texto.
    const div = document.createElement("div");
    div.setAttribute("contenteditable", "true");
    const span = document.createElement("span");
    div.append(span);
    document.body.append(div);
    expect(esCampoDeTexto(span)).toBe(true);
    div.remove();
  });

  it("no confunde un subarbol con la edicion apagada", () => {
    const div = document.createElement("div");
    div.setAttribute("contenteditable", "false");
    document.body.append(div);
    expect(esCampoDeTexto(div)).toBe(false);
    div.remove();
  });
});
