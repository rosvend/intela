import { useState } from "react";
import type { Explicacion, ValorizacionDeUso } from "../ingresos";
import { citarReglamento, citaDeRetencion } from "../reglamento";
import BarraApilada from "../ui/BarraApilada";
import Cifra from "../ui/Cifra";
import Detalle from "../ui/Detalle";
import { aNumero, formatearCOP } from "../ui/dinero";
import { USOS_VISIBLES, lineaDeValorizacion } from "../valorizacion";
import {
  fechaDeAprobacion,
  fraseDeUso,
  modalidadDeUso,
  partesDelBruto,
} from "./explicacion";
import {
  fechaLlana,
  nombreFuente,
  nombrePeriodo,
  numeroLlano,
  porcentajeLlano,
} from "./presentacion";
import "../titular.css";

type Props = {
  cifra: Explicacion;
  /** Personal de REDES: ve el linaje completo plegado (auditoria). */
  tecnico?: boolean;
};

/** ExplicarCifra contado como historia: recaudo -> tu parte -> descuentos -> lo que recibes. */
export default function HistoriaCifra({ cifra, tecnico = false }: Props) {
  const aprobado = fechaDeAprobacion(cifra.firmas);
  return (
    <section
      className="historia"
      role="region"
      aria-label="Cómo se calculó tu pago"
    >
      <ol className="historia-pasos">
        {cifra.bolsa && aNumero(cifra.bolsa.bruto) > 0 && (
          <Paso titulo="Lo que se recaudó">
            <p className="historia-cifra">{formatearCOP(cifra.bolsa.bruto)}</p>
            <p className="historia-texto">
              Lo que{" "}
              {cifra.reporte.fuente
                ? nombreFuente(cifra.reporte.fuente)
                : "el usuario"}{" "}
              pagó a REDES SGC por usar obras del repertorio en{" "}
              {nombrePeriodo(cifra.corrida.periodo)}. Cada obra recibe una parte
              según cuánto se usó.
            </p>
          </Paso>
        )}

        <Paso titulo="Tu parte">
          <div className="historia-parte">
            {cifra.split && <Anillo porcentaje={cifra.split.porcentaje} />}
            <div>
              <p className="historia-cifra">{formatearCOP(cifra.bruto)}</p>
              <p className="historia-texto">
                {cifra.split
                  ? `Tu obra generó una parte de esa bolsa y a ti te corresponde el ${porcentajeLlano(cifra.split.porcentaje)}, antes de descuentos.`
                  : "Lo que generó la obra antes de descuentos."}{" "}
                <Detalle
                  disparador="¿De dónde sale tu parte?"
                  titulo="Tu parte"
                >
                  <span className="detalle-texto">
                    Tu porcentaje sale únicamente de la Declaración de Obra que
                    firmaron los escritores de la obra. Ni los canales ni los
                    contratos lo cambian.
                  </span>
                </Detalle>
              </p>
            </div>
          </div>
        </Paso>

        {cifra.retenida ? (
          <Reserva bruto={cifra.bruto} />
        ) : (
          <>
            <Paso titulo="Descuentos de ley">
              <BarraApilada
                etiqueta="Descuentos de ley"
                segmentos={partesDelBruto(cifra).map((p) => ({
                  id: p.id,
                  etiqueta: p.etiqueta,
                  valor: p.valor,
                  color: p.color,
                  detalle: (
                    <>
                      {p.razon}
                      {p.cita && (
                        <span className="historia-cita">
                          <strong>{p.cita.titulo}.</strong> {p.cita.texto}
                        </span>
                      )}
                    </>
                  ),
                }))}
              />
              <ul className="leyenda historia-leyenda">
                {partesDelBruto(cifra).map((p) => (
                  <li key={p.id} className="leyenda-fila">
                    <span
                      className="leyenda-punto"
                      style={{ "--color": p.color } as React.CSSProperties}
                    />
                    <span>{p.etiqueta}</span>
                    <span className="leyenda-cifra">
                      {formatearCOP(p.valor)}
                    </span>
                  </li>
                ))}
              </ul>
            </Paso>
            <Paso titulo="Tú recibes" marca>
              <Cifra valor={cifra.neto} />
            </Paso>
          </>
        )}

        <Paso titulo="Cómo se usó tu obra">
          <Usos valorizacion={cifra.valorizacion} puntos={cifra.obra.puntos} />
        </Paso>
      </ol>

      {aprobado && (
        <p className="historia-aprobado">
          <span aria-hidden="true" className="historia-check">
            ✓
          </span>
          Revisado y aprobado por REDES SGC el {aprobado}
        </p>
      )}

      {tecnico && <DetalleTecnico cifra={cifra} />}
    </section>
  );
}

function Paso({
  titulo,
  marca = false,
  children,
}: {
  titulo: string;
  marca?: boolean;
  children: React.ReactNode;
}) {
  return (
    <li className={`historia-paso${marca ? " historia-paso-marca" : ""}`}>
      <h4 className="historia-titulo">{titulo}</h4>
      {children}
    </li>
  );
}

function Anillo({ porcentaje }: { porcentaje: string }) {
  const pct = Math.min(100, Math.max(0, aNumero(porcentaje)));
  return (
    <svg
      className="anillo"
      viewBox="0 0 48 48"
      role="img"
      aria-label={`Tu parte: ${porcentajeLlano(porcentaje)}`}
    >
      <circle
        className="anillo-fondo"
        cx="24"
        cy="24"
        r="19"
        pathLength={100}
      />
      <circle
        className="anillo-valor"
        cx="24"
        cy="24"
        r="19"
        pathLength={100}
        strokeDasharray={`${pct} 100`}
      />
      <text x="24" y="27.5" textAnchor="middle" className="anillo-texto">
        {Math.round(pct)}%
      </text>
    </svg>
  );
}

function Reserva({ bruto }: { bruto: string }) {
  const cita = citaDeRetencion();
  return (
    <li
      className="historia-paso historia-reserva"
      role="status"
      aria-label="Este pago está en reserva"
    >
      <h4 className="historia-titulo">Este pago está en reserva</h4>
      <p className="historia-cifra">{formatearCOP(bruto)}</p>
      <p className="historia-texto">
        La Declaración de Obra todavía no suma 100 % entre todos sus escritores.
        Mientras no esté completa, REDES SGC guarda el pago completo de esta
        obra y no reparte una parte.
      </p>
      <p className="historia-texto">
        <strong>Qué puedes hacer:</strong> escríbele a REDES SGC para completar
        la declaración con tus coautores. El pago se libera cuando sume 100 %.
      </p>
      <Detalle disparador="¿Por qué está en reserva?" titulo={cita.titulo}>
        <span className="detalle-texto">{cita.texto}</span>
      </Detalle>
    </li>
  );
}

function Usos({
  valorizacion,
  puntos,
}: {
  valorizacion: ValorizacionDeUso[];
  puntos: string;
}) {
  const [todos, setTodos] = useState(false);
  if (valorizacion.length === 0) {
    return (
      <p className="historia-texto">
        Tu obra sumó {numeroLlano(puntos)} puntos de uso en este periodo. El
        detalle por emisión no se guardó para esta liquidación.
      </p>
    );
  }
  const maximo = Math.max(...valorizacion.map((v) => aNumero(v.puntos)), 1);
  const visibles = todos ? valorizacion : valorizacion.slice(0, USOS_VISIBLES);
  return (
    <>
      <p className="historia-texto">
        Cada uso suma puntos; más puntos, más parte de la bolsa. En total,{" "}
        {numeroLlano(puntos)} puntos.
      </p>
      <ul className="usos" aria-label="Usos de tu obra">
        {visibles.map((v, i) => (
          <li key={v.uso_id} className="uso">
            <span className="uso-nombre">
              {modalidadDeUso(v)} · uso {i + 1}
            </span>
            <Detalle
              titulo={`${modalidadDeUso(v)} · uso ${i + 1}`}
              claseDisparador="uso-barra"
              etiquetaDisparador={`Uso ${i + 1}: ${numeroLlano(v.puntos)} puntos`}
              estiloDisparador={
                {
                  "--ancho": `${(aNumero(v.puntos) / maximo) * 100}%`,
                  "--retardo": `${i * 50}ms`,
                } as React.CSSProperties
              }
              disparador={<span className="uso-relleno" />}
            >
              <span className="detalle-texto">{fraseDeUso(v)}</span>
            </Detalle>
            <span className="uso-puntos">{numeroLlano(v.puntos)} pts</span>
          </li>
        ))}
      </ul>
      {valorizacion.length > USOS_VISIBLES && (
        <button
          type="button"
          className="enlace-pastilla"
          aria-expanded={todos}
          onClick={() => setTodos((t) => !t)}
        >
          {todos ? "Ver menos" : `Ver los ${valorizacion.length} usos`}
        </button>
      )}
    </>
  );
}

function DetalleTecnico({ cifra }: { cifra: Explicacion }) {
  const [abierto, setAbierto] = useState(false);
  return (
    <div className="historia-tecnico">
      <button
        type="button"
        className="enlace-pastilla"
        aria-expanded={abierto}
        onClick={() => setAbierto((a) => !a)}
      >
        {abierto ? "Ocultar detalle técnico" : "Ver detalle técnico"}
      </button>
      {abierto && (
        <div role="region" aria-label="Detalle técnico" className="desplegable">
          <dl className="tecnico-lista">
            <dt>Corrida</dt>
            <dd>
              {cifra.corrida.proceso_id} · {cifra.corrida.periodo} ·{" "}
              {cifra.corrida.circuito}
            </dd>
            <dt>Reporte de origen</dt>
            <dd>
              {cifra.reporte.fuente || "—"} · {cifra.reporte.id || "—"}
              {cifra.reporte.sha256 ? ` · sha256 ${cifra.reporte.sha256}` : ""}
            </dd>
            {cifra.bolsa?.id && (
              <>
                <dt>Bolsa</dt>
                <dd>
                  {cifra.bolsa.id} · {cifra.bolsa.bruto}
                </dd>
              </>
            )}
            <dt>Obra y match</dt>
            <dd>
              {cifra.obra.titulo} · escalón {cifra.obra.escalon || "—"} ·
              puntaje {cifra.obra.puntaje}
            </dd>
            <dt>Regla y snapshot</dt>
            <dd>
              {cifra.regla.reglamento} · snapshot {cifra.regla.snapshot_id}
            </dd>
            <dt>Split</dt>
            <dd>
              {cifra.split
                ? `${cifra.split.porcentaje}% · IPI ${cifra.split.ipi}${
                    typeof cifra.split.version === "number"
                      ? ` · declaración v${cifra.split.version}`
                      : ""
                  }`
                : "—"}
            </dd>
            <dt>Bruto → neto</dt>
            <dd>
              {cifra.bruto} →{" "}
              {cifra.deducciones
                .map((d) => `${d.concepto} ${d.porcentaje}% = ${d.monto}`)
                .join(" · ") || "sin deducciones"}{" "}
              → {cifra.neto}
            </dd>
            {cifra.retenida && (
              <>
                <dt>Retención</dt>
                <dd>{cifra.motivo || "retenida"} (RD 13.1.3)</dd>
              </>
            )}
            {cifra.firmas.length > 0 && (
              <>
                <dt>Firmas</dt>
                <dd>
                  <ul>
                    {cifra.firmas.map((f) => (
                      <li
                        key={`${f.rol}-${f.etapa}-${f.sobre_revision}-${f.cuando}`}
                      >
                        {f.rol} · {f.actor_id} · {f.etapa} · revisión{" "}
                        {f.sobre_revision} · {fechaLlana(f.cuando)}
                      </li>
                    ))}
                  </ul>
                </dd>
              </>
            )}
            {(cifra.faltantes ?? []).length > 0 && (
              <>
                <dt>Eslabones sin asiento</dt>
                <dd>{(cifra.faltantes ?? []).join(" · ")}</dd>
              </>
            )}
            {cifra.valorizacion.length > 0 && (
              <>
                <dt>Valorización</dt>
                <dd>
                  <ul>
                    {cifra.valorizacion.map((v) => (
                      <li key={v.uso_id}>
                        {v.uso_id} · {lineaDeValorizacion(v)}
                      </li>
                    ))}
                  </ul>
                </dd>
              </>
            )}
            <dt>Reglamento</dt>
            <dd>
              {citarReglamento(cifra.regla.reglamento).map((c) => (
                <p key={c.token} className="tecnico-cita">
                  <strong>{c.titulo}</strong>
                  {c.texto ? <> — {c.texto}</> : null}
                </p>
              ))}
            </dd>
          </dl>
        </div>
      )}
    </div>
  );
}
