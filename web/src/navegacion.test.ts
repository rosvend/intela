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

  it("el auditor ve todo lo del administrador salvo /ingesta, /catalogo y /titulares (solo lectura no es subir, ni administrar el catalogo, ni llevarse el padron entero)", () => {
    const delAdministrador = itemsDeNav("administrador").map((r) => r.to);
    expect(itemsDeNav("auditor").map((r) => r.to)).toEqual(
      delAdministrador.filter(
        (to) => to !== "/ingesta" && to !== "/catalogo" && to !== "/titulares",
      ),
    );
  });

  it("contabilidad y auditor no ven /titulares, y el administrador si: es lo que el servidor sirve (X2, D-014)", () => {
    for (const rol of ["contabilidad", "auditor"] as const) {
      const suyas = itemsDeNav(rol).map((r) => r.to);
      // Precondicion: si la lista llegara vacia -por un fallo de la funcion o
      // del propio test-, "no ve /titulares" seria cierto por el motivo
      // equivocado en los dos roles.
      expect(suyas.length, rol).toBeGreaterThan(0);
      expect(suyas, rol).not.toContain("/titulares");
    }
    const delAdministrador = itemsDeNav("administrador").map((r) => r.to);
    expect(delAdministrador.length).toBeGreaterThan(0);
    expect(delAdministrador).toContain("/titulares");
  });

  it("control negativo del anterior: /distribucion sigue visible para auditor, asi que el cambio no vacio la navegacion", () => {
    // Sin este control, "auditor no ve /titulares" pasaria igual si el arreglo
    // hubiera dejado a auditor sin ninguna ruta.
    const suyas = itemsDeNav("auditor").map((r) => r.to);
    expect(suyas.length).toBeGreaterThan(0);
    expect(suyas).toContain("/distribucion");
  });

  it("distribucion no ve /reportes (es de contabilidad)", () => {
    expect(itemsDeNav("distribucion").map((r) => r.to)).not.toContain(
      "/reportes",
    );
  });

  it("contabilidad ve /distribucion: es la otra firma de la compuerta", () => {
    expect(itemsDeNav("contabilidad").map((r) => r.to)).toContain(
      "/distribucion",
    );
  });

  it("contabilidad ve /anomalias: es la segunda firma y necesita el aviso de alertas", () => {
    expect(itemsDeNav("contabilidad").map((r) => r.to)).toContain("/anomalias");
    expect(puedeVer("contabilidad", "/anomalias")).toBe(true);
  });

  it("las dos firmas no se solapan en reportes: distribucion no los ve", () => {
    expect(itemsDeNav("contabilidad").map((r) => r.to)).toContain("/reportes");
    expect(itemsDeNav("distribucion").map((r) => r.to)).not.toContain(
      "/reportes",
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

  it("/titulares es solo del administrador, como el requiereRol del grupo /titulares (#30, D-014)", () => {
    // Contabilidad y auditor no lo ven porque el servidor les responde 403 en
    // el padron: no se les ofrece para no anunciar una pantalla que el sistema
    // no les da. La segunda puerta por la que se pregunta lo mismo que
    // `itemsDeNav` tiene que decir lo mismo.
    for (const rol of TODOS_LOS_ROLES) {
      expect(puedeVer(rol, "/titulares"), rol).toBe(rol === "administrador");
    }
  });
});
