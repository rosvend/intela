import { Bolsa } from "./recaudo";

/**
 * Datos de demostracion: SOLO para la tarjeta de recaudo cuando `/api/bolsas`
 * no trae nada. La tarjeta los marca con el chip "Datos de demostración".
 * No son cifras del cliente.
 */
export const BOLSAS_DEMO: readonly Bolsa[] = [
  demo("caracol", "nacional", "600000000.00"),
  demo("rcn", "nacional", "600000000.00"),
  demo("netflix", "nacional", "350000000.00"),
  demo("procinal", "nacional", "200000000.00"),
  demo("dago", "internacional", "100000000.00"),
  demo("transporte", "nacional", "40000000.00"),
];

function demo(
  usuario_id: string,
  circuito: Bolsa["circuito"],
  bruto: string,
): Bolsa {
  return {
    id: `demo-${usuario_id}-2025`,
    usuario_id,
    periodo: "2025",
    circuito,
    bruto,
  };
}
