import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ApiError, ErrorDeRed } from "../api";
import PanelResultado, {
  type Entrega,
  pudoHaberLlegado,
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

// El id de un rechazo es `rep-<64 hex de la huella>-<n>`, con `n` contando
// TODAS las filas parseadas del lote -las aceptadas incluidas- y no los
// rechazos (internal/aplicacion/ingesta.go), ni la linea de la hoja. Por eso
// los ids de estos fixtures son largos y su sufijo no tiene por que coincidir
// con el "fila N" del motivo: es justo el malentendido que la columna "Id"
// evita.
const parcial: Entrega = {
  ...sinRechazos,
  aceptados: 58,
  rechazados: [
    {
      id: `rep-${SHA}-1`,
      titulo: "Obra sintetica A",
      // Formato de ids_fuente (ADR 0018): una linea `clave=valor` por id.
      ids_fuente: "id_ficha=F-001\nimdb=tt0000001",
      motivo:
        'fila 6, duracion_min (columna "Duracion_total"): "cuarenta y cinco" no es un numero',
    },
    {
      id: `rep-${SHA}-4`,
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

const PUDO_LLEGAR =
  "La entrega pudo haber llegado al servidor: revisa el listado de cargas antes de volver a subirla.";

// Texto literal del 409 de evidencia corrupta: lo levanta
// internal/infraestructura/httpapi/reportes.go cuando la boveda tiene bytes
// distintos bajo la huella. Se copia aqui a proposito: si el backend lo
// reformula, este test avisa de que la UI lo estaba mostrando.
const MENSAJE_EVIDENCIA_CORRUPTA =
  "la boveda ya tiene contenido distinto bajo esa huella; avise a operacion";

// Pagina de error de nginx: no es un mensaje de la API y no se debe pintar.
const HTML_DEL_PROXY =
  "<html><head><title>504 Gateway Time-out</title></head><body><h1>504 Gateway Time-out</h1><hr><center>nginx</center></body></html>";

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
    // "Id" y no "Fila": la primera columna trae el id interno del rechazo, que
    // no es la linea de la hoja (el motivo la nombra cuando la sabe).
    expect(encabezados).toEqual(["Id", "Título", "IDs de la fuente", "Motivo"]);
    const filasDeDatos = within(tabla).getAllByRole("row").slice(1);
    expect(filasDeDatos).toHaveLength(2);
    // El id se pinta entero, con sus 64 hex, para poder copiarlo al reportar.
    for (const rechazo of parcial.rechazados) {
      expect(
        within(tabla)
          .getByText(rechazo.id)
          .classList.contains("tabla-rechazos-id"),
      ).toBe(true);
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
    // Un solo elemento con el mensaje entero: ni partido ni reescrito; partirlo
    // acoplaria la UI a la prosa de Go.
    expect(within(alerta).getByText(MENSAJE_400).textContent).toBe(MENSAJE_400);
    expect(within(alerta).getByText(NO_SE_GUARDO)).toBeTruthy();
  });

  // Estos status si cierran la pregunta de si quedo algo escrito -un 4xx prueba
  // que ESA peticion no registro nada, y el 503 de esta ruta ni entra, porque lo
  // produce una guarda previa al handler-, asi que su titulo puede afirmar el
  // no-registro y no hace falta el aviso. El 500 NO entra aqui: tiene su caso
  // propio debajo, porque es el unico que deja la pregunta abierta.
  //
  // El 403 entra en la lista por lo mismo, y con titulo propio: `requiereRol` lo
  // responde ANTES de que `subirReporte` corra, asi que no se escribio nada. Con
  // el cajon neutro ("no se sabe si la entrega se registro") el panel afirmaba de
  // menos, y en la direccion que hace dudar al operador.
  it.each([
    [403, "La sesión no tiene permiso para registrar entregas"],
    [409, "La entrega no se registró"],
    [413, "El archivo es demasiado grande"],
    [503, "La ingesta no está disponible en esta instalación"],
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
      // Contestado, y sin duda de si la entrega llego.
      expect(screen.queryByText(PUDO_LLEGAR)).toBeNull();
    },
  );

  // El 500 no afirma el no-registro, y ademas lleva el aviso. Que las dos
  // escrituras del caso de uso vayan en UNA transaccion
  // (internal/aplicacion/ingesta.go, `GuardarEntrega`) y que las ramas
  // anteriores no dejen fila dice como se escriben las filas, no como acaba el
  // COMMIT: si el enlace con Postgres se pierde despues de mandarlo, el cliente
  // no puede saber si entro -el motor tiene estados propios para eso
  // (`08007 transaction_resolution_unknown`, `40003`)- y `Store.EnTransaccion`
  // convierte ese fallo en el mismo 500 que un fallo limpio
  // (`internal/infraestructura/postgres/store.go`). El mensaje del backend va
  // entero debajo; lo que no se hace es prometer lo que no se sabe.
  it("un 500 no afirma que la entrega no se registró y conserva el aviso", () => {
    const mensaje = "no se pudo registrar la entrega";
    render(
      <PanelResultado resultado={{ tipo: "fallo", status: 500, mensaje }} />,
    );

    const alerta = screen.getByRole("alert");
    expect(
      within(alerta).getByRole("heading", {
        name: "No se sabe si la entrega se registró",
      }),
    ).toBeTruthy();
    // El mensaje del backend va tal cual: es lo que el servidor contesto.
    expect(within(alerta).getByText(mensaje)).toBeTruthy();
    expect(within(alerta).getByText(PUDO_LLEGAR)).toBeTruthy();
    expect(within(alerta).queryByText(NO_SE_GUARDO)).toBeNull();
  });

  // Un 5xx que nadie clasifico tampoco puede afirmar el no-registro. Hoy el
  // handler de esta ruta solo produce el 500 entre los 5xx, pero el predicado no
  // lo enumera: generaliza a 5xx, porque el precio de decir "pudo haber llegado"
  // de mas es el lado seguro. El 503 queda fuera a proposito: lo emite
  // `conIngesta`, una guarda PREVIA al handler.
  it("un 5xx sin titulo propio cae en el cajon neutro y lleva el aviso", () => {
    render(
      <PanelResultado
        resultado={{ tipo: "fallo", status: 501, mensaje: "no implementado" }}
      />,
    );

    const alerta = screen.getByRole("alert");
    expect(
      within(alerta).getByRole("heading", {
        name: "No se sabe si la entrega se registró",
      }),
    ).toBeTruthy();
    expect(within(alerta).getByText(PUDO_LLEGAR)).toBeTruthy();
    expect(within(alerta).queryByText(NO_SE_GUARDO)).toBeNull();
  });

  // La duda es UNA definicion y de ella dependen las dos mitades de la pantalla:
  // el aviso que se pinta y el listado que se vuelve a pedir. Se prueba suelta
  // porque es lo unico que no puede divergir entre las dos.
  it.each([
    ["red", true],
    ["desconocido", true],
    [500, true],
    [501, true],
    [502, true],
    [504, true],
    [503, false],
    [400, false],
    [403, false],
    [409, false],
    [413, false],
  ] as const)("pudoHaberLlegado(%s) = %s", (status, quiere) => {
    expect(pudoHaberLlegado(status)).toBe(quiere);
  });

  // El 409 de evidencia corrupta entra por el mismo status que el duplicado,
  // asi que el titulo tiene que dejar hablar al mensaje del backend.
  it("un 409 de evidencia corrupta conserva su mensaje y no lo titula duplicado", () => {
    render(
      <PanelResultado
        resultado={{
          tipo: "fallo",
          status: 409,
          mensaje: MENSAJE_EVIDENCIA_CORRUPTA,
        }}
      />,
    );

    const alerta = screen.getByRole("alert");
    expect(
      within(alerta).getByRole("heading", {
        name: "La entrega no se registró",
      }),
    ).toBeTruthy();
    // El mensaje de la API llega entero: es lo que dice que hay que avisar a
    // operacion, y no un "ya se habia cargado" inofensivo.
    expect(within(alerta).getByText(MENSAJE_EVIDENCIA_CORRUPTA)).toBeTruthy();
    expect(within(alerta).queryByText(/ya se había cargado/i)).toBeNull();
    // Con un 409 el servidor contesto: no hay duda de si la entrega llego.
    expect(within(alerta).queryByText(PUDO_LLEGAR)).toBeNull();
  });

  it.each([502, 504] as const)(
    "un %s del proxy no pinta su cuerpo y avisa que la entrega pudo llegar",
    (status) => {
      render(
        <PanelResultado
          resultado={resultadoDeError(new ApiError(status, HTML_DEL_PROXY))}
        />,
      );

      const alerta = screen.getByRole("alert");
      expect(
        within(alerta).getByRole("heading", {
          name: "No se sabe si la entrega se registró",
        }),
      ).toBeTruthy();
      // Un 502/504 no se distingue de cualquier otro fallo no clasificable:
      // lleva el mensaje generico, no la pagina del proxy. (Que el HTML del
      // proxy no llegue hasta aqui lo garantiza `mensajeDeError` en api.ts; el
      // mapeo de este status existe para que el aviso de abajo no se pierda.)
      expect(
        within(alerta).getByText("error desconocido al subir el archivo"),
      ).toBeTruthy();
      expect(within(alerta).getByText(PUDO_LLEGAR)).toBeTruthy();
      // Ni el HTML del proxy ni su titulo llegan al documento: no son la
      // explicacion del backend.
      expect(document.body.textContent).not.toContain("Gateway Time-out");
      expect(document.body.textContent).not.toContain("nginx");
      // El "no se guardo nada" es solo del 400: aqui pudo haber entrado.
      expect(within(alerta).queryByText(NO_SE_GUARDO)).toBeNull();
    },
  );

  // Sin respuesta legible no se sabe si la entrega quedo registrada, y
  // reintentar a ciegas daria 409 si llego.
  it("sin red avisa que la entrega pudo llegar y no repite el titulo", () => {
    const red = new ErrorDeRed(new TypeError("Failed to fetch"));
    render(<PanelResultado resultado={resultadoDeError(red)} />);

    const alerta = screen.getByRole("alert");
    expect(
      within(alerta).getByRole("heading", {
        name: "No se pudo contactar al servidor",
      }),
    ).toBeTruthy();
    expect(within(alerta).getByText(PUDO_LLEGAR)).toBeTruthy();
    // El mensaje de ErrorDeRed dice lo mismo que el titulo: va una sola vez.
    expect(
      within(alerta).getAllByText(/no se pudo contactar al servidor/i),
    ).toHaveLength(1);
    expect(screen.queryByText(NO_SE_GUARDO)).toBeNull();
  });

  it("un error desconocido tiene un titulo neutro, un mensaje generico y el mismo aviso", () => {
    render(<PanelResultado resultado={resultadoDeError(new Error("boom"))} />);

    const alerta = screen.getByRole("alert");
    expect(
      within(alerta).getByRole("heading", {
        name: "No se sabe si la entrega se registró",
      }),
    ).toBeTruthy();
    // El texto interno ("boom") no es un mensaje del backend y no se pinta.
    expect(
      within(alerta).getByText("error desconocido al subir el archivo"),
    ).toBeTruthy();
    expect(within(alerta).queryByText("boom")).toBeNull();
    expect(within(alerta).getByText(PUDO_LLEGAR)).toBeTruthy();
    expect(within(alerta).queryByText(/contactar al servidor/i)).toBeNull();
  });
});

describe("resultadoDeError", () => {
  it("un ApiError conserva su status y el mensaje del backend", () => {
    expect(resultadoDeError(new ApiError(409, "ya se habia cargado"))).toEqual({
      tipo: "fallo",
      status: 409,
      mensaje: "ya se habia cargado",
    });
  });

  it.each([502, 504] as const)(
    "un %s del proxy queda como desconocido y descarta su mensaje",
    (status) => {
      expect(resultadoDeError(new ApiError(status, HTML_DEL_PROXY))).toEqual({
        tipo: "fallo",
        status: "desconocido",
        mensaje: "error desconocido al subir el archivo",
      });
    },
  );

  it("un ErrorDeRed queda marcado como fallo de red", () => {
    const red = new ErrorDeRed(new TypeError("Failed to fetch"));
    expect(resultadoDeError(red)).toEqual({
      tipo: "fallo",
      status: "red",
      mensaje: red.message,
    });
  });

  it("cualquier otro error queda como desconocido, con un mensaje generico", () => {
    expect(resultadoDeError(new Error("boom"))).toEqual({
      tipo: "fallo",
      status: "desconocido",
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

  // El 201 trae el log entero en el cuerpo, asi que un archivo con la cabecera
  // equivocada -todas sus filas rechazadas- llega hasta aqui con cientos de
  // miles de elementos: un `<tr>` por fila congela la pestana. La pintura se
  // acota, y lo que NO se acota es la cifra: se dice cuantas hay en total y el
  // resto queda a un clic. Recortar sin decirlo seria esconder filas.
  it("con mas rechazos de los que caben pinta los primeros y dice cuantos hay", () => {
    const muchos = Array.from({ length: 250 }, (_, i) => ({
      id: `rep-${SHA}-${i}`,
      titulo: `Obra sintetica ${i}`,
      ids_fuente: `id_ficha=F-${i}`,
      motivo: `fila ${i}, titulo (columna "Titulo"): vacio`,
    }));
    render(<TablaRechazos rechazos={muchos} />);

    const tabla = screen.getByRole("table", { name: "Filas rechazadas" });
    // Cien filas pintadas, ni una mas: es el tope de la pintura.
    expect(within(tabla).getAllByRole("row").slice(1)).toHaveLength(100);
    // Y la cifra entera dicha en voz alta, con el resto a un clic.
    expect(screen.getByText(/Mostrando 100 de 250 rechazos\./)).toBeTruthy();
    const verMas = screen.getByRole("button", { name: "Ver 100 más" });
    expect(within(tabla).queryByText(muchos[100].motivo)).toBeNull();

    fireEvent.click(verMas);

    expect(within(tabla).getAllByRole("row").slice(1)).toHaveLength(200);
    expect(within(tabla).getByText(muchos[100].motivo)).toBeTruthy();
    // En el ultimo tramo el boton no promete mas de lo que queda: quedan 50.
    expect(screen.getByRole("button", { name: "Ver 50 más" })).toBeTruthy();
  });

  // El panel de la subida es el consumidor que lo necesita: el 201 trae el log
  // en el cuerpo y no hay endpoint que paginar.
  it("el panel acota la pintura del log que vino en el 201", () => {
    const muchos = Array.from({ length: 150 }, (_, i) => ({
      id: `rep-${SHA}-${i}`,
      titulo: `Obra sintetica ${i}`,
      ids_fuente: `id_ficha=F-${i}`,
      motivo: `fila ${i}, titulo (columna "Titulo"): vacio`,
    }));
    render(
      <PanelResultado
        resultado={{
          tipo: "entrega",
          entrega: { ...sinRechazos, rechazados: muchos },
        }}
      />,
    );

    // El recuento del panel es el del log entero, no el de las filas pintadas.
    expect(dato("Filas rechazadas")).toBe("150");
    const tabla = screen.getByRole("table", { name: "Filas rechazadas" });
    expect(within(tabla).getAllByRole("row").slice(1)).toHaveLength(100);
  });
});
