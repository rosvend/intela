import { Rol } from "./sesion";

export type Seccion = "principal" | "configuracion";

export type ItemDeNav = {
  to: string;
  label: string;
  roles: readonly Rol[];
  seccion: Seccion;
  // Numero de pendientes para el badge del mockup (ej. "Anomalías 7"). Nadie
  // lo puebla todavia: es un dato del servidor y #37 (deteccion de anomalias)
  // aun no existe. Queda `undefined` -no se inventa un contador- hasta que
  // ese endpoint exista y algo lo asigne.
  contador?: number;
};

// Etiquetas, orden y agrupacion salen del mockup de Figma Make (issue #19,
// seccion "Pase visual"). Los roles son una propuesta del frontend a validar
// contra el `requiereRol` de #17 -si divergen, manda #17-.
//
// "/" (Inicio) es el primer item de PRINCIPAL, visible a los cinco roles: asi
// aparece en el mockup. Lo que cambia por rol no es si aparece en la nav, sino
// que renderiza <Inicio> -administrador ve el panel de control, titular ve su
// liquidacion (M-5)-.
export const RUTAS: readonly ItemDeNav[] = [
  {
    to: "/",
    label: "Inicio",
    roles: [
      "administrador",
      "distribucion",
      "contabilidad",
      "auditor",
      "titular",
    ],
    seccion: "principal",
  },
  {
    to: "/ingesta",
    label: "Ingesta",
    roles: ["administrador", "distribucion", "auditor"],
    seccion: "principal",
  },
  {
    to: "/catalogo",
    label: "Catálogo",
    roles: ["administrador", "distribucion", "auditor"],
    seccion: "principal",
  },
  {
    to: "/titulares",
    label: "Titulares",
    roles: ["administrador", "contabilidad", "auditor"],
    seccion: "principal",
  },
  {
    to: "/distribucion",
    label: "Distribución",
    roles: ["administrador", "distribucion", "auditor"],
    seccion: "principal",
  },
  {
    to: "/anomalias",
    label: "Anomalías",
    roles: ["administrador", "distribucion", "auditor"],
    seccion: "principal",
  },
  {
    to: "/reportes",
    label: "Reportes",
    roles: ["administrador", "contabilidad", "auditor"],
    seccion: "principal",
  },
  {
    to: "/deducciones",
    label: "Deducciones",
    roles: ["administrador", "auditor"],
    seccion: "configuracion",
  },
  {
    to: "/auditoria",
    label: "Auditoría",
    roles: ["administrador", "auditor"],
    seccion: "configuracion",
  },
];

export function itemsDeNav(rol: Rol): readonly ItemDeNav[] {
  return RUTAS.filter((item) => item.roles.includes(rol));
}

export function puedeVer(rol: Rol, ruta: string): boolean {
  return RUTAS.some((item) => item.to === ruta && item.roles.includes(rol));
}
