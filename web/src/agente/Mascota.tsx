/** Cara del asistente: una esfera con dos ojos pequenos e inclinados arriba a la derecha, que parpadean y de vez en cuando miran al otro lado. Decorativa; el boton que la contiene lleva el nombre. */
export default function Mascota() {
  return (
    <svg className="mascota" viewBox="0 0 48 48" aria-hidden="true">
      <circle className="mascota-cara" cx="24" cy="24" r="22" />
      <g className="mascota-ojos">
        <g transform="rotate(-20 27.4 14.4)">
          <rect
            className="mascota-ojo"
            x="24.9"
            y="10.1"
            width="5"
            height="8.6"
            rx="2.5"
          />
        </g>
        <g transform="rotate(-20 36.8 14.4)">
          <rect
            className="mascota-ojo"
            x="34.3"
            y="10.1"
            width="5"
            height="8.6"
            rx="2.5"
          />
        </g>
      </g>
    </svg>
  );
}
