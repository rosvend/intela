import { describe, expect, it } from "vitest";
import {
  FILTROS_VACIOS,
  diferenciaDePartes,
  esAsiento,
  familiaDeHecho,
  filtrarAsientos,
  leerPartes,
  type Asiento,
} from "./tipos";

function asiento(parcial: Partial<Asiento> & { id: string }): Asiento {
  return {
    hecho: "declaracion.guardada",
    ref_tipo: "obra",
    ref_id: "obra-1",
    actor: "usr-admin",
    payload: {},
    cuando: "2026-04-02T10:30:00Z",
    ...parcial,
  };
}

describe("esAsiento", () => {
  it("acepta un asiento bien formado", () => {
    expect(esAsiento(asiento({ id: "a-1" }))).toBe(true);
  });

  it("rechaza lo que no es objeto y los campos que no son texto", () => {
    expect(esAsiento(null)).toBe(false);
    expect(esAsiento([])).toBe(false);
    expect(esAsiento(asiento({ id: 7 as unknown as string }))).toBe(false);
    expect(
      esAsiento(asiento({ id: "x", cuando: 20260402 as unknown as string })),
    ).toBe(false);
  });

  it("rechaza un payload nulo: el detalle lo recorre como objeto", () => {
    expect(
      esAsiento(
        asiento({ id: "x", payload: null as unknown as Asiento["payload"] }),
      ),
    ).toBe(false);
  });
});

describe("familiaDeHecho", () => {
  it("clasifica por prefijo de modulo", () => {
    expect(familiaDeHecho("obra.registrada")).toBe("catalogo");
    expect(familiaDeHecho("declaracion.guardada")).toBe("splits");
    expect(familiaDeHecho("reparto.corrida_cerrada")).toBe("distribucion");
    expect(familiaDeHecho("recaudo.registrado")).toBe("recaudo");
    expect(familiaDeHecho("matching.cascada")).toBe("identificacion");
  });

  it("un hecho futuro se sigue mostrando como otro, no se esconde", () => {
    expect(familiaDeHecho("reclamo.respondido")).toBe("otro");
  });
});

describe("filtrarAsientos", () => {
  const asientos = [
    asiento({
      id: "a-1",
      hecho: "declaracion.guardada",
      actor: "usr-admin",
      cuando: "2026-03-01T09:00:00Z",
    }),
    asiento({
      id: "a-2",
      hecho: "recaudo.registrado",
      ref_tipo: "bolsa",
      ref_id: "bolsa-1",
      actor: "usr-contable",
      cuando: "2026-04-02T10:30:00Z",
    }),
    asiento({
      id: "a-3",
      hecho: "declaracion.guardada",
      ref_id: "obra-2",
      actor: "usr-admin",
      cuando: "2026-05-03T10:30:00Z",
    }),
  ];

  it("sin filtros devuelve la pagina entera", () => {
    expect(filtrarAsientos(asientos, FILTROS_VACIOS)).toHaveLength(3);
  });

  it("filtra por familia sin nombrar el hecho crudo", () => {
    const got = filtrarAsientos(asientos, {
      ...FILTROS_VACIOS,
      familia: "recaudo",
    });
    expect(got.map((a) => a.id)).toEqual(["a-2"]);
  });

  it("filtra por rango de fechas contra el dia del cuando", () => {
    const got = filtrarAsientos(asientos, {
      ...FILTROS_VACIOS,
      desde: "2026-04-01",
      hasta: "2026-04-30",
    });
    expect(got.map((a) => a.id)).toEqual(["a-2"]);
  });

  it("el actor es parcial y sin distinguir mayusculas", () => {
    const got = filtrarAsientos(asientos, {
      ...FILTROS_VACIOS,
      actor: "CONTABLE",
    });
    expect(got.map((a) => a.id)).toEqual(["a-2"]);
  });

  it("la obra solo cruza contra referencias de obra", () => {
    const porObra = filtrarAsientos(asientos, {
      ...FILTROS_VACIOS,
      obra: "bolsa",
    });
    expect(porObra).toHaveLength(0);
    const porObra2 = filtrarAsientos(asientos, {
      ...FILTROS_VACIOS,
      obra: "OBRA-2",
    });
    expect(porObra2.map((a) => a.id)).toEqual(["a-3"]);
  });
});

describe("leerPartes", () => {
  it("lee las claves PascalCase del marshal de Go", () => {
    expect(
      leerPartes({
        version: 2,
        estado: "completa",
        partes: [
          { TitularID: "t1", IPI: "IPI-1", Porcentaje: 60 },
          { TitularID: "t2", IPI: "IPI-2", Porcentaje: 40 },
        ],
      }),
    ).toEqual([
      { titularId: "t1", ipi: "IPI-1", porcentaje: 60 },
      { titularId: "t2", ipi: "IPI-2", porcentaje: 40 },
    ]);
  });

  it("devuelve null si las partes no tienen forma de reparto", () => {
    expect(leerPartes({})).toBeNull();
    expect(leerPartes({ partes: [{ TitularID: "t1" }] })).toBeNull();
    expect(leerPartes(null)).toBeNull();
  });
});

describe("diferenciaDePartes", () => {
  it("distingue parte nueva, retirada y cambio de porcentaje", () => {
    expect(
      diferenciaDePartes(
        [
          { titularId: "t1", ipi: "IPI-1", porcentaje: 60 },
          { titularId: "t2", ipi: "IPI-2", porcentaje: 40 },
        ],
        [
          { titularId: "t1", ipi: "IPI-1", porcentaje: 50 },
          { titularId: "t3", ipi: "IPI-3", porcentaje: 50 },
        ],
      ),
    ).toEqual([
      { titularId: "t1", ipi: "IPI-1", antes: 60, despues: 50 },
      { titularId: "t2", ipi: "IPI-2", antes: 40, despues: null },
      { titularId: "t3", ipi: "IPI-3", antes: null, despues: 50 },
    ]);
  });

  it("sin cambios no hay filas que mostrar", () => {
    expect(
      diferenciaDePartes(
        [{ titularId: "t1", ipi: "IPI-1", porcentaje: 100 }],
        [{ titularId: "t1", ipi: "IPI-1", porcentaje: 100 }],
      ),
    ).toEqual([]);
  });
});
