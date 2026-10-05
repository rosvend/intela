import { describe, expect, it } from "vitest";
import {
  accionDe,
  agruparPorAfectado,
  describirAlerta,
} from "./agruparAlertas";
import { Alerta } from "./tipos";

function alerta(
  cambios: Partial<Alerta> & Pick<Alerta, "id" | "tipo">,
): Alerta {
  return {
    detalle: "",
    periodo: "2025",
    referencia: "obra:obra-serie",
    ref_tipo: "obra",
    ref_id: "obra-serie",
    critica: false,
    detectada: "2026-05-02T08:30:00Z",
    resuelta: false,
    autocerrada: false,
    ...cambios,
  };
}

const RESERVA_60 =
  'la declaracion vigente de la obra "obra-serie" no esta completa: lo declarado suma 60% en 1 parte(s) y R-04 exige 100 exactos. Se retiene el TOTAL de esa obra, no se reparte la parte declarada (R-04, RD 13.1.3)';
const COAUTOR =
  'el coautor con IPI IPI-00000002 figura en el catalogo de la obra "obra-serie" y no tiene parte en la declaracion vigente: su porcentaje no esta declarado y sin el no se le puede pagar (R-03)';

describe("describirAlerta", () => {
  it("una declaracion al 60 % es una barra 60/100", () => {
    const d = describirAlerta(
      alerta({
        id: "a",
        tipo: "reserva_declaracion_incompleta",
        detalle: RESERVA_60,
      }),
    );
    expect(d.etiqueta).toBe("Declaración al 60 %");
    expect(d.visual).toEqual({ forma: "progreso", valor: 60 });
  });

  it("una suma decimal se escribe con coma", () => {
    const d = describirAlerta(
      alerta({
        id: "a",
        tipo: "reserva_declaracion_incompleta",
        detalle: "lo declarado suma 62.5% en 2 parte(s) y R-04 exige 100",
      }),
    );
    expect(d.etiqueta).toBe("Declaración al 62,5 %");
    expect(d.visual).toEqual({ forma: "progreso", valor: 62.5 });
  });

  it("una obra sin ninguna declaracion es una barra vacia", () => {
    const d = describirAlerta(
      alerta({
        id: "a",
        tipo: "reserva_declaracion_incompleta",
        detalle:
          'la obra "obra-x" no tiene ninguna Declaracion de Obra: se retiene el total',
      }),
    );
    expect(d.etiqueta).toBe("Sin declaración");
    expect(d.visual).toEqual({ forma: "progreso", valor: 0 });
  });

  it("un coautor ausente es un titular con porcentaje desconocido", () => {
    const d = describirAlerta(
      alerta({
        id: "a",
        tipo: "titular_sin_porcentaje",
        detalle: COAUTOR,
        ref_titular: "ipi:IPI-00000002",
      }),
    );
    expect(d.etiqueta).toBe("Coautor sin porcentaje");
    expect(d.visual).toEqual({
      forma: "titular",
      porcentaje: null,
      ipi: "IPI-00000002",
    });
  });

  it("una parte con porcentaje y sin IPI dice el porcentaje y no el IPI", () => {
    const d = describirAlerta(
      alerta({
        id: "a",
        tipo: "titular_sin_porcentaje",
        detalle:
          'la parte del titular "tit-7" en la obra "obra-serie" declara 30% y no trae IPI: el porcentaje esta',
        ref_titular: "titular:tit-7",
      }),
    );
    expect(d.etiqueta).toBe("Parte sin IPI");
    expect(d.visual).toEqual({ forma: "titular", porcentaje: "30", ipi: null });
  });

  it("de una ONI saca el titulo reportado, con las comillas de Go", () => {
    const d = describirAlerta(
      alerta({
        id: "a",
        tipo: "oni",
        ref_tipo: "uso",
        ref_id: "u-1",
        detalle:
          'la cascada no reconocio "El \\"Patron\\" del mal" (fuente "caracol", entrega "r-1"): queda en la cola manual',
      }),
    );
    expect(d.etiqueta).toBe("Sin obra identificada");
    expect(d.nombre).toBe('El "Patron" del mal');
  });

  it("lo que no se puede leer cae en la etiqueta corta del tipo", () => {
    for (const [tipo, etiqueta] of [
      ["reserva_declaracion_incompleta", "Declaración incompleta"],
      ["titular_sin_porcentaje", "Coautor sin porcentaje"],
      ["oni", "Sin obra identificada"],
      ["duplicado_archivo", "Archivo duplicado"],
      ["duplicado_registro", "Uso duplicado"],
      ["tipo_obra_sin_mapear", "Sin tipo de obra"],
    ] as const) {
      const d = describirAlerta(
        alerta({ id: "a", tipo, detalle: "otra cosa" }),
      );
      expect(d.etiqueta).toBe(etiqueta);
      expect(d.nombre).toBeNull();
    }
    expect(
      describirAlerta(
        alerta({ id: "a", tipo: "reserva_declaracion_incompleta" }),
      ).visual,
    ).toEqual({ forma: "ninguna" });
  });

  it("un coautor ilegible toma el IPI de ref_titular", () => {
    const d = describirAlerta(
      alerta({
        id: "a",
        tipo: "titular_sin_porcentaje",
        detalle: "frase nueva",
        ref_titular: "IPI-9",
      }),
    );
    expect(d.visual).toEqual({
      forma: "titular",
      porcentaje: null,
      ipi: "IPI-9",
    });
  });
});

describe("agruparPorAfectado", () => {
  it("junta las alertas de la misma obra en una tarjeta con un problema por alerta", () => {
    const grupos = agruparPorAfectado([
      alerta({
        id: "a1",
        tipo: "reserva_declaracion_incompleta",
        detalle: RESERVA_60,
      }),
      alerta({ id: "u1", tipo: "oni", ref_tipo: "uso", ref_id: "u-1" }),
      alerta({
        id: "a2",
        tipo: "titular_sin_porcentaje",
        detalle: COAUTOR,
        ref_titular: "ipi:IPI-00000002",
      }),
    ]);
    expect(grupos.map((g) => g.clave)).toEqual(["obra:obra-serie", "uso:u-1"]);
    expect(grupos[0].problemas.map((p) => p.etiqueta)).toEqual([
      "Declaración al 60 %",
      "Coautor sin porcentaje",
    ]);
  });

  it("el mismo id en tablas distintas son dos tarjetas", () => {
    const grupos = agruparPorAfectado([
      alerta({ id: "a", tipo: "oni", ref_tipo: "uso", ref_id: "x" }),
      alerta({
        id: "b",
        tipo: "duplicado_archivo",
        ref_tipo: "reporte",
        ref_id: "x",
      }),
    ]);
    expect(grupos).toHaveLength(2);
  });

  it("pone primero lo que bloquea, despues los avisos y al final lo cerrado", () => {
    const grupos = agruparPorAfectado([
      alerta({
        id: "c",
        tipo: "oni",
        ref_tipo: "uso",
        ref_id: "cerrada",
        resuelta: true,
      }),
      alerta({ id: "o", tipo: "oni", ref_tipo: "uso", ref_id: "aviso" }),
      alerta({
        id: "d",
        tipo: "duplicado_registro",
        ref_tipo: "uso",
        ref_id: "bloquea",
        critica: true,
      }),
      alerta({
        id: "x",
        tipo: "duplicado_archivo",
        ref_tipo: "reporte",
        ref_id: "critica-resuelta",
        critica: true,
        autocerrada: true,
      }),
    ]);
    expect(grupos.map((g) => g.refId)).toEqual([
      "bloquea",
      "aviso",
      "cerrada",
      "critica-resuelta",
    ]);
    expect(grupos.map((g) => g.bloquea)).toEqual([true, false, false, false]);
    expect(grupos.map((g) => g.cerrado)).toEqual([false, false, true, true]);
  });

  it("dentro de la tarjeta el problema que bloquea va primero", () => {
    const [g] = agruparPorAfectado([
      alerta({ id: "o", tipo: "oni", ref_tipo: "uso", ref_id: "u" }),
      alerta({
        id: "d",
        tipo: "duplicado_registro",
        ref_tipo: "uso",
        ref_id: "u",
        critica: true,
      }),
    ]);
    expect(g.problemas.map((p) => p.alerta.id)).toEqual(["d", "o"]);
    expect(g.bloquea).toBe(true);
  });

  it("el nombre de la tarjeta es el primero que se pudo leer", () => {
    const [g] = agruparPorAfectado([
      alerta({
        id: "d",
        tipo: "duplicado_registro",
        ref_tipo: "uso",
        ref_id: "u",
      }),
      alerta({
        id: "o",
        tipo: "oni",
        ref_tipo: "uso",
        ref_id: "u",
        detalle: 'la cascada no reconocio "Pasion de gavilanes" (fuente "rcn")',
      }),
    ]);
    expect(g.nombre).toBe("Pasion de gavilanes");
  });
});

describe("accionDe", () => {
  const una = (a: Alerta) => agruparPorAfectado([a])[0];

  it("una ONI se identifica", () => {
    expect(
      accionDe(
        una(alerta({ id: "a", tipo: "oni", ref_tipo: "uso", ref_id: "u" })),
      ),
    ).toEqual({ etiqueta: "Identificar", ruta: "/identificacion" });
  });

  it("declaracion y coautor abren la declaracion de la obra", () => {
    for (const tipo of [
      "reserva_declaracion_incompleta",
      "titular_sin_porcentaje",
    ] as const) {
      expect(
        accionDe(una(alerta({ id: "a", tipo, ref_id: "obra 1" }))),
      ).toEqual({
        etiqueta: "Abrir declaración",
        ruta: "/catalogo/obra%201/declaracion",
      });
    }
  });

  it("duplicados y tipo sin mapear se revisan en Ingesta", () => {
    for (const tipo of [
      "duplicado_archivo",
      "duplicado_registro",
      "tipo_obra_sin_mapear",
    ] as const) {
      expect(accionDe(una(alerta({ id: "a", tipo, ref_tipo: "uso" })))).toEqual(
        {
          etiqueta: "Revisar en Ingesta",
          ruta: "/ingesta",
        },
      );
    }
  });

  it("la accion la decide el problema que bloquea", () => {
    const [g] = agruparPorAfectado([
      alerta({ id: "o", tipo: "oni", ref_tipo: "uso", ref_id: "u" }),
      alerta({
        id: "d",
        tipo: "duplicado_registro",
        ref_tipo: "uso",
        ref_id: "u",
        critica: true,
      }),
    ]);
    expect(accionDe(g)?.etiqueta).toBe("Revisar en Ingesta");
  });

  it("una tarjeta cerrada no ofrece accion", () => {
    expect(
      accionDe(una(alerta({ id: "a", tipo: "oni", resuelta: true }))),
    ).toBeNull();
  });
});
