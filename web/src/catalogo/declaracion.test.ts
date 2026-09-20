import { describe, expect, it } from "vitest";
import {
  ROTULO_NOMBRE_EN_PADRON_ACTUAL,
  TOLERANCIA_SUMA,
  avisoDeVersion,
  estadoDelBorrador,
  formatearPorcentaje,
  puedeGuardarBorrador,
  totalDeclarado,
  versionAbierta,
  type AvisoDeVersion,
  type MotivoDeDuda,
} from "./declaracion";
import type { VersionDeclaracion } from "./tipos";

/**
 * Una version del historial con las tres cosas que estas pruebas miran: su
 * numero, si es la que rige y su reparto. Los demas campos van con la forma del
 * contrato para que `typecheck` siga siendo la guarda de la forma.
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

describe("totalDeclarado", () => {
  const partes = (...porcentajes: number[]) =>
    porcentajes.map((porcentaje) => ({ porcentaje }));

  it("suma los porcentajes de las filas del borrador", () => {
    expect(totalDeclarado(partes(40, 35.5, 24.5))).toBe(100);
  });

  it("sin filas el total es cero, no NaN", () => {
    expect(totalDeclarado([])).toBe(0);
  });

  it("un porcentaje que no es un numero finito cuenta como cero y no envenena el total", () => {
    // El editor lee el porcentaje de un campo de texto: mientras se teclea, un
    // campo vacio llega como NaN. Si eso entrara en la suma, el total seria NaN
    // y la pantalla no diria nada de las filas que si tienen valor.
    expect(totalDeclarado(partes(60, Number.NaN, 40))).toBe(100);
    expect(totalDeclarado(partes(Number.POSITIVE_INFINITY, 40))).toBe(40);
  });

  it("no modifica la lista que recibe", () => {
    const lista = partes(30, 70);
    const copia = lista.map((p) => ({ ...p }));
    totalDeclarado(lista);
    expect(lista).toEqual(copia);
  });
});

describe("estadoDelBorrador", () => {
  it("sin nada declarado el borrador esta vacio", () => {
    expect(estadoDelBorrador(0)).toBe("vacia");
  });

  it("un total de 100 es completa", () => {
    expect(estadoDelBorrador(100)).toBe("completa");
  });

  it("el ruido de la coma flotante no convierte un 100 exacto en incompleta", () => {
    // 44.289 + 53.6457 + 2.0653 da 99.99999999999999 en coma flotante y 100 en
    // aritmetica decimal. Sin la tolerancia, la pantalla pintaria "100.0000%"
    // junto a "Incompleta": se contradiria con sus propios datos.
    const total = totalDeclarado(
      [44.289, 53.6457, 2.0653].map((porcentaje) => ({ porcentaje })),
    );
    expect(total).not.toBe(100);
    expect(estadoDelBorrador(total)).toBe("completa");
  });

  it("la tolerancia tiene los dos lados: dentro es completa, justo en el limite ya no", () => {
    expect(estadoDelBorrador(100 - TOLERANCIA_SUMA / 2)).toBe("completa");
    expect(estadoDelBorrador(100 + TOLERANCIA_SUMA / 2)).toBe("completa");
    // En el limite exacto la comparacion es estricta (`< TOLERANCIA_SUMA`), asi
    // que cae del lado conservador: no se afirma que sume 100.
    expect(estadoDelBorrador(100 - TOLERANCIA_SUMA)).toBe("incompleta");
    expect(estadoDelBorrador(100 + TOLERANCIA_SUMA)).toBe("excedida");
  });

  it("por debajo de 100 el borrador es incompleta, que es un estado valido", () => {
    // R-04: no se reparte nada de esa obra y el importe completo se retiene.
    // Es un estado del negocio, no un error de quien declara.
    expect(estadoDelBorrador(99.9999)).toBe("incompleta");
    expect(estadoDelBorrador(60)).toBe("incompleta");
    expect(estadoDelBorrador(0.0001)).toBe("incompleta");
  });

  it("por encima de 100 el borrador esta excedido: es lo unico que el backend rechaza", () => {
    expect(estadoDelBorrador(100.0001)).toBe("excedida");
    expect(estadoDelBorrador(120)).toBe("excedida");
  });

  it("un total que no llega a positivo cuenta como borrador vacio y no como guardable", () => {
    // Una fila negativa solo existe mientras se teclea; `NuevaDeclaracion`
    // rechaza ese cuerpo igual (exige partes estrictamente positivas), asi que
    // el borrador no es guardable.
    expect(estadoDelBorrador(-5)).toBe("vacia");
    expect(estadoDelBorrador(Number.NaN)).toBe("vacia");
  });
});

describe("puedeGuardarBorrador", () => {
  it("una declaracion incompleta SI se puede guardar: es la asimetria del contrato", () => {
    // Bloquear la suma menor a 100 contradiria el contrato y haria imposible
    // declarar a proposito por debajo de 100. Solo se bloquea lo que el backend
    // rechaza con 400.
    expect(puedeGuardarBorrador(estadoDelBorrador(60))).toBe(true);
    expect(puedeGuardarBorrador("incompleta")).toBe(true);
    expect(puedeGuardarBorrador(estadoDelBorrador(100))).toBe(true);
  });

  it("un borrador vacio o excedido no se puede guardar: el backend los rechaza", () => {
    expect(puedeGuardarBorrador(estadoDelBorrador(0))).toBe(false);
    expect(puedeGuardarBorrador(estadoDelBorrador(120))).toBe(false);
  });

  it("no existe un quinto estado del borrador que se pueda guardar", () => {
    // El arreglo se prueba contra las cuatro claves del tipo: si alguien anade
    // una quinta, este test cuenta cinco y falla, en vez de dejarla sin
    // decision de guardado.
    const estados = ["vacia", "completa", "incompleta", "excedida"] as const;
    const guardables = estados.filter(puedeGuardarBorrador);
    expect(guardables).toEqual(["completa", "incompleta"]);
  });
});

describe("formatearPorcentaje", () => {
  it("pinta siempre 4 decimales, la precision que guarda la columna", () => {
    expect(formatearPorcentaje(100)).toBe("100.0000%");
    expect(formatearPorcentaje(74.5)).toBe("74.5000%");
    expect(formatearPorcentaje(0)).toBe("0.0000%");
    expect(formatearPorcentaje(33.3333)).toBe("33.3333%");
  });

  it("no redondea hacia arriba un total por debajo de 100: 99.9999 no se pinta como 100", () => {
    // Si lo hiciera, la celda diria 100.0000% junto a un estado "incompleta"
    // que acaba de mandar el backend.
    expect(formatearPorcentaje(99.9999)).toBe("99.9999%");
    expect(formatearPorcentaje(99.99996)).toBe("100.0000%");
  });

  it("escribe el mismo numero igual que el mensaje del backend", () => {
    // `NuevaDeclaracion` contesta con `suma.StringFixed(4)` -"100.0001%"- y las
    // dos cifras pueden acabar en la misma pantalla: punto decimal, cuatro
    // decimales, sin espacio antes del signo.
    expect(formatearPorcentaje(100.0001)).toBe("100.0001%");
    expect(formatearPorcentaje(100.0001)).not.toContain(",");
  });
});

describe("ROTULO_NOMBRE_EN_PADRON_ACTUAL", () => {
  it("rotula la columna como el padron de HOY, que es lo unico que el sistema sabe", () => {
    expect(ROTULO_NOMBRE_EN_PADRON_ACTUAL).toBe("Nombre en el padrón actual");
    expect(ROTULO_NOMBRE_EN_PADRON_ACTUAL).toMatch(/actual/);
  });

  it("no promete el nombre que tenia el titular cuando se declaro", () => {
    // Resolver el nombre contra el padron actual reescribe el pasado: un
    // titular renombrado aparece con su nombre nuevo en las versiones antiguas.
    // El rotulo no puede sugerir que ese nombre es el de la declaracion.
    expect(ROTULO_NOMBRE_EN_PADRON_ACTUAL).not.toMatch(/declarac/i);
    expect(ROTULO_NOMBRE_EN_PADRON_ACTUAL).not.toMatch(/versi/i);
  });
});

describe("versionAbierta", () => {
  it("una sola version sin cerrar es la que rige", () => {
    expect(versionAbierta([version(3), version(4, true)])?.version).toBe(4);
  });

  it("sin ninguna abierta no hay version vigente, y no vale la ultima", () => {
    expect(versionAbierta([version(3), version(4)])).toBeNull();
    expect(versionAbierta([])).toBeNull();
  });

  it("con dos abiertas no se elige ninguna: el backend cerraria otra cosa", () => {
    // El numero que sale de aqui es el que el aviso enseña como "se cerrara la
    // version N". Con dos candidatas, afirmar una seria afirmar de mas.
    expect(versionAbierta([version(3, true), version(4, true)])).toBeNull();
  });
});

/**
 * El desenlace, en una linea. La tabla de abajo compara esto y no el objeto
 * entero por una razon de lectura: un caso por renglon se lee de un vistazo y un
 * objeto de cuatro campos por renglon no. La proyeccion es LOSSLESS -lleva los
 * tres `tipo`, los cuatro `porque` de `sinNumeros` y los dos booleanos-, asi que
 * no hay ningun campo del aviso que la tabla deje de comparar.
 */
function desenlace(aviso: AvisoDeVersion): string {
  if (aviso.tipo === "cierraYabre") {
    return `cierraYabre ${aviso.seCierra}->${aviso.seAbre}`;
  }
  if (aviso.tipo === "abreLaPrimera") return "abreLaPrimera";
  return `sinNumeros/${aviso.porque}/sinBorrador=${aviso.sinBorrador}/rechazo=${aviso.hayRechazoPosterior}`;
}

/** Las cuatro entradas del aviso, en el orden de su firma. */
type Entradas = [
  historial: readonly VersionDeclaracion[] | null,
  versionGuardada: number | null,
  dudaPendiente: MotivoDeDuda | null,
  hayRechazoPosterior: boolean,
];

describe("avisoDeVersion", () => {
  // La tabla de verdad, un caso por renglon: [nombre, entradas, desenlace].
  //
  // Seis `return` y SIETE desenlaces observables, porque el primero de ellos
  // -la rama de la duda pendiente- devuelve dos `porque` distintos. Los siete:
  //   cierraYabre (numeros del `PUT`)      cierraYabre (numeros del historial)
  //   abreLaPrimera
  //   sinNumeros/historialNoLeido          sinNumeros/sinVersionAbierta
  //   sinNumeros/guardadoSinLeer           sinNumeros/guardadoIncierto
  //
  // Los cuatro primeros casos son el CONTROL: el camino comun, que tiene que
  // seguir dando lo mismo. Del 5 al 12 son las PRECEDENCIAS -los casos en que dos
  // desenlaces compiten y gana uno-, que es la parte que costo tres iteraciones
  // del PR #30 y la que un test de integracion puede cumplir de casualidad.
  // Los dos ultimos completan `sinVersionAbierta`, que no aparece antes.
  //
  // `prettier-ignore` esta aqui a proposito: esta tabla se mantiene a mano, un
  // caso por renglon, y el formateador la partiria en nueve renglones por caso.
  /* prettier-ignore */
  const TABLA: readonly [string, Entradas, string][] = [
    ["control: la abierta del historial, sin guardado todavia", [[version(3, true)], null, null, false], "cierraYabre 3->4"],
    ["control: historial vacio, la obra no tiene declaracion", [[], null, null, false], "abreLaPrimera"],
    ["control: historial ilegible, no se afirma ningun numero", [null, null, null, false], "sinNumeros/historialNoLeido/sinBorrador=true/rechazo=false"],
    ["control: el 200 legible manda sobre el historial en memoria", [[version(3, true)], 7, null, false], "cierraYabre 7->8"],
    ["precedencia: la duda manda sobre la version que devolvio el PUT", [[version(3, true)], 7, "guardadoIncierto", false], "sinNumeros/guardadoIncierto/sinBorrador=false/rechazo=false"],
    ["precedencia: la duda manda sobre el PUT, tambien la del 2xx ilegible", [[version(3, true)], 7, "guardadoSinLeer", false], "sinNumeros/guardadoSinLeer/sinBorrador=false/rechazo=false"],
    ["precedencia: sin PUT, la duda manda sobre el historial", [[version(3, true)], null, "guardadoIncierto", false], "sinNumeros/guardadoIncierto/sinBorrador=false/rechazo=false"],
    ["precedencia: la duda con el historial ilegible deja sinBorrador", [null, null, "guardadoSinLeer", false], "sinNumeros/guardadoSinLeer/sinBorrador=true/rechazo=false"],
    ["precedencia: el PUT manda sobre un historial vacio", [[], 7, null, false], "cierraYabre 7->8"],
    ["precedencia: el PUT manda sobre un historial ilegible", [null, 7, null, false], "cierraYabre 7->8"],
    ["precedencia: el rechazo posterior no se dice fuera de la rama de duda", [[version(3, true)], null, null, true], "cierraYabre 3->4"],
    ["precedencia: y dentro de la rama de duda si se dice", [[version(3, true)], null, "guardadoIncierto", true], "sinNumeros/guardadoIncierto/sinBorrador=false/rechazo=true"],
    ["dos versiones abiertas: no se sabe cual se cierra", [[version(3, true), version(4, true)], null, null, false], "sinNumeros/sinVersionAbierta/sinBorrador=true/rechazo=false"],
    ["ninguna abierta: tampoco, aunque el historial tenga versiones", [[version(3), version(4)], null, null, false], "sinNumeros/sinVersionAbierta/sinBorrador=true/rechazo=false"],
  ];

  it.each(TABLA)("%s", (_nombre, entradas, esperado) => {
    expect(desenlace(avisoDeVersion(...entradas))).toBe(esperado);
  });

  it("la forma completa del aviso, no solo su resumen", () => {
    // El resumen que compara la tabla es lossless, pero si el tipo ganara un
    // campo, el resumen y la funcion podrian quedarse quietos los dos y seguir
    // coincidiendo. Estas dos formas fijan el objeto entero.
    expect(avisoDeVersion([version(3, true)], null, null, false)).toEqual({
      tipo: "cierraYabre",
      seCierra: 3,
      seAbre: 4,
    });
    expect(
      avisoDeVersion([version(3, true)], 7, "guardadoIncierto", true),
    ).toEqual({
      tipo: "sinNumeros",
      porque: "guardadoIncierto",
      sinBorrador: false,
      hayRechazoPosterior: true,
    });
  });
});
