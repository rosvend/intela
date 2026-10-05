import { ArrowLeftIcon, ArrowRightIcon } from "@heroicons/react/20/solid";
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactElement,
} from "react";
import { Link } from "react-router-dom";
import "../revision.css";
import "./fusion.css";
import { useApi } from "../useApi";
import { posicionEnCola, vecinoEnCola } from "./cola";
import { IconoCheck, IconoTodoEnOrden } from "./iconos";
import { useFijarPendientes } from "./pendientes";
import { formatearPeriodo, nombreDeFuente } from "./presentacion";
import {
  LIMITE_BANDEJA,
  RUTAS_IDENTIFICACION,
  esPaginaDeCasos,
  type CasoIdentificacion,
} from "./tipos";
import VistaFusion from "./VistaFusion";

/** El aviso de exito, `role="status"` durante 4 segundos (plano seccion 5). */
type Aviso = { detalle: string };

const DURACION_AVISO_MS = 4000;

/**
 * Bandeja de identificacion (`/identificacion`): los casos pendientes, uno a
 * la vez, como una union entre lo reportado y la obra del catalogo.
 *
 * Es solo el envoltorio que fuerza la recarga completa (D2, D3): `useApi` no
 * vuelve a pedir el mismo path, asi que "Intentar de nuevo", "Cargar los
 * siguientes casos" y "Recargar caso" incrementan `recarga`, que via `key`
 * remonta `Contenido` entero y su estado local arranca limpio.
 */
export default function BandejaIdentificacion(): ReactElement {
  const [recarga, setRecarga] = useState(0);
  return (
    <Contenido key={recarga} onRecargar={() => setRecarga((r) => r + 1)} />
  );
}

/** Un evento de teclado que nace en un campo es de ese campo, no de la cola. */
function esDeUnCampo(destino: EventTarget | null): boolean {
  if (!(destino instanceof HTMLElement)) return false;
  return (
    destino.isContentEditable ||
    ["INPUT", "TEXTAREA", "SELECT"].includes(destino.tagName)
  );
}

function Contenido({ onRecargar }: { onRecargar: () => void }): ReactElement {
  const fijarPendientes = useFijarPendientes();
  const lectura = useApi<unknown>(
    RUTAS_IDENTIFICACION.casos({
      estado: "pendiente",
      limite: LIMITE_BANDEJA,
    }),
  );

  // Remocion optimista (D3): los ids que ya salieron de la cola, por
  // resolverse o mientras se guardan. Lo que cambia es lo que se PINTA de la
  // pagina, no la pagina.
  const [fuera, setFuera] = useState<Set<string>>(new Set());
  // El caso en escena. Puede estar fuera de la cola (guardandose, o tras un
  // 409 que lo saco) y seguir en escena con su error.
  const [foco, setFoco] = useState<string | null>(null);
  const [bloqueado, setBloqueado] = useState(false);
  const [trasUnion, setTrasUnion] = useState(false);
  const [aviso, setAviso] = useState<Aviso | null>(null);

  const pagina =
    !lectura.cargando && !lectura.error && esPaginaDeCasos(lectura.datos)
      ? lectura.datos
      : null;
  const ids = pagina ? pagina.casos.map((c) => c.id) : [];
  const visibles = new Set(ids.filter((id) => !fuera.has(id)));
  const casosVisibles = pagina
    ? pagina.casos.filter((c) => visibles.has(c.id))
    : [];
  const enEscena: CasoIdentificacion | null =
    pagina?.casos.find((c) => c.id === foco) ?? casosVisibles[0] ?? null;
  // `pagina.pendientes` es el conteo del servidor al cargar; restarle lo que
  // ya salio lo mantiene en vivo sin pedir de nuevo.
  const conteo = pagina
    ? Math.max(pagina.pendientes - fuera.size, 0)
    : undefined;
  const posicion = enEscena ? posicionEnCola(ids, visibles, enEscena.id) : null;
  const anterior = enEscena
    ? vecinoEnCola(ids, visibles, enEscena.id, -1)
    : null;
  const siguiente = enEscena
    ? vecinoEnCola(ids, visibles, enEscena.id, 1)
    : null;

  useEffect(() => {
    if (conteo !== undefined) fijarPendientes(conteo);
    // `fijarPendientes` es estable (memoizada en `useFijarPendientes`).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [conteo]);

  useEffect(() => {
    if (!aviso) return;
    const temporizador = setTimeout(() => setAviso(null), DURACION_AVISO_MS);
    return () => clearTimeout(temporizador);
  }, [aviso]);

  function irA(id: string | null) {
    if (id === null || bloqueado) return;
    setTrasUnion(false);
    setFoco(id);
  }

  // Las flechas pasan de caso. Sin animacion: es una accion de teclado que se
  // repite decenas de veces seguidas. Un solo listener; los vecinos vigentes
  // llegan por ref, fijada en el commit para que no haya tecla entre medias.
  const navegacion = useRef({ anterior, siguiente, irA });
  useLayoutEffect(() => {
    navegacion.current = { anterior, siguiente, irA };
  });
  useEffect(() => {
    function alPulsar(e: KeyboardEvent) {
      if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
      if (esDeUnCampo(e.target)) return;
      const { anterior, siguiente, irA } = navegacion.current;
      if (e.key === "ArrowRight") irA(siguiente);
      else if (e.key === "ArrowLeft") irA(anterior);
      else return;
      e.preventDefault();
    }
    window.addEventListener("keydown", alPulsar);
    return () => window.removeEventListener("keydown", alPulsar);
  }, []);

  function conId(previos: Set<string>, id: string, poner: boolean) {
    const siguientes = new Set(previos);
    if (poner) siguientes.add(id);
    else siguientes.delete(id);
    return siguientes;
  }

  return (
    <section
      className="bandeja revision-pantalla"
      aria-label="Bandeja de identificación"
    >
      <header className="revision-cabecera">
        <div className="revision-titulo-fila">
          <h1>Bandeja de identificación</h1>
          {conteo !== undefined && conteo > 0 && (
            <span className="chip chip-marca">{conteo} por revisar</span>
          )}
          <Link to="/lista-oni" className="boton-secundario boton-enlace">
            Ver lista ONI
            <ArrowRightIcon aria-hidden="true" />
          </Link>
        </div>
        <p className="revision-intro">
          Une cada uso reportado con su obra, o descártalo.
        </p>
      </header>

      {aviso && (
        <div className="revision-toast" role="status">
          <span className="revision-toast-icono">
            <IconoCheck tamano={16} />
          </span>
          <strong>Decisión registrada</strong>
          <span>{aviso.detalle}</span>
        </div>
      )}

      {lectura.cargando && (
        <EsqueletoDeCasos etiqueta="Cargando los casos pendientes" />
      )}

      {!lectura.cargando && lectura.error && (
        <div className="revision-aviso-error" role="alert">
          <p>No pudimos cargar los casos: {lectura.error.message}</p>
          <button
            type="button"
            className="boton-secundario"
            onClick={onRecargar}
          >
            Intentar de nuevo
          </button>
        </div>
      )}

      {!lectura.cargando && !lectura.error && pagina === null && (
        <p className="revision-aviso-error" role="alert">
          La bandeja no llegó como una lista de casos legibles.
        </p>
      )}

      {pagina !== null && !enEscena && (
        <EstadoVacio
          titulo="Todo en orden: no hay usos pendientes por identificar"
          texto={
            fuera.size > 0
              ? "Cada decisión quedó registrada con su nota."
              : "Cuando llegue un uso dudoso, aparecerá aquí."
          }
        />
      )}

      {pagina !== null && enEscena && (
        <div className="bandeja-fusion">
          <aside className="bandeja-cola">
            <div className="bandeja-progreso">
              {posicion !== null && conteo !== undefined && (
                <span className="bandeja-progreso-texto">
                  Caso {posicion} de {conteo}
                </span>
              )}
              <span className="bandeja-pasos">
                <button
                  type="button"
                  className="bandeja-paso"
                  aria-label="Caso anterior"
                  aria-keyshortcuts="ArrowLeft"
                  disabled={anterior === null || bloqueado}
                  onClick={() => irA(anterior)}
                >
                  <ArrowLeftIcon aria-hidden="true" />
                </button>
                <button
                  type="button"
                  className="bandeja-paso"
                  aria-label="Caso siguiente"
                  aria-keyshortcuts="ArrowRight"
                  disabled={siguiente === null || bloqueado}
                  onClick={() => irA(siguiente)}
                >
                  <ArrowRightIcon aria-hidden="true" />
                </button>
              </span>
            </div>
            {casosVisibles.length > 0 && (
              <nav aria-label="Cola de casos">
                <ol className="cola">
                  {casosVisibles.map((caso) => (
                    <li key={caso.id}>
                      <button
                        type="button"
                        className="cola-caso"
                        aria-current={caso.id === enEscena.id || undefined}
                        disabled={bloqueado}
                        onClick={() => irA(caso.id)}
                      >
                        <span className="cola-titulo">{caso.titulo}</span>
                        <span className="cola-meta">
                          {nombreDeFuente(caso.fuente)} ·{" "}
                          {formatearPeriodo(caso.periodo)}
                        </span>
                      </button>
                    </li>
                  ))}
                </ol>
              </nav>
            )}
            {conteo !== undefined && conteo > casosVisibles.length && (
              <div className="cola-mas">
                <p>
                  Se muestran {casosVisibles.length} de {conteo}, en orden de
                  llegada.
                </p>
                <button
                  type="button"
                  className="boton-secundario"
                  onClick={onRecargar}
                >
                  Cargar los siguientes casos
                </button>
              </div>
            )}
            <p className="bandeja-atajo" aria-hidden="true">
              <kbd>←</kbd> <kbd>→</kbd> para pasar de caso
            </p>
          </aside>

          <div
            className={`bandeja-escena${trasUnion ? " bandeja-escena-entra" : ""}`}
          >
            <VistaFusion
              key={enEscena.id}
              caso={enEscena}
              onEnviandoCambia={setBloqueado}
              onEnviarInicio={() => {
                setFoco(enEscena.id);
                setFuera((p) => conId(p, enEscena.id, true));
              }}
              onFalla={(volvioALaLista) => {
                if (volvioALaLista) {
                  setFuera((p) => conId(p, enEscena.id, false));
                }
              }}
              onExito={({ obraTitulo, esDescarte }) => {
                setAviso({
                  detalle: esDescarte
                    ? `Registro “${enEscena.titulo}” descartado`
                    : `Registro asignado a “${obraTitulo}”`,
                });
                setTrasUnion(true);
                // `enEscena` ya salio de `visibles`: sus vecinos son los que siguen.
                setFoco(siguiente ?? anterior);
              }}
              onRecargarTodo={onRecargar}
            />
          </div>
        </div>
      )}
    </section>
  );
}

/** Esqueleto de carga: la forma de las tarjetas, sin texto que leer. */
export function EsqueletoDeCasos({
  etiqueta,
}: {
  etiqueta: string;
}): ReactElement {
  return (
    <div
      className="esqueletos"
      role="status"
      aria-label={etiqueta}
      aria-busy="true"
    >
      {[0, 1, 2].map((i) => (
        <div key={i} className="esqueleto-caso" aria-hidden="true">
          <span className="esqueleto esqueleto-titulo" />
          <span className="esqueleto esqueleto-linea" />
          <span className="esqueleto esqueleto-barra" />
        </div>
      ))}
    </div>
  );
}

export function EstadoVacio({
  titulo,
  texto,
}: {
  titulo: string;
  texto?: string;
}): ReactElement {
  return (
    <div className="revision-vacio">
      <span className="revision-vacio-icono">
        <IconoTodoEnOrden />
      </span>
      <p className="revision-vacio-titulo">{titulo}</p>
      {texto && <p className="revision-vacio-texto">{texto}</p>}
    </div>
  );
}
