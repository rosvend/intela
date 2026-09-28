import { describe, expect, it } from "vitest";
import { ApiError, ErrorDeCuerpoIlegible, ErrorDeRed } from "../api";
import {
  avisoDelRechazo,
  comoSeNombraLaFila,
  falloDelGuardado,
  filasDePartida,
  nuevaFila,
  porcentajeDeTexto,
  puedeHaberGuardado,
  textoDePorcentaje,
  tituloDelRechazo,
} from "./borrador";
import type { VersionDeclaracion } from "./tipos";

/**
 * Una version del historial con las tres cosas que estas pruebas miran. Los
 * demas campos van con la forma del contrato para que `typecheck` siga siendo la
 * guarda de la forma.
 */
function version(
  numero: number,
  abierta = false,
  partes: VersionDeclaracion["partes"] = [],
): VersionDeclaracion {
  return {
    version: numero,
    vigente_desde: "2026-01-01T00:00:00Z",
    vigente_hasta: abierta ? null : "2026-02-01T00:00:00Z",
    estado: partes.length > 0 ? "completa" : "incompleta",
    partes,
  };
}

const unaParte = (titularId: string, porcentaje: number) => ({
  titular_id: titularId,
  ipi: "123456789",
  porcentaje,
});

describe("porcentajeDeTexto", () => {
  it("lee la coma como separador decimal", () => {
    // Quien teclea en un teclado en espanol escribe "33,5", y `Number("33,5")`
    // es `NaN`: la fila contaria como cero sin que nadie lo haya escrito.
    expect(porcentajeDeTexto("33,5")).toBe(33.5);
  });

  it("un campo vacio NO es un cero", () => {
    // Con el 0 de `Number("")` el borrador diria que hay un porcentaje
    // declarado donde no hay ninguno, y ese 0 se guardaria como declarado.
    expect(porcentajeDeTexto("")).toBeNaN();
    expect(porcentajeDeTexto("   ")).toBeNaN();
  });

  it("lo que no se lee como numero sale NaN, que es lo que frena el envio", () => {
    expect(porcentajeDeTexto("70%")).toBeNaN();
    expect(porcentajeDeTexto("3.3.3")).toBeNaN();
  });

  it("lee un porcentaje con espacios alrededor, que es lo que deja el pegado", () => {
    expect(porcentajeDeTexto(" 74.5 ")).toBe(74.5);
  });
});

describe("textoDePorcentaje", () => {
  it("es la vuelta atras exacta del numero que llego, sin el signo de porcentaje", () => {
    // El campo no lleva el "%" -no se podria volver a leer- y los cuatro
    // decimales de `formatearPorcentaje` serian ruido en un valor que se edita.
    expect(textoDePorcentaje(74.5)).toBe("74.5");
    expect(textoDePorcentaje(100)).toBe("100");
    expect(textoDePorcentaje(74.5)).not.toContain("%");
  });

  it("lo que escribe se puede volver a leer sin perder la cifra", () => {
    expect(porcentajeDeTexto(textoDePorcentaje(74.5))).toBe(74.5);
    expect(porcentajeDeTexto(textoDePorcentaje(33.3333))).toBe(33.3333);
  });
});

describe("nuevaFila", () => {
  it("una fila nace sin porcentaje: la cifra la escribe quien declara", () => {
    const fila = nuevaFila("tit-1", "123456789", "", "Ana");
    expect(fila).toEqual({
      clave: expect.any(String),
      titularId: "tit-1",
      ipi: "123456789",
      porcentaje: "",
      nombre: "Ana",
    });
    expect(porcentajeDeTexto(fila.porcentaje)).toBeNaN();
  });

  it("cada fila estrena clave, y la clave no es el indice", () => {
    // Con el indice, quitar una fila del medio correria las claves de las de
    // abajo y React reutilizaria el estado de la fila equivocada.
    const primera = nuevaFila("tit-1");
    const segunda = nuevaFila("tit-2");
    const tercera = nuevaFila("tit-3");
    expect(new Set([primera.clave, segunda.clave, tercera.clave]).size).toBe(3);
    // La tercera se crea despues de la segunda y su clave no depende de cuantas
    // filas hay vivas: quitando la primera, las otras dos no se renumeran.
    expect([segunda.clave, tercera.clave]).not.toContain(primera.clave);
  });
});

describe("comoSeNombraLaFila", () => {
  it("nombra con el nombre del titular cuando la fila lo sabe", () => {
    expect(comoSeNombraLaFila(nuevaFila("tit-1", "", "", "Ana Ruiz"))).toBe(
      "Ana Ruiz",
    );
  });

  it("cae al identificador cuando no lo sabe, en vez de a un hueco", () => {
    // La fila que nace del historial no PUEDE tener nombre: la `Parte` no lo
    // lleva (D-006). Un hueco dejaria el boton sin nombre accesible.
    expect(comoSeNombraLaFila(nuevaFila("tit-1", "123456789"))).toBe("tit-1");
  });
});

describe("filasDePartida", () => {
  it("siembra el borrador con el reparto que rige hoy", () => {
    const filas = filasDePartida([
      version(1, false, [unaParte("tit-viejo", 100)]),
      version(2, true, [unaParte("tit-1", 60), unaParte("tit-2", 40)]),
    ]);
    expect(filas.map((fila) => fila.titularId)).toEqual(["tit-1", "tit-2"]);
    expect(filas.map((fila) => fila.porcentaje)).toEqual(["60", "40"]);
  });

  it("las filas del historial nacen sin nombre: la `Parte` no lo lleva", () => {
    // Rellenarlo pidiendo el padron pondria el nombre de HOY a un reparto que es
    // de una version, que es justo lo que la columna rotulada declara (D-006).
    const filas = filasDePartida([version(1, true, [unaParte("tit-1", 100)])]);
    expect(filas.map((fila) => fila.nombre)).toEqual([""]);
    expect(filas.map(comoSeNombraLaFila)).toEqual(["tit-1"]);
  });

  it("sin una version abierta el borrador arranca vacio, no con la ultima", () => {
    expect(filasDePartida([])).toEqual([]);
    expect(filasDePartida([version(1), version(2)])).toEqual([]);
    expect(filasDePartida([version(1, true), version(2, true)])).toEqual([]);
  });
});

describe("puedeHaberGuardado", () => {
  it("un 5xx deja la pregunta abierta: el COMMIT pudo haber entrado", () => {
    expect(puedeHaberGuardado(500)).toBe(true);
    expect(puedeHaberGuardado(502)).toBe(true);
    expect(puedeHaberGuardado(504)).toBe(true);
  });

  it("la falta de respuesta y lo desconocido tambien la dejan abierta", () => {
    expect(puedeHaberGuardado("red")).toBe(true);
    expect(puedeHaberGuardado("desconocido")).toBe(true);
  });

  it("un 4xx la cierra: el servidor contesto y no quedo ninguna version abierta", () => {
    expect(puedeHaberGuardado(400)).toBe(false);
    expect(puedeHaberGuardado(403)).toBe(false);
    expect(puedeHaberGuardado(404)).toBe(false);
  });
});

describe("falloDelGuardado", () => {
  it("un 2xx con el cuerpo ilegible NO es un guardado incierto", () => {
    // `res.ok` prueba que el servidor contesto sin error, o sea que una version
    // se abrio: lo que falta es su numero. Mandarlo a `incierta` afirmaba de
    // menos y nombraba una causa -"una respuesta que se perdio"- que no ocurrio.
    expect(
      falloDelGuardado(new ErrorDeCuerpoIlegible(200, new SyntaxError("x"))),
    ).toEqual({ tipo: "guardadaSinLeer" });
  });

  it("un 4xx se rechaza con el mensaje del servidor, que es el unico que explica la regla", () => {
    expect(falloDelGuardado(new ApiError(400, "la suma pasa de 100"))).toEqual({
      tipo: "rechazada",
      status: 400,
      mensaje: "la suma pasa de 100",
    });
  });

  it("un 5xx es incierto, con su status y su mensaje", () => {
    expect(falloDelGuardado(new ApiError(503, "base caida"))).toEqual({
      tipo: "incierta",
      status: 503,
      mensaje: "base caida",
    });
  });

  it("sin respuesta es incierta y se nombra como red, no como un status", () => {
    expect(falloDelGuardado(new ErrorDeRed(new TypeError("fetch")))).toEqual({
      tipo: "incierta",
      status: "red",
      mensaje: "no se pudo contactar al servidor",
    });
  });

  it("lo que no es ninguno de los tres errores tipados es incierto y desconocido", () => {
    // De un fallo que no es una respuesta del servidor no se sabe nada, y de ahi
    // no se puede afirmar que no se escribiera.
    expect(falloDelGuardado(new TypeError("otra cosa"))).toEqual({
      tipo: "incierta",
      status: "desconocido",
      mensaje: "error desconocido al guardar",
    });
  });
});

describe("tituloDelRechazo", () => {
  it("solo afirma lo que el status dice", () => {
    expect(tituloDelRechazo(400)).toBe("El servidor rechazó la declaración");
    expect(tituloDelRechazo(404)).toBe("Esa obra no está en el catálogo");
    expect(tituloDelRechazo(403)).toBe("El servidor no autoriza este guardado");
    expect(tituloDelRechazo(401)).toBe("El servidor no autoriza este guardado");
  });

  it("ante un 4xx que no es ninguno de los cuatro conocidos no inventa un titulo propio", () => {
    expect(tituloDelRechazo(405)).toBe("El servidor rechazó la declaración");
  });
});

describe("avisoDelRechazo", () => {
  it("en los cuatro 4xx afirma que no se guardo nada", () => {
    // Es lo unico que los cuatro comparten -que no quedo version abierta- aunque
    // el mecanismo sea distinto en cada uno.
    for (const status of [400, 401, 403, 404, 405]) {
      expect(avisoDelRechazo(status)).toContain("No se guardó nada");
    }
  });

  it("el 403 y el 404 dicen por que reintentar no lo mueve", () => {
    // El 403 es de autorizacion -corregir las filas no lo mueve- y el 404 es
    // sobre una obra que no existe: son los dos casos en que reintentar igual
    // no sirve, y el aviso tiene que decirlo.
    expect(avisoDelRechazo(403)).toContain("autorización");
    expect(avisoDelRechazo(404)).toContain("no está en el catálogo");
  });

  it("no promete que el guardado no ocurriera mas alla del 4xx", () => {
    // El texto no puede extenderse al 5xx: ahi el COMMIT pudo haber entrado, y
    // afirmar el no-registro seria el defecto de D-009 otra vez.
    expect(avisoDelRechazo(400)).not.toContain("COMMIT");
    expect(avisoDelRechazo(405)).not.toContain("COMMIT");
  });
});
