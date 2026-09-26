import { api } from "../api";
import { PedidoDeFirma, RUTAS_REPARTO } from "./tipos";

/**
 * Firma o rechaza la compuerta actual. El cuerpo discrimina la accion
 * porque #40 manda las dos por `POST /procesos/{id}/firmar`. El resultado
 * no se usa: quien llama recarga el listado, que es la fuente de la etapa.
 */
export async function firmarProceso(
  id: string,
  pedido: PedidoDeFirma,
): Promise<void> {
  await api(RUTAS_REPARTO.firmar(id), {
    method: "POST",
    body: JSON.stringify(pedido),
  });
}
