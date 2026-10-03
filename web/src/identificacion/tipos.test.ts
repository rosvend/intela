import { describe, expect, it } from "vitest";
import {
  ETIQUETA_ESTADO,
  ESTADOS_DE_CASO,
  HECHO_ASIGNADA,
  RUTAS_IDENTIFICACION,
  esCaso,
  esPaginaDeCasos,
  etiquetaDeModalidad,
  formatearPuntaje,
  idsDeFuente,
  leerResolucionAsentada,
  sugerenciaNinguna,
  type CandidatoIdentificacion,
  type CasoIdentificacion,
  type PaginaCasosIdentificacion,
} from "./tipos";

// Fixtures tipados con el contrato generado: si `CasoIdentificacion` cambia en
// api/openapi.yaml, `tsc` rompe aqui antes que en pantalla. Titulos y fuentes
// sinteticos, no de ningun canal real.
const candidata = {
  obra_id: "obra-12",
  titulo: "Serie de Prueba",
  anio: 2016,
  genero: "Drama",
  puntaje: 0.71,
  titulo_consultado: "serie de prueba",
} satisfies CandidatoIdentificacion;

const pendiente = {
  id: "uso-1",
  titulo: "Serie de Prueba T3 E12",
  titulo_original: "",
  fuente: "canal-prueba",
  modalidad: "tv",
  reporte_id: "rep-1",
  periodo: "2024-11",
  ids_fuente: "id_ficha=48213\nid_emision=991204",
  evidencia: "banda ambigua: 1 candidatos",
  estado: "pendiente",
  candidatos: [candidata],
  obra_asignada: null,
  resuelto_por: null,
  resuelto_en: null,
  ultima_actualizacion: "2024-11-28T14:03:00Z",
  nota: null,
  sugerencia: sugerenciaNinguna(),
} satisfies CasoIdentificacion;

const asignado = {
  ...pendiente,
  id: "uso-2",
  estado: "asignado",
  candidatos: [],
  obra_asignada: { id: "obra-12", titulo: "Serie de Prueba" },
  resuelto_por: { id: "usr-1", nombre: "Ana Pérez" },
  resuelto_en: "2024-11-28T15:00:00Z",
  ultima_actualizacion: "2024-11-28T15:00:00Z",
  nota: "coincide la ficha tecnica",
} satisfies CasoIdentificacion;

describe("RUTAS_IDENTIFICACION", () => {
  it("la cola lleva los filtros en el orden del contrato y omite los vacios", () => {
    expect(
      RUTAS_IDENTIFICACION.casos({
        estado: "pendiente",
        fuente: "canal prueba",
        periodo: "2024-11",
        limite: 25,
        desplazamiento: 50,
      }),
    ).toBe(
      "/api/identificacion/casos?estado=pendiente&fuente=canal+prueba&periodo=2024-11&limite=25&desplazamiento=50",
    );
    expect(RUTAS_IDENTIFICACION.casos({ fuente: "", limite: 50 })).toBe(
      "/api/identificacion/casos?limite=50",
    );
  });

  it("el desplazamiento cero no viaja: es la primera pagina", () => {
    expect(
      RUTAS_IDENTIFICACION.casos({ estado: "pendiente", desplazamiento: 0 }),
    ).toBe("/api/identificacion/casos?estado=pendiente");
  });

  it("la resolucion escapa el id del uso", () => {
    expect(RUTAS_IDENTIFICACION.resolucion("uso 1/x")).toBe(
      "/api/identificacion/casos/uso%201%2Fx/resolucion",
    );
  });
});

describe("esCaso", () => {
  it("acepta un caso pendiente y uno resuelto del contrato", () => {
    expect(esCaso(pendiente)).toBe(true);
    expect(esCaso(asignado)).toBe(true);
  });

  it("rechaza un estado fuera del enum del contrato", () => {
    expect(esCaso({ ...pendiente, estado: "excluido" })).toBe(false);
  });

  it("rechaza un caso sin `sugerencia`: ausente no es ninguna", () => {
    const { sugerencia: _sugerencia, ...sinSugerencia } = pendiente;
    expect(esCaso(sinSugerencia)).toBe(false);
  });

  it("rechaza una sugerencia sin `sello`: ausente no es null", () => {
    const { sello: _sello, ...sinSello } = pendiente.sugerencia;
    expect(esCaso({ ...pendiente, sugerencia: sinSello })).toBe(false);
  });

  it("rechaza un caso sin `nota`: ausente no es lo mismo que null", () => {
    const { nota: _nota, ...sinNota } = pendiente;
    expect(esCaso(sinNota)).toBe(false);
  });

  it("rechaza un puntaje fuera de [0, 1] o que no es numero", () => {
    expect(
      esCaso({ ...pendiente, candidatos: [{ ...candidata, puntaje: 1.2 }] }),
    ).toBe(false);
    expect(
      esCaso({ ...pendiente, candidatos: [{ ...candidata, puntaje: "0.7" }] }),
    ).toBe(false);
  });

  it("rechaza un responsable sin nombre", () => {
    expect(esCaso({ ...asignado, resuelto_por: { id: "usr-1" } })).toBe(false);
  });

  it("rechaza un id vacio: es la clave de la fila", () => {
    expect(esCaso({ ...pendiente, id: "" })).toBe(false);
  });
});

describe("esPaginaDeCasos", () => {
  it("acepta la pagina del contrato", () => {
    const pagina = {
      pendientes: 1,
      casos: [pendiente, asignado],
    } satisfies PaginaCasosIdentificacion;
    expect(esPaginaDeCasos(pagina)).toBe(true);
  });

  it("una pagina con un caso mal formado es ilegible entera", () => {
    expect(
      esPaginaDeCasos({
        pendientes: 2,
        casos: [pendiente, { ...pendiente, id: 7 }],
      }),
    ).toBe(false);
  });

  it("rechaza un conteo de pendientes negativo o fraccionario", () => {
    expect(esPaginaDeCasos({ pendientes: -1, casos: [] })).toBe(false);
    expect(esPaginaDeCasos({ pendientes: 1.5, casos: [] })).toBe(false);
  });
});

describe("formatearPuntaje", () => {
  it("es un decimal es-CO con dos cifras, nunca un porcentaje", () => {
    expect(formatearPuntaje(0.71)).toBe("0,71");
    expect(formatearPuntaje(0.52941)).toBe("0,53");
    expect(formatearPuntaje(1)).toBe("1,00");
  });
});

describe("idsDeFuente", () => {
  it("parte una pareja tipo=valor por linea y descarta las vacias", () => {
    expect(idsDeFuente("id_ficha=48213\r\n\nid_emision=991204 ")).toEqual([
      "id_ficha=48213",
      "id_emision=991204",
    ]);
    expect(idsDeFuente("")).toEqual([]);
  });
});

describe("etiquetas", () => {
  it("cada estado del contrato tiene su etiqueta, en el orden del filtro", () => {
    expect(ESTADOS_DE_CASO).toEqual(["pendiente", "asignado", "descartado"]);
    expect(ESTADOS_DE_CASO.map((e) => ETIQUETA_ESTADO[e])).toEqual([
      "Pendiente",
      "Asignada",
      "Descartada",
    ]);
  });

  it("las siglas de modalidad van en mayusculas y lo demas tal cual", () => {
    expect(etiquetaDeModalidad("tv")).toBe("TV");
    expect(etiquetaDeModalidad("ott")).toBe("OTT");
    expect(etiquetaDeModalidad("cine")).toBe("cine");
  });
});

describe("leerResolucionAsentada", () => {
  // El payload de `identificacion.asignada` segun la descripcion del esquema
  // `Asiento` del contrato provisional de #175 (snake_case).
  const payload = {
    uso_id: "uso-1",
    decision: "asignar",
    obra_id: "obra-12",
    candidata: true,
    puntaje: "0.71000",
    titulo: "Serie de Prueba T3 E12",
    titulo_original: "",
    fuente: "canal-prueba",
    periodo: "2024-11",
    reporte_id: "rep-1",
    ids_fuente: "id_ficha=48213",
    escalon_anterior: "oni",
    evidencia_anterior: "banda ambigua",
    nota: "coincide la ficha tecnica",
    actor_nombre: "Ana Pérez",
    alias: null,
  };

  it("lee lo que el historial pinta", () => {
    expect(HECHO_ASIGNADA).toBe("identificacion.asignada");
    expect(leerResolucionAsentada(payload)).toEqual({
      usoId: "uso-1",
      titulo: "Serie de Prueba T3 E12",
      fuente: "canal-prueba",
      periodo: "2024-11",
      reporteId: "rep-1",
      nota: "coincide la ficha tecnica",
      actorNombre: "Ana Pérez",
    });
  });

  it("devuelve null si falta un campo que se pinta", () => {
    const { nota: _nota, ...sinNota } = payload;
    expect(leerResolucionAsentada(sinNota)).toBeNull();
    expect(leerResolucionAsentada(null)).toBeNull();
    expect(leerResolucionAsentada("texto")).toBeNull();
  });
});
