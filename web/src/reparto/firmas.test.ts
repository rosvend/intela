import { describe, expect, it } from "vitest";
import { Rol } from "../sesion";
import {
  esCompuerta,
  puedeFirmar,
  rolesFirmados,
  rolesPendientes,
} from "./firmas";
import { Proceso } from "./tipos";

function proceso(parcial: Partial<Proceso> = {}): Proceso {
  return {
    id: "proc-1",
    circuito: "nacional",
    etapa: "verificacion",
    periodo: "2025",
    revision: 1,
    firmas: [],
    ...parcial,
  };
}

describe("esCompuerta", () => {
  it("verificacion y pago_registro son compuertas", () => {
    expect(esCompuerta("verificacion")).toBe(true);
    expect(esCompuerta("pago_registro")).toBe(true);
  });

  it("el resto de etapas no piden firma", () => {
    expect(esCompuerta("recaudo")).toBe(false);
    expect(esCompuerta("liquidacion_final")).toBe(false);
    expect(esCompuerta("auditoria")).toBe(false);
  });
});

describe("rolesPendientes / rolesFirmados", () => {
  it("sin firmas de esta revision, los dos roles estan pendientes", () => {
    expect(rolesPendientes(proceso())).toEqual([
      "distribucion",
      "contabilidad",
    ]);
    expect(rolesFirmados(proceso())).toEqual([]);
  });

  it("una firma de otra revision no cuenta", () => {
    const p = proceso({
      firmas: [{ rol: "distribucion", actor_id: "usr-1", sobre_rev: 1 }],
      revision: 2,
    });
    expect(rolesPendientes(p)).toEqual(["distribucion", "contabilidad"]);
  });

  it("con una firma de esta revision, el otro rol queda pendiente", () => {
    const p = proceso({
      firmas: [{ rol: "distribucion", actor_id: "usr-1", sobre_rev: 1 }],
    });
    expect(rolesFirmados(p)).toEqual(["distribucion"]);
    expect(rolesPendientes(p)).toEqual(["contabilidad"]);
  });
});

describe("puedeFirmar", () => {
  const roles: Rol[] = [
    "administrador",
    "distribucion",
    "contabilidad",
    "auditor",
    "titular",
  ];

  it.each(roles)("el rol %s no firma fuera de una compuerta", (rol) => {
    expect(puedeFirmar(rol, proceso({ etapa: "recaudo" }))).toBe(false);
  });

  it("distribucion y contabilidad firman si su rol sigue pendiente", () => {
    const p = proceso();
    expect(puedeFirmar("distribucion", p)).toBe(true);
    expect(puedeFirmar("contabilidad", p)).toBe(true);
  });

  it("quien ya firmo esta revision no vuelve a firmar", () => {
    const p = proceso({
      firmas: [{ rol: "distribucion", actor_id: "usr-1", sobre_rev: 1 }],
    });
    expect(puedeFirmar("distribucion", p)).toBe(false);
    expect(puedeFirmar("contabilidad", p)).toBe(true);
  });

  it("administrador, auditor y titular no ven el control", () => {
    const p = proceso();
    expect(puedeFirmar("administrador", p)).toBe(false);
    expect(puedeFirmar("auditor", p)).toBe(false);
    expect(puedeFirmar("titular", p)).toBe(false);
  });
});
