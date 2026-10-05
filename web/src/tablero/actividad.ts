import { Asiento, ETIQUETA_FAMILIA, familiaDeHecho } from "../auditoria/tipos";
import { ETIQUETA_ETAPA } from "../reparto/etapas";
import { ETIQUETA_ROL_FIRMA } from "../reparto/firmas";
import { formatearImporte } from "./formato";
import { etiquetaDeFuente } from "./recaudo";

function texto(payload: Asiento["payload"], clave: string): string {
  const valor = (payload as Record<string, unknown>)[clave];
  return typeof valor === "string" ? valor : "";
}

function etapa(payload: Asiento["payload"]): string {
  const clave = texto(payload, "etapa");
  return (ETIQUETA_ETAPA as Record<string, string>)[clave] ?? "una etapa";
}

/** Un asiento de la bitacora en una frase que se entiende sin conocer el modelo. */
export function describirAsiento(a: Asiento): string {
  const p = a.payload;
  switch (a.hecho) {
    case "declaracion.guardada":
      return texto(p, "estado") === "incompleta"
        ? `La declaración de ${a.ref_id} no suma 100%: la obra queda en reserva`
        : `Se guardó la declaración de ${a.ref_id}`;
    case "recaudo.registrado": {
      const fuente = etiquetaDeFuente(texto(p, "usuario_id") || a.ref_id);
      const bruto = texto(p, "bruto");
      return bruto
        ? `Llegó el recaudo de ${fuente}: ${formatearImporte(bruto)}`
        : `Llegó el recaudo de ${fuente}`;
    }
    case "proceso.abierto":
      return "Se abrió una distribución";
    case "proceso.etapa_avanzada":
      return `La distribución avanzó a ${etapa(p)}`;
    case "firma.registrada": {
      const rol =
        (ETIQUETA_ROL_FIRMA as Record<string, string>)[texto(p, "rol")] ??
        "Alguien";
      return `${rol} firmó ${etapa(p)}`;
    }
    case "proceso.gate_rechazado":
      return `Se rechazó ${etapa(p)} y la distribución volvió atrás`;
    case "reparto.valorizado":
      return "Se calculó el reparto de una bolsa";
    case "reparto.obra_valorizada":
      return `Se valorizó la obra ${a.ref_id}`;
    case "identificacion.asignada":
      return "Se identificó un uso que estaba sin obra";
    case "identificacion.descartada":
      return "Se descartó un uso sin identificar";
  }
  const familia = familiaDeHecho(a.hecho);
  return familia === "otro"
    ? a.hecho
    : `Movimiento en ${ETIQUETA_FAMILIA[familia]}`;
}
