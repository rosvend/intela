import { useState, type FormEvent } from "react";
import { api } from "../api";
import {
  type DatosAlta,
  datosVacios,
  errorDelPaso,
  conflictoExclusividad,
} from "./reglas";

const PASOS = [
  "Identidad",
  "Vínculo",
  "Documentos",
  "Exclusividad",
  "Revisión",
] as const;

type AfiliacionCreada = {
  id: string;
  estado: string;
  subtipo: string;
  elegible_anticipo: boolean;
};

export default function WizardAfiliacion() {
  const [paso, setPaso] = useState(0);
  const [datos, setDatos] = useState<DatosAlta>(datosVacios);
  const [error, setError] = useState("");
  const [enviando, setEnviando] = useState(false);
  const [creada, setCreada] = useState<AfiliacionCreada | null>(null);

  function patch(p: Partial<DatosAlta>) {
    setDatos((d) => ({ ...d, ...p }));
    setError("");
  }

  function adelante() {
    const fallo = errorDelPaso(paso, datos);
    if (fallo) {
      setError(fallo);
      return;
    }
    setError("");
    setPaso((p) => Math.min(p + 1, PASOS.length - 1));
  }

  function atras() {
    setError("");
    setPaso((p) => Math.max(p - 1, 0));
  }

  async function enviar(e: FormEvent) {
    e.preventDefault();
    const conflicto = conflictoExclusividad(
      datos.perteneceOtraSgc,
      Boolean(datos.renuncia),
    );
    if (conflicto) {
      setError(conflicto);
      return;
    }
    if (!datos.rut || !datos.certificacionBancaria || !datos.subtipo) {
      setError("Faltan datos para enviar la solicitud.");
      return;
    }

    const cuerpo = new FormData();
    cuerpo.set("nombre", datos.nombre.trim());
    cuerpo.set("email", datos.email.trim());
    cuerpo.set("documento_identidad", datos.documentoIdentidad.trim());
    cuerpo.set("subtipo", datos.subtipo);
    if (datos.ipi.trim()) cuerpo.set("ipi", datos.ipi.trim());
    cuerpo.set("pertenece_otra_sgc", datos.perteneceOtraSgc ? "true" : "false");
    cuerpo.set("rut", datos.rut);
    cuerpo.set("certificacion_bancaria", datos.certificacionBancaria);
    if (datos.renuncia) cuerpo.set("renuncia", datos.renuncia);

    setEnviando(true);
    setError("");
    try {
      const r = (await api("/api/afiliaciones", {
        method: "POST",
        body: cuerpo,
      })) as AfiliacionCreada;
      setCreada(r);
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo enviar.");
    } finally {
      setEnviando(false);
    }
  }

  if (creada) {
    return (
      <section className="wizard">
        <h1>Solicitud enviada</h1>
        <p role="status">
          Quedó en estado <strong>pendiente de admisión</strong>. El Consejo
          Directivo la estudia según el reglamento de socios (RS 5.2 / RS 5.3).
        </p>
        <p className="muted">Referencia: {creada.id}</p>
        <p className="muted">
          Vínculo:{" "}
          {creada.subtipo === "socio" ? "Socio" : "Titular administrado"}. El
          anticipo solo procede si, una vez admitido, el vínculo es societario
          (R-30).
        </p>
      </section>
    );
  }

  return (
    <section className="wizard">
      <h1>Alta de titular</h1>
      <p className="muted">
        Cinco pasos para dejar la solicitud lista: identidad, tipo de vínculo,
        documentos de cobro (R-12), exclusividad de sociedad (R-28) y revisión.
      </p>
      <ol className="pasos" aria-label="Pasos del alta">
        {PASOS.map((nombre, i) => (
          <li key={nombre} aria-current={i === paso ? "step" : undefined}>
            {nombre}
          </li>
        ))}
      </ol>

      <form onSubmit={enviar}>
        {paso === 0 && (
          <fieldset>
            <legend>Identidad e IPI</legend>
            <label>
              Nombre
              <input
                name="nombre"
                value={datos.nombre}
                onChange={(e) => patch({ nombre: e.target.value })}
                autoComplete="name"
                required
              />
            </label>
            <label>
              Correo
              <input
                name="email"
                type="email"
                value={datos.email}
                onChange={(e) => patch({ email: e.target.value })}
                autoComplete="email"
                required
              />
            </label>
            <label>
              Documento de identidad
              <input
                name="documento_identidad"
                value={datos.documentoIdentidad}
                onChange={(e) => patch({ documentoIdentidad: e.target.value })}
                required
              />
            </label>
            <label>
              IPI (opcional)
              <input
                name="ipi"
                value={datos.ipi}
                onChange={(e) => patch({ ipi: e.target.value })}
                placeholder={"Se puede completar después, antes de la admisión"}
              />
            </label>
            <p className="muted">
              El IPI identifica a la persona en el sistema CISAC. No es
              obligatorio en este paso; una persona natural no entra al padrón
              de cobro sin él.
            </p>
          </fieldset>
        )}

        {paso === 1 && (
          <fieldset>
            <legend>Tipo de vínculo</legend>
            <label className="opcion">
              <input
                type="radio"
                name="subtipo"
                value="socio"
                checked={datos.subtipo === "socio"}
                onChange={() => patch({ subtipo: "socio" })}
              />
              Socio — vínculo societario (RS 4.1). Titular originario. Puede
              pedir anticipo una vez admitido (R-30).
            </label>
            <label className="opcion">
              <input
                type="radio"
                name="subtipo"
                value="administrado"
                checked={datos.subtipo === "administrado"}
                onChange={() => patch({ subtipo: "administrado" })}
              />
              Titular administrado — vínculo contractual (RS 4.2). Herederos y
              quienes optan por no ser socios. No pide anticipo.
            </label>
          </fieldset>
        )}

        {paso === 2 && (
          <fieldset>
            <legend>Documentos para poder cobrar (R-12)</legend>
            <p className="muted">
              RUT actualizado y certificación bancaria. Sin ellos no hay orden
              de pago, y el alta no avanza.
            </p>
            <label>
              RUT actualizado
              <input
                name="rut"
                type="file"
                accept="application/pdf,image/jpeg,image/png"
                onChange={(e) => patch({ rut: e.target.files?.[0] ?? null })}
              />
            </label>
            <label>
              Certificación bancaria
              <input
                name="certificacion_bancaria"
                type="file"
                accept="application/pdf,image/jpeg,image/png"
                onChange={(e) =>
                  patch({
                    certificacionBancaria: e.target.files?.[0] ?? null,
                  })
                }
              />
            </label>
          </fieldset>
        )}

        {paso === 3 && (
          <fieldset>
            <legend>Exclusividad de sociedad (R-28)</legend>
            <p className="muted">
              No se acepta como afiliado a quien pertenezca a otra sociedad de
              gestión colectiva del mismo género, en el país o en el exterior,
              sin renuncia previa y expresa.
            </p>
            <label className="opcion">
              <input
                type="radio"
                name="pertenece_otra_sgc"
                value="false"
                checked={datos.perteneceOtraSgc === false}
                onChange={() =>
                  patch({ perteneceOtraSgc: false, renuncia: null })
                }
              />
              Declaro que no pertenezco a otra SGC del mismo género.
            </label>
            <label className="opcion">
              <input
                type="radio"
                name="pertenece_otra_sgc"
                value="true"
                checked={datos.perteneceOtraSgc === true}
                onChange={() => patch({ perteneceOtraSgc: true })}
              />
              Sí pertenezco; adjunto la renuncia expresa.
            </label>
            {datos.perteneceOtraSgc === true && (
              <label>
                Documento de renuncia
                <input
                  name="renuncia"
                  type="file"
                  accept="application/pdf,image/jpeg,image/png"
                  onChange={(e) =>
                    patch({ renuncia: e.target.files?.[0] ?? null })
                  }
                />
              </label>
            )}
          </fieldset>
        )}

        {paso === 4 && (
          <fieldset>
            <legend>Revisión</legend>
            <dl className="revision">
              <dt>Nombre</dt>
              <dd>{datos.nombre}</dd>
              <dt>Correo</dt>
              <dd>{datos.email}</dd>
              <dt>Documento</dt>
              <dd>{datos.documentoIdentidad}</dd>
              <dt>IPI</dt>
              <dd>{datos.ipi.trim() || "No informado (opcional)"}</dd>
              <dt>Vínculo</dt>
              <dd>
                {datos.subtipo === "socio"
                  ? "Socio (elegible a anticipo tras la admisión)"
                  : "Titular administrado (sin anticipo)"}
              </dd>
              <dt>RUT</dt>
              <dd>{datos.rut?.name ?? "Falta"}</dd>
              <dt>Certificación bancaria</dt>
              <dd>{datos.certificacionBancaria?.name ?? "Falta"}</dd>
              <dt>Exclusividad</dt>
              <dd>
                {datos.perteneceOtraSgc
                  ? `Pertenece a otra SGC; renuncia: ${
                      datos.renuncia?.name ?? "falta"
                    }`
                  : "No pertenece a otra SGC del mismo género"}
              </dd>
            </dl>
          </fieldset>
        )}

        {error ? (
          <p role="alert" className="alerta">
            {error}
          </p>
        ) : null}

        <div className="acciones">
          {paso > 0 ? (
            <button type="button" onClick={atras}>
              Atrás
            </button>
          ) : null}
          {paso < PASOS.length - 1 ? (
            <button type="button" onClick={adelante}>
              Siguiente
            </button>
          ) : (
            <button type="submit" disabled={enviando}>
              {enviando ? "Enviando…" : "Enviar solicitud"}
            </button>
          )}
        </div>
      </form>
    </section>
  );
}
