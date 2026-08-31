import { useSesion } from "./sesion";

/**
 * "/" no cambia de ruta segun el rol (M-5): el mockup no le da al titular un
 * item de nav propio, cambia el CONTENIDO de Inicio. El panel de
 * administracion real (KPIs, graficos) llega con #31; esto es el esqueleto
 * que ese PR reemplaza.
 */
export default function Inicio() {
  const { usuario } = useSesion();

  if (usuario?.rol === "titular") {
    return (
      <section>
        <h1>Mi liquidación</h1>
        <p className="muted">
          Ingresos, deducciones y detalle por obra de tus periodos de reparto.
        </p>
      </section>
    );
  }

  return (
    <section>
      <h1>Panel de control</h1>
      <p className="muted">
        Reconocimiento de obras y distribución de ingresos por propiedad
        intelectual para REDES SGC.
      </p>
    </section>
  );
}
