/** Cara del asistente: un disco y dos ojos alargados que parpadean. Decorativa; el boton que la contiene lleva el nombre. */
export default function Mascota() {
  return (
    <svg className="mascota" viewBox="0 0 48 48" aria-hidden="true">
      <circle className="mascota-cara" cx="24" cy="24" r="22" />
      <rect
        className="mascota-ojo"
        x="14.5"
        y="16"
        width="6"
        height="15"
        rx="3"
      />
      <rect
        className="mascota-ojo"
        x="27.5"
        y="16"
        width="6"
        height="15"
        rx="3"
      />
    </svg>
  );
}
