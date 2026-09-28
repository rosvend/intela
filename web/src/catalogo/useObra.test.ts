import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Obra, VersionDeclaracion } from "./tipos";
import { conciliarConElHistorial, useObra, type EstadoDeObra } from "./useObra";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// Fixtures tipados con el contrato generado: si `Obra` o `VersionDeclaracion`
// cambian en `api/openapi.yaml`, `tsc` rompe aqui antes que en pantalla.
const obraConDeclaracion = {
  id: "obra-1",
  titulo: "La Casa de las Dos Palmas",
  genero: "Drama",
  anio: 1991,
  tipo: "serie",
  ida: "IDA-1",
  coautores: [
    { nombre: "Ana Escritora", ipi: "IPI-00000001", rol: "guionista" },
  ],
  estado_declaracion: "completa",
  suma_porcentajes: 100,
  version_vigente: 3,
} satisfies Obra;

// La obra que el editor abre para su PRIMERA declaracion: declara que no tiene
// ninguna version vigente, y eso es una afirmacion del backend, no un hueco.
const obraSinDeclarar = {
  ...obraConDeclaracion,
  id: "obra-3",
  titulo: "Sin Declarar Todavia",
  estado_declaracion: "incompleta",
  suma_porcentajes: 0,
  version_vigente: null,
} satisfies Obra;

const versionCerrada: VersionDeclaracion = {
  version: 2,
  vigente_desde: "2026-01-15T10:00:00Z",
  vigente_hasta: "2026-03-01T09:00:00Z",
  estado: "completa",
  partes: [
    { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 60 },
    { titular_id: "tit-2", ipi: "IPI-00000002", porcentaje: 40 },
  ],
};

const versionTresAbierta: VersionDeclaracion = {
  ...versionCerrada,
  version: 3,
  vigente_desde: "2026-03-01T09:00:00Z",
  vigente_hasta: null,
  partes: [{ titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 100 }],
};

const HISTORIAL: VersionDeclaracion[] = [versionCerrada, versionTresAbierta];

describe("useObra: los desenlaces de leer una obra (item 13)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("empieza cargando y termina en `ok` con la obra que contesto el servidor", async () => {
    const respuesta = json(obraConDeclaracion);
    vi.mocked(fetch).mockResolvedValue(respuesta);

    const { result } = renderHook(() => useObra("obra-1"));

    expect(result.current.estado).toBe("cargando");

    await waitFor(() => expect(result.current.estado).toBe("ok"));

    // Precondicion: la peticion SALIO, y salio a esta direccion. Un doble que
    // nadie llamara dejaria el `ok` de abajo sin explicacion.
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1);
    expect(String(vi.mocked(fetch).mock.calls[0][0])).toBe("/api/obras/obra-1");
    expect(respuesta.ok).toBe(true);

    if (result.current.estado !== "ok") throw new Error("no llego a `ok`");
    expect(result.current.obra).toEqual(obraConDeclaracion);
    expect(result.current.obra.version_vigente).toBe(3);
  });

  it("codifica el identificador de la direccion, que es lo que hacian las tres pantallas", async () => {
    vi.mocked(fetch).mockResolvedValue(json(obraConDeclaracion));

    const { result } = renderHook(() => useObra("obra 1/2"));

    await waitFor(() => expect(result.current.estado).toBe("ok"));

    expect(String(vi.mocked(fetch).mock.calls[0][0])).toBe(
      "/api/obras/obra%201%2F2",
    );
  });

  // El caso negativo que el item 13 exige: un 404 NO es `error`. Confundirlos
  // apaga la pantalla que dice "esa obra no esta en el catalogo" y enciende la
  // que dice "algo fallo", que es afirmar un fallo que no ocurrio -las dos
  // peticiones contestaron bien, con el 404 que el contrato usa para esto-.
  it("un 404 da `ausente`, NO `error`", async () => {
    const respuesta = json({ error: "esa obra no esta en el catalogo" }, 404);
    vi.mocked(fetch).mockResolvedValue(respuesta);

    const { result } = renderHook(() => useObra("obra-9"));

    await waitFor(() => expect(result.current.estado).toBe("ausente"));

    // Precondicion: el doble contesto un 404 de verdad, con cuerpo y todo. Un
    // doble que rechazara la promesa mediria el camino del fallo de red y esta
    // prueba pasaria afirmando lo mismo por el motivo equivocado.
    expect(respuesta.status).toBe(404);
    expect(respuesta.ok).toBe(false);

    if (result.current.estado !== "ausente")
      throw new Error("no llego a `ausente`");
    expect(result.current.id).toBe("obra-9");
    // Ni el mensaje del backend ni ninguna otra forma de `error`: el 404 trae su
    // propio desenlace y no se mezcla con el fallo.
    expect(Object.keys(result.current).sort()).toEqual(["estado", "id"]);
  });

  // El control positivo del de arriba: `error` SI existe y es alcanzable. Sin
  // esto, un arreglo que mandara TODO a `ausente` pasaria la prueba anterior.
  it("control positivo: un 500 da `error` con el mensaje del backend", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json({ error: "la base esta caida" }, 500),
    );

    const { result } = renderHook(() => useObra("obra-1"));

    await waitFor(() => expect(result.current.estado).toBe("error"));

    if (result.current.estado !== "error")
      throw new Error("no llego a `error`");
    expect(result.current.mensaje).toBe("la base esta caida");
  });

  const OBRAS_ILEGIBLES: [string, unknown][] = [
    ["un objeto que no es una obra", {}],
    [
      "una obra sin `version_vigente` (el backend no lo poblo)",
      { ...obraConDeclaracion, version_vigente: undefined },
    ],
    [
      "una obra con la suma como texto",
      { ...obraConDeclaracion, suma_porcentajes: "100" },
    ],
  ];

  it.each(OBRAS_ILEGIBLES)(
    "un 2xx con %s da `ilegible`, sin pintarlo ni tumbarlo",
    async (_caso, cuerpo) => {
      vi.mocked(fetch).mockResolvedValue(json(cuerpo));

      const { result } = renderHook(() => useObra("obra-1"));

      await waitFor(() => expect(result.current.estado).toBe("ilegible"));

      // Y sobre todo: un payload sin `version_vigente` NO se lee como "esta obra
      // no tiene declaracion". `null` es una afirmacion del backend; el campo
      // ausente no lo es, y el desenlace tiene que poder separarlos.
      expect(result.current.estado).not.toBe("ok");
    },
  );

  it("un 2xx que no trae JSON da `error`, no `ilegible`", async () => {
    // Son dos cosas distintas y el preambulo unificado no las puede fundir: aqui
    // el cuerpo no llego a ser un dato -`useApi` lo trata como fallo-, mientras
    // que `ilegible` es un cuerpo que llego, se pudo leer y no tiene la forma de
    // una obra. Fundirlos dejaria al detalle diciendo "la obra no llegó con los
    // datos que esta pantalla lee" sobre una respuesta que no se pudo leer.
    vi.mocked(fetch).mockResolvedValue(
      new Response("<html><body>sin API</body></html>", {
        status: 200,
        headers: { "content-type": "text/html" },
      }),
    );

    const { result } = renderHook(() => useObra("obra-1"));

    await waitFor(() => expect(result.current.estado).toBe("error"));

    if (result.current.estado !== "error")
      throw new Error("no llego a `error`");
    expect(result.current.mensaje).toBe("la respuesta no vino en JSON");
  });
});

describe("conciliarConElHistorial (item 13): la quinta salida", () => {
  it("la union puede expresar la quinta salida, y no solo los cuatro desenlaces", () => {
    // Esta prueba es de compilacion y por eso importa: `EstadoDeObra` es la
    // promesa del item 13 -los cuatro desenlaces MAS "hay obra pero el historial
    // no cuadra"-, y si alguien sacara `descuadrado` de la union, estas dos
    // asignaciones dejarian de compilar y `npm run typecheck` caeria antes de
    // que ninguna pantalla se enterara.
    const quinto: EstadoDeObra = {
      estado: "descuadrado",
      mensaje: "hay obra, pero el historial no cuadra",
    };
    const lectura: EstadoDeObra = { estado: "cargando" };
    const conciliado: EstadoDeObra = {
      estado: "vigente",
      vigente: versionTresAbierta,
    };

    expect([quinto.estado, lectura.estado, conciliado.estado]).toEqual([
      "descuadrado",
      "cargando",
      "vigente",
    ]);
  });

  const CASOS: [string, Obra, VersionDeclaracion[], EstadoDeObra][] = [
    [
      "el historial trae abierta la version que la obra declara vigente",
      obraConDeclaracion,
      HISTORIAL,
      { estado: "vigente", vigente: versionTresAbierta },
    ],
    [
      "el historial viene vacio y la obra declara vigente la 3",
      obraConDeclaracion,
      [],
      {
        estado: "descuadrado",
        mensaje:
          "El servidor declara vigente la versión 3, pero el historial no trae esa versión como única versión abierta.",
      },
    ],
    [
      "el historial no trae ninguna version abierta",
      obraConDeclaracion,
      [versionCerrada],
      {
        estado: "descuadrado",
        mensaje:
          "El servidor declara vigente la versión 3, pero el historial no trae esa versión como única versión abierta.",
      },
    ],
    [
      "el historial trae abierta OTRA version",
      obraConDeclaracion,
      [{ ...versionTresAbierta, version: 4 }],
      {
        estado: "descuadrado",
        mensaje:
          "El servidor declara vigente la versión 3, pero el historial no trae esa versión como única versión abierta.",
      },
    ],
    [
      "el historial trae DOS versiones abiertas",
      obraConDeclaracion,
      [versionTresAbierta, { ...versionTresAbierta, version: 4 }],
      {
        estado: "descuadrado",
        mensaje:
          "El servidor declara vigente la versión 3, pero el historial no trae esa versión como única versión abierta.",
      },
    ],
    [
      "la obra no declara ninguna vigente y el historial no abre ninguna",
      obraSinDeclarar,
      [],
      { estado: "sinVigente" },
    ],
    [
      "la obra no declara ninguna vigente y el historial abre una",
      obraSinDeclarar,
      [versionTresAbierta],
      {
        estado: "descuadrado",
        mensaje:
          "El servidor no declara ninguna versión vigente, pero el historial no trae esa versión como única versión abierta.",
      },
    ],
  ];

  it.each(CASOS)(
    "con %s la conciliacion dice lo que se sabe",
    (_caso, obra, historial, esperado) => {
      expect(conciliarConElHistorial(obra, historial)).toEqual(esperado);
    },
  );

  it("un descuadre no se puede leer como `ausente` ni como `error`", () => {
    // La distincion que PM-5 protege, dicha como prueba y no como comentario: en
    // el descuadre LA OBRA ESTA y las dos peticiones contestaron bien. Si el
    // arreglo lo mandara a `ausente` -o a `error`-, el detalle dejaria de decir
    // que el reparto no cuadra y el editor volveria a ofrecer guardar.
    const conciliacion = conciliarConElHistorial(obraConDeclaracion, []);

    expect(conciliacion.estado).toBe("descuadrado");
    expect(conciliacion.estado).not.toBe("ausente");
    expect(conciliacion.estado).not.toBe("error");
    expect(conciliacion.estado).not.toBe("ok");
  });
});
