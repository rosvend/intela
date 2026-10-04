import { describe, expect, it } from "vitest";
import {
  TIPOS_DE_ALERTA,
  advertenciaDeCompuerta,
  conteoDeTipo,
  etiquetaDeTipo,
  puedeEvaluar,
} from "./anomalias";
import { resumenDePrueba } from "./resumenDePrueba";

describe("tipos de alerta", () => {
  it("son los seis del contrato y terminan en tipo_obra_sin_mapear", () => {
    expect(TIPOS_DE_ALERTA).toHaveLength(6);
    expect(TIPOS_DE_ALERTA[5]).toBe("tipo_obra_sin_mapear");
    expect(etiquetaDeTipo("tipo_obra_sin_mapear")).toBe(
      "Tipo de obra sin mapear",
    );
  });

  it("un tipo desconocido se pinta crudo", () => {
    expect(etiquetaDeTipo("nuevo")).toBe("nuevo");
  });
});

describe("conteoDeTipo", () => {
  it("lee las abiertas de por_tipo", () => {
    const r = resumenDePrueba({ porTipo: { oni: 7 } });
    expect(conteoDeTipo({ tipo: "listo", datos: r }, "oni")).toEqual({
      tipo: "listo",
      datos: { total: 7 },
    });
  });

  it("pasa los demas estados tal cual", () => {
    expect(conteoDeTipo({ tipo: "cargando" }, "oni")).toEqual({
      tipo: "cargando",
    });
    expect(conteoDeTipo({ tipo: "ausente" }, "oni")).toEqual({
      tipo: "ausente",
    });
    expect(conteoDeTipo({ tipo: "inactivo" }, "oni")).toEqual({
      tipo: "inactivo",
    });
    expect(conteoDeTipo({ tipo: "error", mensaje: "x" }, "oni")).toEqual({
      tipo: "error",
      mensaje: "x",
    });
  });
});

describe("advertenciaDeCompuerta", () => {
  it("un periodo sin evaluar no se presenta como limpio", () => {
    const texto = advertenciaDeCompuerta(
      resumenDePrueba({ ultima_evaluacion: null }),
    );
    expect(texto).toContain("no se ha evaluado");
    expect(texto).toContain("Evalúalo antes de firmar");
  });

  it("distingue las criticas de las abiertas en total", () => {
    const texto = advertenciaDeCompuerta(
      resumenDePrueba({ abiertas: 250, criticas_abiertas: 150 }),
    );
    expect(texto).toContain("150 críticas bloquean");
    expect(texto).toContain("250 alertas abiertas");
  });

  it("usa el singular", () => {
    const texto = advertenciaDeCompuerta(
      resumenDePrueba({ abiertas: 1, criticas_abiertas: 1 }),
    );
    expect(texto).toContain("1 crítica bloquea");
    expect(texto).toContain("1 alerta abierta en total");
  });

  it("sin criticas dice que ninguna bloquea", () => {
    const texto = advertenciaDeCompuerta(resumenDePrueba({ abiertas: 3 }));
    expect(texto).toBe("3 alertas abiertas; ninguna bloquea la corrida.");
  });

  it("un periodo limpio no avisa", () => {
    expect(advertenciaDeCompuerta(resumenDePrueba())).toBe("");
  });

  it("agrega las criticas aceptadas sin corregir", () => {
    expect(
      advertenciaDeCompuerta(
        resumenDePrueba({ abiertas: 3, criticas_aceptadas: 2 }),
      ),
    ).toBe(
      "3 alertas abiertas; ninguna bloquea la corrida. 2 críticas se aceptaron sin corregir el dato.",
    );
  });

  it("limpio con aceptadas: solo el sufijo, sin espacio inicial", () => {
    expect(
      advertenciaDeCompuerta(resumenDePrueba({ criticas_aceptadas: 1 })),
    ).toBe("1 crítica se aceptó sin corregir el dato.");
  });
});

describe("puedeEvaluar", () => {
  it("solo administrador y distribucion", () => {
    expect(puedeEvaluar("administrador")).toBe(true);
    expect(puedeEvaluar("distribucion")).toBe(true);
    expect(puedeEvaluar("contabilidad")).toBe(false);
    expect(puedeEvaluar("auditor")).toBe(false);
    expect(puedeEvaluar("titular")).toBe(false);
  });
});
