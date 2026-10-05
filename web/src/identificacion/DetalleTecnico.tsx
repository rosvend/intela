import type { ReactElement } from "react";
import { idsDeFuente, type CasoIdentificacion } from "./tipos";

/** Lo que solo necesita quien audita: ids, entrega y la evidencia en bruto. */
export default function DetalleTecnico({
  caso,
}: {
  caso: CasoIdentificacion;
}): ReactElement {
  return (
    <dl className="detalle-tecnico">
      <dt>Uso</dt>
      <dd>{caso.id}</dd>
      <dt>Entrega</dt>
      <dd>{caso.reporte_id}</dd>
      {caso.titulo_original !== "" && (
        <>
          <dt>Título original</dt>
          <dd>{caso.titulo_original}</dd>
        </>
      )}
      {idsDeFuente(caso.ids_fuente).map((id) => (
        <dd key={id} className="detalle-tecnico-id">
          {id}
        </dd>
      ))}
      {caso.evidencia !== "" && (
        <>
          <dt>Por qué está aquí</dt>
          <dd>{caso.evidencia}</dd>
        </>
      )}
      {caso.sugerencia.motivo !== "" && (
        <>
          <dt>Motivo de la sugerencia</dt>
          <dd>{caso.sugerencia.motivo}</dd>
        </>
      )}
    </dl>
  );
}
