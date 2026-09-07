import { FormEvent, useState } from "react";
import { Rol } from "../sesion";
import {
  ETIQUETA_ROL_FIRMA,
  ROLES_DE_COMPUERTA,
  esCompuerta,
  puedeFirmar,
  rolesFirmados,
} from "./firmas";
import { ETIQUETA_ETAPA } from "./etapas";
import { Proceso } from "./tipos";

export default function Compuerta({
  proceso,
  rol,
  enviando = false,
  error,
  onFirmar,
  onRechazar,
}: {
  proceso: Proceso;
  rol: Rol;
  enviando?: boolean;
  error?: string;
  onFirmar: () => void;
  onRechazar: (motivo: string) => void;
}) {
  const [rechazando, setRechazando] = useState(false);
  const [motivo, setMotivo] = useState("");

  if (!esCompuerta(proceso.etapa)) return null;

  const firmados = new Set(rolesFirmados(proceso));
  const ofreceAccion = puedeFirmar(rol, proceso);

  function confirmarRechazo(evento: FormEvent) {
    evento.preventDefault();
    const texto = motivo.trim();
    if (!texto) return;
    onRechazar(texto);
  }

  return (
    <article className="compuerta">
      <h3 className="tarjeta-etiqueta">
        Compuerta · {ETIQUETA_ETAPA[proceso.etapa]}
      </h3>
      <p className="muted">
        Revisión {proceso.revision}. El dinero no sale con una sola firma (RD
        13.5).
      </p>
      <ul className="compuerta-firmas">
        {ROLES_DE_COMPUERTA.map((rolFirma) => (
          <li key={rolFirma}>
            <span>{ETIQUETA_ROL_FIRMA[rolFirma]}</span>
            <span
              className={
                firmados.has(rolFirma)
                  ? "compuerta-estado-firmado"
                  : "compuerta-estado-pendiente"
              }
            >
              {firmados.has(rolFirma) ? "Firmado" : "Pendiente"}
            </span>
          </li>
        ))}
      </ul>

      {ofreceAccion && !rechazando && (
        <div className="compuerta-acciones">
          <button
            type="button"
            className="boton-primario"
            disabled={enviando}
            onClick={onFirmar}
          >
            Firmar
          </button>
          <button
            type="button"
            className="boton-secundario"
            disabled={enviando}
            onClick={() => setRechazando(true)}
          >
            Rechazar
          </button>
        </div>
      )}

      {ofreceAccion && rechazando && (
        <form className="compuerta-rechazo" onSubmit={confirmarRechazo}>
          <label htmlFor="motivo-rechazo">Motivo del rechazo</label>
          <textarea
            id="motivo-rechazo"
            value={motivo}
            onChange={(evento) => setMotivo(evento.target.value)}
            required
            rows={3}
            disabled={enviando}
          />
          <div className="compuerta-acciones">
            <button
              type="submit"
              className="boton-peligro"
              disabled={enviando || motivo.trim() === ""}
            >
              Confirmar rechazo
            </button>
            <button
              type="button"
              className="boton-secundario"
              disabled={enviando}
              onClick={() => setRechazando(false)}
            >
              Cancelar
            </button>
          </div>
        </form>
      )}

      {error && (
        <p className="tarjeta-error" role="alert">
          {error}
        </p>
      )}
    </article>
  );
}
