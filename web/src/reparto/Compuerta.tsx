import { FormEvent, useId, useState } from "react";
import { CheckIcon, ShieldCheckIcon } from "@heroicons/react/20/solid";
import { iniciales } from "../iniciales";
import { Rol } from "../sesion";
import {
  ETIQUETA_ROL_FIRMA,
  ROLES_DE_COMPUERTA,
  esCompuerta,
  firmasDeRevision,
  puedeFirmar,
} from "./firmas";
import { ETIQUETA_ETAPA } from "./etapas";
import { Proceso } from "./tipos";
import "../staff.css";

export default function Compuerta({
  proceso,
  rol,
  advertencia,
  enviando = false,
  error,
  onFirmar,
  onRechazar,
}: {
  proceso: Proceso;
  rol: Rol;
  /**
   * Lo que el firmante deberia saber antes de firmar. Avisa, no bloquea, y
   * distingue las criticas (el backend responde 409 en `avanzar`) de las
   * abiertas que no bloquean y de las aceptadas sin corregir el dato. Una
   * reserva por declaracion incompleta es un estado normal del periodo
   * (`R-04`, `RD 13.1.3`). Quien hace cumplir la regla es el backend.
   */
  advertencia?: string;
  enviando?: boolean;
  error?: string;
  onFirmar: () => void;
  onRechazar: (motivo: string) => void;
}) {
  const [rechazando, setRechazando] = useState(false);
  const [motivo, setMotivo] = useState("");
  const idMotivo = useId();

  if (!esCompuerta(proceso.etapa)) return null;

  const firmas = firmasDeRevision(proceso);
  const firmados = new Set(firmas.map((f) => f.rol));
  const total = ROLES_DE_COMPUERTA.length;
  const ofreceAccion = puedeFirmar(rol, proceso);
  const etapa = ETIQUETA_ETAPA[proceso.etapa];

  function confirmarRechazo(evento: FormEvent) {
    evento.preventDefault();
    const texto = motivo.trim();
    if (!texto) return;
    onRechazar(texto);
  }

  return (
    <article className="compuerta-tarjeta">
      <header className="compuerta-cabecera">
        <span className="compuerta-sello" aria-hidden="true">
          <ShieldCheckIcon />
        </span>
        <div>
          <h3 className="compuerta-titulo">
            Esta etapa necesita {total} firmas
          </h3>
          <p className="muted">
            {etapa}: el dinero no avanza sin la firma de Distribución y de
            Contabilidad.
          </p>
        </div>
        <span
          className={`chip ${firmados.size === total ? "chip-ok" : "chip-marca"}`}
        >
          {firmados.size} de {total}
        </span>
      </header>

      <ul className="compuerta-firmas">
        {ROLES_DE_COMPUERTA.map((rolFirma) => {
          const firmado = firmados.has(rolFirma);
          const nombre = ETIQUETA_ROL_FIRMA[rolFirma];
          return (
            <li
              key={rolFirma}
              className={firmado ? "compuerta-firmante-hecho" : undefined}
            >
              <span className="compuerta-avatar" aria-hidden="true">
                {firmado ? <CheckIcon /> : iniciales(nombre)}
              </span>
              <span className="compuerta-firmante">
                <span>{nombre}</span>
                <span className={`chip ${firmado ? "chip-ok" : ""}`}>
                  {firmado ? "Firmado" : "Pendiente"}
                </span>
              </span>
            </li>
          );
        })}
      </ul>

      {ofreceAccion && advertencia && (
        <p className="compuerta-advertencia" role="status">
          {advertencia}
        </p>
      )}

      {ofreceAccion && !rechazando && (
        <div className="compuerta-acciones">
          <button
            type="button"
            className="boton-primario"
            disabled={enviando}
            onClick={onFirmar}
          >
            {enviando ? "Firmando…" : "Firmar"}
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
          <label htmlFor={idMotivo}>Motivo del rechazo</label>
          <p className="muted compuerta-ayuda">
            La distribución vuelve a la etapa anterior y quien la preparó verá
            este motivo.
          </p>
          <textarea
            id={idMotivo}
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

      <details className="detalle-tecnico">
        <summary>Detalle técnico de las firmas</summary>
        <dl>
          <dt>Proceso</dt>
          <dd>{proceso.id}</dd>
          <dt>Revisión</dt>
          <dd>Revisión {proceso.revision}</dd>
          {firmas.map((f) => (
            <div key={f.rol}>
              <dt>Firma de {ETIQUETA_ROL_FIRMA[f.rol]}</dt>
              <dd>{f.actor_id}</dd>
            </div>
          ))}
        </dl>
      </details>
    </article>
  );
}
