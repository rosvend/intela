export const MENSAJE_EXCLUSIVIDAD =
  "R-28: no se acepta como afiliado a quien pertenezca a otra sociedad de gestión colectiva del mismo género sin renuncia previa y expresa.";

export const MENSAJE_DOCUMENTOS =
  "R-12: el RUT actualizado y la certificación bancaria son obligatorios.";

export type SubtipoAfiliacion = "socio" | "administrado";

export type DatosAlta = {
  nombre: string;
  email: string;
  documentoIdentidad: string;
  ipi: string;
  subtipo: SubtipoAfiliacion | "";
  perteneceOtraSgc: boolean | null;
  rut: File | null;
  certificacionBancaria: File | null;
  renuncia: File | null;
};

export const datosVacios: DatosAlta = {
  nombre: "",
  email: "",
  documentoIdentidad: "",
  ipi: "",
  subtipo: "",
  perteneceOtraSgc: null,
  rut: null,
  certificacionBancaria: null,
  renuncia: null,
};

export function conflictoExclusividad(
  perteneceOtraSgc: boolean | null,
  tieneRenuncia: boolean,
): string | null {
  if (perteneceOtraSgc === true && !tieneRenuncia) {
    return MENSAJE_EXCLUSIVIDAD;
  }
  return null;
}

export function errorDelPaso(paso: number, d: DatosAlta): string | null {
  switch (paso) {
    case 0:
      if (!d.nombre.trim()) return "El nombre es obligatorio.";
      if (!d.email.includes("@") || !d.email.includes(".")) {
        return "El correo no es válido.";
      }
      if (!d.documentoIdentidad.trim()) {
        return "El documento de identidad es obligatorio.";
      }
      return null;
    case 1:
      if (d.subtipo !== "socio" && d.subtipo !== "administrado") {
        return "Hay que elegir el tipo de vínculo.";
      }
      return null;
    case 2:
      if (!d.rut || !d.certificacionBancaria) return MENSAJE_DOCUMENTOS;
      return null;
    case 3:
      if (d.perteneceOtraSgc === null) {
        return "Hay que declarar si se pertenece a otra sociedad del mismo género.";
      }
      return conflictoExclusividad(d.perteneceOtraSgc, Boolean(d.renuncia));
    default:
      return null;
  }
}
