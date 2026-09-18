import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ApiError, ErrorDeRed } from "../api";
import PanelResultado, {
  type Entrega,
  resultadoDeError,
} from "./PanelResultado";
import TablaRechazos from "./TablaRechazos";

// Fixtures tipados con el contrato generado: si `Entrega` o `Rechazo` cambian
// en api/openapi.yaml, `tsc` rompe aqui antes que en pantalla. Datos
// sinteticos, no de ningun canal real.
const SHA = "9f2c4e1a7b3d5f60a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718";

const sinRechazos: Entrega = {
  id: "rep-9f2c4e1a",
  fuente: "caracol",
  periodo: "2026-01",
  sha256: SHA,
  clave_objeto: "reportes/9f2c4e1a",
  nbytes: 21032,
  aceptados: 1234,
  rechazados: [],
};

const parcial: Entrega = {
  ...sinRechazos,
  aceptados: 58,
  rechazados: [
    {
      id: "rep-9f2c4e1a-6",
      titulo: "Obra sintetica A",
      // Formato de ids_fuente (ADR 0018): una linea `clave=valor` por id.
      ids_fuente: "id_ficha=F-001\nimdb=tt0000001",
      motivo:
        'fila 6, duracion_min (columna "Duracion_total"): "cuarenta y cinco" no es un numero',
    },
    {
      id: "rep-9f2c4e1a-9",
      titulo: "Obra sintetica B",
      ids_fuente: "id_ficha=F-002",
      motivo: 'fila 9, titulo (columna "Titulo"): vacio',
    },
  ],
};

// Ejemplo del 400 en api/openapi.yaml: un unico string que nombra columnas.
const MENSAJE_400 =
  'reporte invalido: a la entrega de "caracol" le faltan columnas requeridas: Duracion_total. El archivo trae: Canal, Titulo, ID_Ficha';

const NO_SE_GUARDO =
  "No se guardó nada: corrige el archivo y vuelve a subirlo.";

/** El valor que acompana a una etiqueta del resumen de la entrega. */
function dato(etiqueta: string): string | null | undefined {
  return screen.getByText(etiqueta).nextElementSibling?.textContent;
}

describe("PanelResultado", () => {
  afterEach(() => {
    cleanup();
  });

  it("una entrega sin rechazos muestra los recuentos y ninguna tabla", () => {
    render(
      <PanelResultado resultado={{ tipo: "entrega", entrega: sinRechazos }} />,
    );

    expect(
      screen.getByRole("heading", { name: "Entrega registrada" }),
    ).toBeTruthy();
    expect(dato("Filas aceptadas")).toBe("1.234");
    expect(dato("Filas rechazadas")).toBe("0");
    expect(dato("Fuente")).toBe("caracol");
    expect(dato("Periodo")).toBe("2026-01");
    expect(screen.getByTitle(SHA).textContent).toBe(SHA.slice(0, 12));
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("una entrega parcial lista cada rechazo con su motivo y sin medidas", () => {
    render(
      <PanelResultado resultado={{ tipo: "entrega", entrega: parcial }} />,
    );

    expect(dato("Filas aceptadas")).toBe("58");
    expect(dato("Filas rechazadas")).toBe("2");
    expect(
      screen.getByText(
        "Las filas rechazadas quedaron en el log de rechazos con su motivo; ninguna se descartó.",
      ),
    ).toBeTruthy();

    const tabla = screen.getByRole("table", { name: "Filas rechazadas" });
    const encabezados = within(tabla)
      .getAllByRole("columnheader")
      .map((th) => th.textContent);
    expect(encabezados).toEqual([
      "Fila",
      "Título",
      "IDs de la fuente",
      "Motivo",
    ]);
    const filasDeDatos = within(tabla).getAllByRole("row").slice(1);
    expect(filasDeDatos).toHaveLength(2);
    for (const rechazo of parcial.rechazados) {
      expect(within(tabla).getByText(rechazo.motivo)).toBeTruthy();
    }
  });

  it("un 400 muestra el mensaje del backend tal cual y avisa que no se guardo nada", () => {
    render(
      <PanelResultado
        resultado={{ tipo: "fallo", status: 400, mensaje: MENSAJE_400 }}
      />,
    );

    const alerta = screen.getByRole("alert");
    expect(
      within(alerta).getByRole("heading", {
        name: "La entrega no cumple la estructura mínima",
      }),
    ).toBeTruthy();
    // Un solo elemento con el mensaje entero: ni partido ni reescrito (D-006).
    expect(within(alerta).getByText(MENSAJE_400).textContent).toBe(MENSAJE_400);
    expect(within(alerta).getByText(NO_SE_GUARDO)).toBeTruthy();
  });

  it.each([
    [409, "Ese archivo ya se había cargado"],
    [413, "El archivo es demasiado grande"],
    [503, "La ingesta no está disponible en esta instalación"],
    [null, "No se pudo contactar al servidor"],
    [500, "No se pudo registrar la entrega"],
  ] as const)(
    "el status %s se titula %j y conserva el mensaje",
    (status, titulo) => {
      const mensaje = "mensaje del servidor";
      render(<PanelResultado resultado={{ tipo: "fallo", status, mensaje }} />);

      const alerta = screen.getByRole("alert");
      expect(
        within(alerta).getByRole("heading", { name: titulo }),
      ).toBeTruthy();
      expect(within(alerta).getByText(mensaje)).toBeTruthy();
      // El aviso de "no se guardo nada" es solo del 400.
      expect(screen.queryByText(NO_SE_GUARDO)).toBeNull();
    },
  );
});

describe("resultadoDeError", () => {
  it("un ApiError conserva su status y el mensaje del backend", () => {
    expect(resultadoDeError(new ApiError(409, "ya se habia cargado"))).toEqual({
      tipo: "fallo",
      status: 409,
      mensaje: "ya se habia cargado",
    });
  });

  it("un ErrorDeRed queda sin status", () => {
    const red = new ErrorDeRed(new TypeError("Failed to fetch"));
    expect(resultadoDeError(red)).toEqual({
      tipo: "fallo",
      status: null,
      mensaje: red.message,
    });
  });

  it("cualquier otro error da un mensaje generico sin status", () => {
    expect(resultadoDeError(new Error("boom"))).toEqual({
      tipo: "fallo",
      status: null,
      mensaje: "error desconocido al subir el archivo",
    });
  });
});

describe("TablaRechazos", () => {
  afterEach(() => {
    cleanup();
  });

  it("sin rechazos dice que no hay y no pinta la tabla", () => {
    render(<TablaRechazos rechazos={[]} />);

    expect(screen.getByText("No hay filas rechazadas.")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });
});
