/** Cara del asistente: una esfera con dos ojos pequenos e inclinados arriba a la derecha, que parpadean y de vez en cuando miran al otro lado. Decorativa; el boton que la contiene lleva el nombre. */
export default function Mascota() {
  return (
    <svg className="mascota" viewBox="0 0 48 48" aria-hidden="true">
      <defs>
        <radialGradient id="mascota-brillo" cx="35%" cy="30%" r="80%">
          <stop offset="0" stopColor="#9c2444" />
          <stop offset="1" stopColor="#7d1a35" />
        </radialGradient>
      </defs>
      <circle cx="24" cy="24" r="22" fill="url(#mascota-brillo)" />
      <g className="mascota-ojos">
        <g transform="rotate(-20 27.4 14.4)">
          <rect
            className="mascota-ojo"
            x="25.6"
            y="11.2"
            width="3.6"
            height="6.4"
            rx="1.8"
          />
        </g>
        <g transform="rotate(-20 36.8 14.4)">
          <rect
            className="mascota-ojo"
            x="35"
            y="11.2"
            width="3.6"
            height="6.4"
            rx="1.8"
          />
        </g>
      </g>
    </svg>
  );
}
