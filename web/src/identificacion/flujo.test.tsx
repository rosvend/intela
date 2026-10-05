import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "../App";
import { setToken } from "../api";
import type { Asiento } from "../auditoria/tipos";
import type { Obra } from "../catalogo/tipos";
import { ProveedorDeSesion } from "../sesion";
import { formatearInstante } from "../tablero/formato";
import type { ResolucionDeCaso } from "./resolucion";
import {
  HECHO_ASIGNADA,
  sugerenciaNinguna,
  type CasoIdentificacion,
} from "./tipos";

/**
 * El recorrido completo de #39 sobre `<App />` con un servidor falso CON
 * ESTADO (plano, seccion 6): unir un caso de la bandeja con su obra, verlo salir de
 * ella y del badge, encontrarlo asignado y con su responsable en la lista ONI,
 * abrir su historial, saltar a la obra y leer la resolucion -usuario, fecha,
 * decision y nota- en la bitacora de la obra.
 *
 * Lo que esta prueba comprueba y ninguna de las unitarias puede comprobar es el
 * CAMINO ENTRE PANTALLAS: que el caso que la bandeja resuelve es el mismo que
 * la lista ONI sirve como asignado y el mismo que la obra asienta. Un servidor
 * de respuestas fijas por ruta probaria las cinco pantallas por separado y no
 * que el dato viaje de una a otra.
 *
 * Titulos, fuentes y nombres sinteticos, como en el resto de `web/src`.
 */

const ACTOR = "Admin Intela";
const CUANDO = "2026-09-28T15:30:00Z";
const NOTA = "coincide la ficha técnica y el elenco";

const OBRA = {
  id: "obra-1",
  titulo: "La Niña",
  genero: "Drama",
  anio: 2016,
  tipo: "serie",
  ida: "IDA-1",
  coautores: [],
  estado_declaracion: "incompleta",
  suma_porcentajes: 0,
  // Sin declaracion a proposito: la ficha no pide entonces el historial de la
  // declaracion, y este recorrido se queda con las peticiones que si le
  // importan -la obra y su bitacora-.
  version_vigente: null,
} satisfies Obra;

const CASO_PENDIENTE = {
  id: "uso-1",
  titulo: "La Niña T3 E12",
  titulo_original: "La niña — Capítulo 12",
  fuente: "Canal de Prueba",
  modalidad: "tv",
  reporte_id: "ING-2026-0890",
  periodo: "2026-09",
  ids_fuente: "ID_Ficha=48213\nID_Emision=991204",
  evidencia: "Coincidencia parcial por título y número de episodio.",
  estado: "pendiente",
  candidatos: [
    {
      obra_id: OBRA.id,
      titulo: OBRA.titulo,
      anio: OBRA.anio,
      genero: OBRA.genero,
      puntaje: 0.71,
      titulo_consultado: "La Niña T3 E12",
    },
  ],
  obra_asignada: null,
  resuelto_por: null,
  resuelto_en: null,
  ultima_actualizacion: "2026-09-20T10:00:00Z",
  nota: null,
  sugerencia: sugerenciaNinguna(),
} satisfies CasoIdentificacion;

/** Un segundo pendiente, sin candidatas, para el conteo del badge. */
const OTRO_CASO = {
  ...CASO_PENDIENTE,
  id: "uso-2",
  titulo: "Otra Novela T1 E01",
  titulo_original: "Otra Novela",
  reporte_id: "ING-2026-0891",
  candidatos: [],
} satisfies CasoIdentificacion;

type ServidorFalso = {
  casos: CasoIdentificacion[];
  asientos: Asiento[];
};

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const CLAVES_DE_FILTRO = ["estado", "fuente", "periodo"] as const;

/** Los filtros de la consulta que el caso cumple: filtra el SERVIDOR, no el cliente. */
function cuadra(
  caso: CasoIdentificacion,
  consulta: URLSearchParams,
  claves: readonly (typeof CLAVES_DE_FILTRO)[number][] = CLAVES_DE_FILTRO,
): boolean {
  return claves.every((clave) => {
    const valor = consulta.get(clave);
    return valor === null || valor === "" || caso[clave] === valor;
  });
}

/** El caso ya resuelto, tal como lo devolveria el contrato de #175. */
function resolver(
  caso: CasoIdentificacion,
  cuerpo: ResolucionDeCaso,
): CasoIdentificacion {
  return {
    ...caso,
    estado: cuerpo.decision === "descartar" ? "descartado" : "asignado",
    obra_asignada:
      cuerpo.decision === "asignar"
        ? { id: cuerpo.obra_id ?? "", titulo: OBRA.titulo }
        : null,
    resuelto_por: { id: "usr-1", nombre: ACTOR },
    resuelto_en: CUANDO,
    ultima_actualizacion: CUANDO,
    nota: cuerpo.nota,
  };
}

/**
 * El asiento `identificacion.asignada` que deja la decision, con el payload en
 * snake_case que describe el esquema `Asiento` del contrato provisional de #175
 * (el mismo que lee `leerResolucionAsentada`).
 */
function asientoDeAsignacion(
  caso: CasoIdentificacion,
  obraId: string,
  nota: string,
): Asiento {
  return {
    id: `asiento-${caso.id}`,
    hecho: HECHO_ASIGNADA,
    ref_tipo: "obra",
    ref_id: obraId,
    actor: "usr-1",
    payload: {
      uso_id: caso.id,
      decision: "asignar",
      obra_id: obraId,
      titulo: caso.titulo,
      fuente: caso.fuente,
      periodo: caso.periodo,
      reporte_id: caso.reporte_id,
      nota,
      actor_nombre: ACTOR,
    },
    cuando: CUANDO,
  };
}

/**
 * Un servidor falso con estado: la resolucion que confirma el POST cambia lo
 * que devuelven las lecturas siguientes. Responde por URL y metodo, y todo lo
 * que no maneja es un 404 explicito -asi una peticion de mas se ve en el
 * mensaje de error de la pantalla en vez de colgarse-.
 */
function instalarServidor({
  casos,
}: {
  casos: CasoIdentificacion[];
}): ServidorFalso {
  const estado: ServidorFalso = { casos, asientos: [] };

  vi.stubGlobal(
    "fetch",
    vi.fn(async (entrada: RequestInfo | URL, init?: RequestInit) => {
      const url = String(entrada);
      const metodo = init?.method ?? "GET";

      if (metodo === "GET" && url === "/api/auth/session") {
        return json({
          id: "usr-1",
          email: "admin@redes.co",
          nombre: ACTOR,
          rol: "administrador",
          titular_id: "",
        });
      }

      if (metodo === "GET" && url.startsWith("/api/identificacion/casos")) {
        const consulta = new URLSearchParams(url.split("?")[1] ?? "");
        return json({
          casos: estado.casos.filter((caso) => cuadra(caso, consulta)),
          // `pendientes` es un conteo, no una pagina (contrato): no lo decide
          // `estado` -el badge pide `limite=1` y lo que quiere es el total-.
          pendientes: estado.casos.filter(
            (caso) =>
              caso.estado === "pendiente" &&
              cuadra(caso, consulta, ["fuente", "periodo"]),
          ).length,
        });
      }

      const resolucion =
        /^\/api\/identificacion\/casos\/([^/]+)\/resolucion$/.exec(url);
      if (metodo === "POST" && resolucion) {
        const id = decodeURIComponent(resolucion[1] ?? "");
        const cuerpo = JSON.parse(String(init?.body)) as ResolucionDeCaso;
        const caso = estado.casos.find((c) => c.id === id);
        if (!caso) return json({ error: `no hay caso ${id}` }, 404);

        const resuelto = resolver(caso, cuerpo);
        estado.casos = estado.casos.map((c) => (c.id === id ? resuelto : c));
        if (cuerpo.decision === "asignar") {
          estado.asientos = [
            ...estado.asientos,
            asientoDeAsignacion(resuelto, cuerpo.obra_id ?? "", cuerpo.nota),
          ];
        }
        return json(resuelto);
      }

      if (metodo === "GET" && /^\/api\/obras\/[^/]+$/.test(url)) {
        return json(OBRA);
      }
      if (metodo === "GET" && /^\/api\/auditoria\/obra\/[^/]+$/.test(url)) {
        return json(estado.asientos);
      }

      return json({ error: `ruta no manejada: ${metodo} ${url}` }, 404);
    }),
  );

  return estado;
}

/**
 * El badge de pendientes del sidebar, o `null` si la pantalla no lo pinta.
 *
 * Se busca por su clase y no por su texto: su contenido visible es el numero
 * -lo demas son las palabras que solo lee un lector de pantalla-, y lo que
 * esta prueba afirma es el numero.
 */
function textoDelBadge(): string | null {
  const enlace = screen.queryByRole("link", { name: /^Identificación/ });
  const badge = enlace?.querySelector(".sidebar-badge");
  return badge?.textContent?.replace(/\s+/g, " ").trim() ?? null;
}

function montar() {
  return render(
    <MemoryRouter initialEntries={["/identificacion"]}>
      <ProveedorDeSesion>
        <App />
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

/** El titulo del caso en escena: el unico `<h2>` de la bandeja. */
function tituloEnLaBandeja(titulo: string): HTMLElement | null {
  return screen.queryByRole("heading", { level: 2, name: titulo });
}

/** La fila de la lista ONI de ese titulo. */
function filaDeLaLista(titulo: string): HTMLElement {
  const tabla = screen.getByRole("table", { name: "Lista ONI" });
  const fila = within(tabla).getByText(titulo).closest("tr");
  if (!fila) throw new Error(`"${titulo}" no esta dentro de una fila`);
  return fila;
}

describe("flujo de identificacion (integracion con App)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    // El token va antes de montar: `ProveedorDeSesion` resuelve la sesion con
    // el, y de esa respuesta sale el rol que decide el badge y el guard.
    setToken("tok-flujo");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("resolver un caso lo saca de la bandeja y del badge, lo muestra asignado en la lista ONI y deja su nota en la bitacora de la obra", async () => {
    instalarServidor({ casos: [CASO_PENDIENTE] });
    montar();

    // 1. La bandeja pone el pendiente en escena con sus candidatas, y el sidebar cuenta
    //    lo mismo que la bandeja tiene delante.
    expect(
      await screen.findByRole("heading", {
        name: "Bandeja de identificación",
      }),
    ).toBeTruthy();
    // El titulo del caso espera a la lectura de la cola: la cabecera de la
    // bandeja se pinta antes que sus tarjetas.
    expect(
      await screen.findByRole("heading", {
        level: 2,
        name: CASO_PENDIENTE.titulo,
      }),
    ).toBeTruthy();
    expect(screen.getByText("0,71")).toBeTruthy();
    await waitFor(() => expect(textoDelBadge()).toBe("1 pendientes"));

    // 2. Unir con la candidata: ninguna llega elegida (ADR 0007), y el boton
    //    principal no se puede pulsar sin obra ni sin nota (es obligatoria).
    const unir = screen.getByRole("button", { name: "Unir con esta obra" });
    expect(unir).toHaveProperty("disabled", true);
    fireEvent.click(
      screen.getByRole("radio", { name: new RegExp(OBRA.titulo) }),
    );
    expect(unir).toHaveProperty("disabled", true);

    fireEvent.change(screen.getByLabelText("Nota para la bitácora"), {
      // Con espacios a los lados: la nota viaja recortada.
      target: { value: `  ${NOTA}  ` },
    });
    fireEvent.click(unir);

    // 3. El aviso de exito, el caso fuera de la bandeja y el badge sin numero
    //    -con cero no se pinta-.
    expect(
      await screen.findByText(`Registro asignado a “${OBRA.titulo}”`),
    ).toBeTruthy();
    expect(tituloEnLaBandeja(CASO_PENDIENTE.titulo)).toBeNull();
    expect(
      screen.getByText("Todo en orden: no hay usos pendientes por identificar"),
    ).toBeTruthy();
    expect(
      screen.getByText("Cada decisión quedó registrada con su nota."),
    ).toBeTruthy();
    await waitFor(() => expect(textoDelBadge()).toBeNull());

    // 4. La lista ONI: el mismo caso, ahora asignado y con quien lo resolvio.
    fireEvent.click(screen.getByRole("link", { name: "Ver lista ONI" }));
    expect(
      await screen.findByRole("heading", { name: "Lista ONI" }),
    ).toBeTruthy();

    const fila = filaDeLaLista(CASO_PENDIENTE.titulo);
    expect(within(fila).getByText("Asignada")).toBeTruthy();
    expect(within(fila).getByText(ACTOR)).toBeTruthy();
    expect(within(fila).queryByText("Pendiente")).toBeNull();
    // El badge sigue sin numero al cambiar de pantalla: el conteo optimista de
    // la bandeja no se pierde al desmontarla.
    expect(textoDelBadge()).toBeNull();

    // 5. Su historial, armado con lo que la propia fila trae, y el salto a la
    //    obra.
    fireEvent.click(
      within(fila).getByRole("button", { name: "Ver historial" }),
    );
    const historial = await screen.findByRole("dialog");
    expect(
      within(historial).getByRole("heading", {
        name: `Registro asignado a “${OBRA.titulo}”`,
      }),
    ).toBeTruthy();
    expect(within(historial).getByText(ACTOR)).toBeTruthy();
    expect(within(historial).getByText(NOTA)).toBeTruthy();

    const verLaObra = within(historial).getByRole("link", {
      name: "Ver la obra",
    });
    expect(verLaObra.getAttribute("href")).toBe(`/catalogo/${OBRA.id}`);
    fireEvent.click(verLaObra);

    // 6. La bitacora de la obra: la resolucion manual, con usuario, fecha,
    //    decision y nota.
    expect(
      await screen.findByRole("heading", { level: 1, name: OBRA.titulo }),
    ).toBeTruthy();
    expect(
      await screen.findByRole("heading", {
        name: "Historial de resoluciones manuales",
      }),
    ).toBeTruthy();
    expect(screen.getByText("1 resoluciones")).toBeTruthy();

    const [entrada] = screen.getAllByRole("listitem");
    if (!entrada)
      throw new Error("el historial de la obra no pinto la resolucion");

    expect(entrada.textContent).toContain(ACTOR);
    expect(entrada.textContent).toContain(
      `Asignó “${CASO_PENDIENTE.titulo}” de ${CASO_PENDIENTE.fuente}, sep 2026, a esta obra.`,
    );
    expect(entrada.textContent).toContain(NOTA);
    // La referencia de origen espera detras de su Detalle.
    fireEvent.click(
      within(entrada).getByRole("button", { name: "Referencia de origen" }),
    );
    expect(entrada.textContent).toContain(
      `${CASO_PENDIENTE.id} · ${CASO_PENDIENTE.reporte_id}`,
    );

    // La fecha, con el formato de la casa y el instante del asiento en el
    // atributo legible por maquina.
    const fecha = within(entrada).getByText(formatearInstante(CUANDO));
    expect(fecha.getAttribute("dateTime")).toBe(CUANDO);
  });

  it("el badge cuenta los que siguen pendientes: resolver uno lo baja, no lo esconde", async () => {
    instalarServidor({ casos: [CASO_PENDIENTE, OTRO_CASO] });
    montar();

    expect(
      await screen.findByRole("heading", {
        name: "Bandeja de identificación",
      }),
    ).toBeTruthy();
    await waitFor(() => expect(textoDelBadge()).toBe("2 pendientes"));

    fireEvent.click(
      screen.getByRole("radio", { name: new RegExp(OBRA.titulo) }),
    );
    fireEvent.change(screen.getByLabelText("Nota para la bitácora"), {
      target: { value: NOTA },
    });
    fireEvent.click(screen.getByRole("button", { name: "Unir con esta obra" }));

    await waitFor(() => expect(textoDelBadge()).toBe("1 pendientes"));
    // Y el otro caso sube a escena: el conteo baja por el que se resolvio.
    await waitFor(() =>
      expect(tituloEnLaBandeja(OTRO_CASO.titulo)).not.toBeNull(),
    );
    expect(tituloEnLaBandeja(CASO_PENDIENTE.titulo)).toBeNull();
  });
});
