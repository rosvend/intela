import { PanelIngresos } from "./PanelIngresos";
import { useSesion } from "./sesion";

/**
 * "/" no cambia de ruta segun el rol (M-5): el mockup no le da al titular un
 * item de nav propio, cambia el CONTENIDO de Inicio. El panel de
 * administracion real (KPIs, graficos) llega con #31; esto es el esqueleto
 * que ese PR reemplaza. El panel del titular (OE-6) ya aterriza aqui:
 * ingresos netos por obra, fuente y periodo, con ExplicarCifra.
 */
export default function Inicio() {
  const { usuario } = useSesion();

  if (usuario?.rol === "titular") {
    return <PanelIngresos />;
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
