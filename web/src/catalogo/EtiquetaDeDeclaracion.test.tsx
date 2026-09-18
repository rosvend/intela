import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "../App";
import { setToken } from "../api";
import { ProveedorDeSesion } from "../sesion";
import type { Obra, VersionDeclaracion } from "./tipos";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// Fixtures tipados con el contrato generado: si `Obra` o `VersionDeclaracion`
// cambian en api/openapi.yaml, `tsc` rompe aqui antes que en pantalla. Nombres y
// titulos sinteticos, no de ningun canal real.
const obraCompleta = {
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

const versionVigente: VersionDeclaracion = {
  version: 3,
  vigente_desde: "2026-03-01T09:00:00Z",
  vigente_hasta: null,
  estado: "completa",
  partes: [
    { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 75 },
    { titular_id: "tit-2", ipi: "IPI-00000002", porcentaje: 25 },
  ],
};

// Los dos casos que llegan del backend con el MISMO `estado_declaracion`
// -`incompleta`- y que solo separa `version_vigente`: una obra que declara de
// menos y una que nadie declaro. Son los que obligan a la etiqueta a decidir
// algo, y por eso no pueden faltar en la comparacion.
const obraIncompleta = {
  ...obraCompleta,
  id: "obra-2",
  titulo: "Noche de Bodas",
  estado_declaracion: "incompleta",
  suma_porcentajes: 74.5,
  version_vigente: 2,
} satisfies Obra;

const versionDosIncompleta: VersionDeclaracion = {
  version: 2,
  vigente_desde: "2026-03-01T09:00:00Z",
  vigente_hasta: null,
  estado: "incompleta",
  partes: [{ titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 74.5 }],
};

const obraSinDeclaracion = {
  ...obraCompleta,
  id: "obra-3",
  titulo: "Sin Declarar Todavia",
  ida: undefined,
  estado_declaracion: "incompleta",
  suma_porcentajes: 0,
  version_vigente: null,
} satisfies Obra;

/**
 * Un hecho de la declaracion, con lo que el backend manda y lo que las DOS
 * pantallas tienen que pintar de el.
 */
type Caso = {
  caso: string;
  obra: Obra;
  historial: VersionDeclaracion[];
  texto: string;
  clase: string;
};

/**
 * Los tres hechos que un estado de declaracion puede decir. El texto y la clase
 * esperados se escriben UNA vez, aqui, y no una por pantalla: es el valor unico
 * contra el que se comprueban las dos, y dos copias del esperado serian el
 * defecto que este archivo existe para impedir, mudado al test.
 */
const CASO_COMPLETA: Caso = {
  caso: "una declaracion completa",
  obra: obraCompleta,
  historial: [versionVigente],
  texto: "Completa",
  clase: "badge-estado-completa",
};

const CASO_INCOMPLETA: Caso = {
  caso: "una declaracion declarada y por debajo de 100",
  obra: obraIncompleta,
  historial: [versionDosIncompleta],
  texto: "Incompleta",
  clase: "badge-estado-incompleta",
};

const CASO_SIN_DECLARACION: Caso = {
  caso: "una obra que nadie declaro",
  obra: obraSinDeclaracion,
  historial: [],
  texto: "Sin declaración",
  clase: "badge-estado-sin-declaracion",
};

const CASOS = [CASO_COMPLETA, CASO_INCOMPLETA, CASO_SIN_DECLARACION];

const esElListado = (url: string) => url.startsWith("/api/obras?");
const esLaObra = (url: string) => /^\/api\/obras\/[^/]+$/.test(url);
const esElHistorial = (url: string) =>
  /^\/api\/obras\/[^/]+\/declaracion\/historial$/.test(url);

/**
 * Un backend falso que le sirve la MISMA obra a las dos pantallas: el listado la
 * recibe dentro de su pagina y el detalle en su propia lectura, con el mismo
 * historial. Esa identidad es la mitad del test: si cada pantalla recibiera un
 * cuerpo distinto, que las dos etiquetas coincidieran no diria nada sobre el
 * mismo hecho.
 */
function simularServidor({
  obra,
  historial,
}: {
  obra: Obra;
  historial: VersionDeclaracion[];
}) {
  vi.mocked(fetch).mockImplementation((entrada, init) => {
    const url = String(entrada);
    const metodo = init?.method ?? "GET";
    if (url === "/api/auth/session") {
      return Promise.resolve(
        json({
          id: "usr-1",
          email: "x@redes.co",
          nombre: "Persona de Prueba",
          rol: "administrador",
          titular_id: "",
        }),
      );
    }
    if (metodo === "GET" && esElListado(url)) {
      return Promise.resolve(json([obra]));
    }
    if (metodo === "GET" && esElHistorial(url)) {
      return Promise.resolve(json(historial));
    }
    if (metodo === "GET" && esLaObra(url)) {
      return Promise.resolve(json(obra));
    }
    return Promise.resolve(json({ error: "ruta no encontrada" }, 404));
  });
}

function montarApp(entrada: string) {
  return render(
    <MemoryRouter initialEntries={[entrada]}>
      <ProveedorDeSesion>
        <App />
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

/**
 * La unica etiqueta de estado en pantalla. Se busca por su clase porque es la
 * MISMA en las dos pantallas -el listado la pinta dentro de una celda de su
 * tabla y el detalle dentro de un dato de su ficha- y lo que este test compara
 * es lo que dice, no donde esta.
 */
function etiquetaDeEstado(): HTMLElement {
  const etiquetas = Array.from(
    document.querySelectorAll<HTMLElement>(".badge-estado"),
  );
  const [etiqueta] = etiquetas;
  if (etiquetas.length !== 1 || !etiqueta) {
    throw new Error(
      `se esperaba una sola etiqueta de estado en pantalla, hay ${etiquetas.length}`,
    );
  }
  return etiqueta;
}

/** Monta el listado del catalogo y devuelve lo que su etiqueta de estado dice. */
async function rotuloEnElCatalogo(caso: Caso) {
  simularServidor({ obra: caso.obra, historial: caso.historial });
  montarApp("/catalogo");
  await screen.findByRole("table", { name: "Catálogo de obras" });
  // Se devuelven cadenas y no el elemento: la etiqueta desaparece del DOM al
  // desmontar esta pantalla para montar la siguiente.
  return {
    texto: etiquetaDeEstado().textContent,
    clase: etiquetaDeEstado().className,
  };
}

/** Monta el detalle de la misma obra y devuelve lo que su etiqueta dice. */
async function rotuloEnElDetalle(caso: Caso) {
  simularServidor({ obra: caso.obra, historial: caso.historial });
  montarApp(`/catalogo/${caso.obra.id}`);
  await screen.findByRole("heading", { name: caso.obra.titulo });
  return {
    texto: etiquetaDeEstado().textContent,
    clase: etiquetaDeEstado().className,
  };
}

/**
 * Monta el historial de versiones de la misma obra -paso 7- y devuelve lo que la
 * etiqueta de su primera version dice.
 *
 * El historial es la TERCERA pantalla que pinta un estado, y la unica que lo
 * pinta sin tener una obra delante: cada bloque lleva el `estado` de SU version.
 * Por eso su etiqueta sale del mismo componente que la de las otras dos
 * (`EtiquetaDeEstado`, que `EtiquetaDeDeclaracion` usa para el caso de una obra
 * con declaracion): si alguien volviera a decidir el texto o el color dentro
 * del historial, esta linea lo diria, porque las dos pantallas dejarian de
 * coincidir sobre el mismo estado.
 */
async function rotuloEnElHistorial(caso: Caso) {
  simularServidor({ obra: caso.obra, historial: caso.historial });
  montarApp(`/catalogo/${caso.obra.id}/historial`);
  // Se espera a la TABLA de partes y no al titulo: el titulo y el nombre de la
  // obra llegan con la obra, y la etiqueta del estado llega con la version, en la
  // peticion siguiente, la del historial.
  await screen.findByRole("table", { name: /^Partes de la versión \d+$/ });
  return {
    texto: etiquetaDeEstado().textContent,
    clase: etiquetaDeEstado().className,
  };
}

describe("el estado de la declaracion se dice una sola vez", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it.each(CASOS)(
    "el catalogo y el detalle pintan el mismo estado para $caso",
    async (caso) => {
      const { texto, clase } = caso;

      const enElCatalogo = await rotuloEnElCatalogo(caso);

      cleanup();

      const enElDetalle = await rotuloEnElDetalle(caso);

      // Uno: las dos pantallas afirman lo mismo del mismo hecho. Si alguien
      // devuelve el rotulo a una sola de las dos -una copia local en la pantalla
      // de detalle, que es la forma exacta en que esto estaba escrito antes-,
      // esta linea es la que lo dice: la otra pantalla sigue pintando el rotulo
      // compartido y las dos dejan de coincidir.
      expect(enElDetalle.texto).toBe(enElCatalogo.texto);
      expect(enElDetalle.clase).toBe(enElCatalogo.clase);

      // Y dos: contra el esperado de arriba, escrito una sola vez para las dos.
      // Un cambio que mueva el rotulo de las dos pantallas a la vez -posible
      // mientras el texto se decidia en dos sitios- tampoco pasa de aqui: lo que
      // se fija es el hecho -que estado se dice-, no solo que las dos coincidan.
      expect(enElCatalogo.texto).toBe(texto);
      expect(enElDetalle.texto).toBe(texto);
      expect(enElCatalogo.clase).toContain(clase);
      expect(enElDetalle.clase).toContain(clase);
    },
  );

  it("sin declaracion y declarada incompleta llegan con el mismo estado y no se pintan igual", async () => {
    // Esta es la razon por la que la etiqueta mira `version_vigente` y no el
    // enum: los dos cuerpos llegan con el MISMO estado. Si alguien "simplificara"
    // la etiqueta para pintar `estado_declaracion` tal cual, los dos rotulos se
    // volverian el mismo texto y esto lo diria antes que la pantalla.
    expect(CASO_SIN_DECLARACION.obra.estado_declaracion).toBe("incompleta");
    expect(CASO_INCOMPLETA.obra.estado_declaracion).toBe("incompleta");
    expect(CASO_INCOMPLETA.obra.version_vigente).not.toBeNull();
    expect(CASO_SIN_DECLARACION.obra.version_vigente).toBeNull();
    expect(CASO_SIN_DECLARACION.texto).not.toBe(CASO_INCOMPLETA.texto);

    const sinDeclaracion = await rotuloEnElCatalogo(CASO_SIN_DECLARACION);
    expect(sinDeclaracion.texto).toBe("Sin declaración");

    cleanup();

    const incompleta = await rotuloEnElCatalogo(CASO_INCOMPLETA);
    expect(incompleta.texto).toBe("Incompleta");
  });

  // El historial de versiones (paso 7) es la tercera pantalla que pinta un
  // estado, y la unica que lo pinta sin una obra delante: el de cada version.
  // Los dos casos con version son los que se pueden comparar; el de la obra sin
  // declaracion no tiene ninguna version, y ahi lo que hay que comprobar es que
  // no se pinte ningun estado.
  const CASOS_CON_VERSION = [CASO_COMPLETA, CASO_INCOMPLETA];

  it.each(CASOS_CON_VERSION)(
    "el historial pinta el mismo estado que el detalle para $caso",
    async (caso) => {
      const enElDetalle = await rotuloEnElDetalle(caso);

      cleanup();

      const enElHistorial = await rotuloEnElHistorial(caso);

      expect(enElHistorial.texto).toBe(enElDetalle.texto);
      expect(enElHistorial.clase).toBe(enElDetalle.clase);

      // Y contra el esperado escrito una sola vez arriba: lo que se fija es el
      // hecho -que estado se dice y de que color-, no solo que dos pantallas
      // coincidan entre ellas.
      expect(enElHistorial.texto).toBe(caso.texto);
      expect(enElHistorial.clase).toContain(caso.clase);
    },
  );

  it("una obra sin ninguna version no pinta ningun estado en el historial", async () => {
    // "Sin declaración" es lo que dice la ficha de una obra que nadie declaro, y
    // el historial no tiene por que repetirlo: no hay ninguna version de la que
    // decir un estado, y pintar la de la obra afirmaria una declaracion que no
    // existe (D-008).
    simularServidor({
      obra: CASO_SIN_DECLARACION.obra,
      historial: CASO_SIN_DECLARACION.historial,
    });
    montarApp(`/catalogo/${CASO_SIN_DECLARACION.obra.id}/historial`);

    await screen.findByText("Esta obra no tiene ninguna versión declarada.");
    expect(document.querySelectorAll(".badge-estado")).toHaveLength(0);
    expect(screen.queryByText("Sin declaración")).toBeNull();
    expect(screen.queryByText("Incompleta")).toBeNull();
  });
});
