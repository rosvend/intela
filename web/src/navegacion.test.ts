import { describe, expect, it } from "vitest";
import { RUTAS, itemsDeNav, puedeVer } from "./navegacion";
import { Rol } from "./sesion";

const TODAS_LAS_RUTAS = RUTAS.map((r) => r.to);
const TODOS_LOS_ROLES: readonly Rol[] = [
  "administrador",
  "distribucion",
  "contabilidad",
  "auditor",
  "titular",
];

describe("itemsDeNav", () => {
  it("el titular ve exactamente un item: Inicio, sin ruta propia mas alla (M-5)", () => {
    expect(itemsDeNav("titular").map((r) => r.to)).toEqual(["/"]);
  });

  it("el administrador ve las nueve rutas de PRINCIPAL y CONFIGURACION (Inicio incluido)", () => {
    expect(itemsDeNav("administrador").map((r) => r.to)).toEqual(
      TODAS_LAS_RUTAS,
    );
  });

  it("el auditor ve todo lo del administrador salvo /ingesta y /catalogo (solo lectura no es subir ni administrar el catalogo)", () => {
    const delAdministrador = itemsDeNav("administrador").map((r) => r.to);
    expect(itemsDeNav("auditor").map((r) => r.to)).toEqual(
      delAdministrador.filter((to) => to !== "/ingesta" && to !== "/catalogo"),
    );
  });

  it("distribucion no ve /reportes (es de contabilidad)", () => {
    expect(itemsDeNav("distribucion").map((r) => r.to)).not.toContain(
      "/reportes",
    );
  });

  it("contabilidad no ve /distribucion (es de distribucion): las dos firmas no se solapan", () => {
    expect(itemsDeNav("contabilidad").map((r) => r.to)).not.toContain(
      "/distribucion",
    );
  });

  it("solo administrador y auditor llegan a la seccion Configuracion", () => {
    const configuracion = RUTAS.filter((r) => r.seccion === "configuracion");
    for (const rol of TODOS_LOS_ROLES) {
      const ve = itemsDeNav(rol).some((r) => r.seccion === "configuracion");
      expect(ve).toBe(rol === "administrador" || rol === "auditor");
    }
    expect(configuracion.map((r) => r.to)).toEqual([
      "/deducciones",
      "/auditoria",
    ]);
  });

  it("cada item trae la seccion correcta segun la tabla del mockup", () => {
    const principal = [
      "/",
      "/ingesta",
      "/catalogo",
      "/titulares",
      "/distribucion",
      "/anomalias",
      "/reportes",
    ];
    const configuracion = ["/deducciones", "/auditoria"];
    for (const to of principal) {
      expect(RUTAS.find((r) => r.to === to)?.seccion).toBe("principal");
    }
    for (const to of configuracion) {
      expect(RUTAS.find((r) => r.to === to)?.seccion).toBe("configuracion");
    }
  });
});

describe("puedeVer", () => {
  it("dice que si cuando el rol esta entre los permitidos de esa ruta", () => {
    expect(puedeVer("administrador", "/auditoria")).toBe(true);
  });

  it("dice que no cuando el rol no esta permitido", () => {
    expect(puedeVer("titular", "/auditoria")).toBe(false);
  });

  it("dice que no para una ruta que no existe en la tabla", () => {
    expect(puedeVer("administrador", "/mis-obras")).toBe(false);
  });

  it("/ingesta es solo del administrador, como /reportes en el servidor", () => {
    for (const rol of TODOS_LOS_ROLES) {
      expect(puedeVer(rol, "/ingesta"), rol).toBe(rol === "administrador");
    }
  });

  it("/catalogo es solo del administrador, como el requiereRol del grupo /obras (#30, D-013)", () => {
    // Distribucion y auditor no lo ven porque el servidor les responde 403 en
    // todas las pantallas de #30: el catalogo no se les ofrece para no anunciar
    // una pantalla que el sistema no les da.
    for (const rol of TODOS_LOS_ROLES) {
      expect(puedeVer(rol, "/catalogo"), rol).toBe(rol === "administrador");
    }
  });
});
