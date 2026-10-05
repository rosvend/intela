import type { ReactElement, ReactNode } from "react";

/** Iconos de trazo de 2px, decorativos: el texto de al lado dice lo mismo. */
function Icono({
  children,
  tamano = 18,
}: {
  children: ReactNode;
  tamano?: number;
}): ReactElement {
  return (
    <svg
      viewBox="0 0 24 24"
      width={tamano}
      height={tamano}
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

export const IconoInfo = () => (
  <Icono tamano={16}>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 11v5M12 8h.01" />
  </Icono>
);

export const IconoPregunta = () => (
  <Icono tamano={16}>
    <circle cx="12" cy="12" r="9" />
    <path d="M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.6.3-1 .8-1 1.5v.2M12 17h.01" />
  </Icono>
);

export const IconoCheck = ({ tamano }: { tamano?: number }) => (
  <Icono tamano={tamano}>
    <path d="M5 12.5l4.5 4.5L19 7.5" />
  </Icono>
);

export const IconoPantalla = () => (
  <Icono>
    <rect x="3" y="4" width="18" height="13" rx="2" />
    <path d="M8 21h8M12 17v4" />
  </Icono>
);

export const IconoLupa = () => (
  <Icono tamano={16}>
    <circle cx="11" cy="11" r="6.5" />
    <path d="M20 20l-4.2-4.2" />
  </Icono>
);

export const IconoDescartar = () => (
  <Icono tamano={16}>
    <path d="M6 6l12 12M18 6L6 18" />
  </Icono>
);

export const IconoChispa = () => (
  <Icono tamano={16}>
    <path d="M12 3v4M12 17v4M3 12h4M17 12h4M6.3 6.3l2.5 2.5M15.2 15.2l2.5 2.5M6.3 17.7l2.5-2.5M15.2 8.8l2.5-2.5" />
  </Icono>
);

/** Estado vacio: escudo con visto, "todo en orden". */
export const IconoTodoEnOrden = () => (
  <Icono tamano={28}>
    <path d="M12 3l7 3v5c0 4.5-3 8.3-7 10-4-1.7-7-5.5-7-10V6l7-3z" />
    <path d="M8.5 12l2.5 2.5 4.5-4.5" />
  </Icono>
);
