import { describe, expect, it } from "vitest";
import { Asiento } from "../auditoria/tipos";
import { describirAsiento } from "./actividad";

function asiento(parcial: Partial<Asiento>): Asiento {
  return {
    id: "a1",
    hecho: "obra.creada",
    ref_tipo: "obra",
    ref_id: "obra-1",
    actor: "usr-admin",
    payload: {},
    cuando: "2026-10-05T10:00:00Z",
    ...parcial,
  };
}

describe("describirAsiento", () => {
  it("una declaracion completa y una incompleta se dicen distinto", () => {
    expect(
      describirAsiento(
        asiento({
          hecho: "declaracion.guardada",
          payload: { estado: "completa" },
        }),
      ),
    ).toBe("Se guardó la declaración de obra-1");
    expect(
      describirAsiento(
        asiento({
          hecho: "declaracion.guardada",
          payload: { estado: "incompleta" },
        }),
      ),
    ).toBe("La declaración de obra-1 no suma 100%: la obra queda en reserva");
  });

  it("el recaudo nombra la fuente y el importe", () => {
    expect(
      describirAsiento(
        asiento({
          hecho: "recaudo.registrado",
          ref_tipo: "bolsa",
          ref_id: "bolsa-caracol",
          payload: { usuario_id: "caracol", bruto: "600000000.00" },
        }),
      ),
    ).toBe("Llegó el recaudo de Caracol: $ 600.000.000");
  });

  it("la distribucion habla de etapas con su nombre, no con su clave", () => {
    expect(
      describirAsiento(
        asiento({
          hecho: "proceso.etapa_avanzada",
          payload: { etapa: "liquidacion_parcial" },
        }),
      ),
    ).toBe("La distribución avanzó a Liquidación parcial");
    expect(
      describirAsiento(
        asiento({
          hecho: "firma.registrada",
          payload: { rol: "contabilidad", etapa: "verificacion" },
        }),
      ),
    ).toBe("Contabilidad firmó Verificación");
  });

  it("un hecho desconocido se sigue mostrando, por su familia", () => {
    expect(describirAsiento(asiento({ hecho: "oni.algo_nuevo" }))).toBe(
      "Movimiento en Identificación",
    );
    expect(describirAsiento(asiento({ hecho: "x.y" }))).toBe("x.y");
  });
});
