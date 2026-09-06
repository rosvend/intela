import marca from "./marca-intela.png";

/**
 * Indicador de carga: la marca de Intela girando.
 *
 * La imagen se aplica como MASCARA, no como <img>: el PNG oficial es rosa
 * (`#ECA0B6`) y el resto del tablero es vinotinto. Enmascarar usa solo su
 * canal alfa y pinta la figura con `--acento`, asi que el color sale de los
 * tokens y no del activo. Si algun dia cambia la paleta, esto la sigue.
 *
 * `role="status"` con texto real porque una figura girando no le dice nada a
 * quien usa un lector de pantalla.
 */
export default function Cargando({ texto = "Cargando…" }: { texto?: string }) {
  return (
    <div className="cargando" role="status">
      <span
        className="logo-spinner"
        style={{ maskImage: `url(${marca})`, WebkitMaskImage: `url(${marca})` }}
        aria-hidden="true"
      />
      <p>{texto}</p>
    </div>
  );
}
