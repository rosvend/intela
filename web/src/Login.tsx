import { FormEvent, useId, useState } from "react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { ApiError, ErrorDeRed, token } from "./api";
import logo from "./logo-intela.png";
import SnakeCanvas from "./login/SnakeCanvas";
import { useSesion } from "./sesion";

// `RutaProtegida` guarda el `location` entero, no solo la ruta: un filtro de
// tabla vive en la query y un ancla en el fragmento. Quedarse con `pathname`
// devolveria /catalogo cuando se pidio /catalogo?pagina=2#resultados.
type EstadoDeUbicacion = {
  from?: { pathname: string; search?: string; hash?: string };
};

type Modo = "entrar" | "crear";

/**
 * Pantalla de acceso a dos columnas: el formulario a la izquierda, la marca y
 * el juego a la derecha.
 *
 * # El alta de cuenta se muestra, y no finge
 *
 * El modo "Crear cuenta" dibuja el formulario completo -nombre, correo,
 * contrasena- porque es parte del diseno pedido, pero **no hay endpoint
 * detras**: `POST /afiliaciones` llega con el issue #50, que sigue abierto. El
 * boton queda deshabilitado con el motivo a la vista en vez de mandar la
 * peticion a ningun sitio.
 *
 * Es la misma linea que ya seguia "¿Olvidaste tu contrasena?" en la version
 * anterior de esta pantalla: se muestra lo que existira y se dice que todavia
 * no, en vez de simular un flujo que no esta construido. Un formulario de alta
 * que acepta datos y no crea nada es peor que uno que avisa.
 *
 * # Lo que sigue sin implementarse del mockup original (issue #19)
 *
 * El toggle Admin/Titular: el rol lo emite el servidor dentro de la respuesta
 * de login, no lo elige quien rellena el formulario (M-2).
 */
export default function Login() {
  const { usuario, entrar, salidaSinRevocar } = useSesion();
  const navigate = useNavigate();
  const location = useLocation();

  const [modo, setModo] = useState<Modo>("entrar");
  const [nombre, setNombre] = useState("");
  const [email, setEmail] = useState("");
  const [clave, setClave] = useState("");
  const [claveVisible, setClaveVisible] = useState(false);
  const [enviando, setEnviando] = useState(false);
  const [error, setError] = useState("");

  // useId y no cadenas fijas: si esta pantalla acaba embebida dos veces -un
  // modal sobre la vista, por ejemplo- los `for` seguirian apuntando a su
  // propio campo y no al del otro formulario.
  const idNombre = useId();
  const idEmail = useId();
  const idClave = useId();
  const idError = useId();

  const creando = modo === "crear";

  // Llegar a /login con una sesion ya vigente (atras del navegador, una
  // pestana vieja) no tiene por que mostrar el formulario otra vez.
  if (token() && usuario) {
    return <Navigate to="/" replace />;
  }

  function cambiarModo(siguiente: Modo) {
    setModo(siguiente);
    // El error del modo anterior no describe este: "credenciales invalidas"
    // sobra en cuanto se pasa al alta.
    setError("");
  }

  async function manejarEnvio(evento: FormEvent) {
    evento.preventDefault();
    setError("");
    setEnviando(true);
    try {
      await entrar(email, clave);
      const origen = (location.state as EstadoDeUbicacion | null)?.from;
      navigate(
        origen
          ? {
              pathname: origen.pathname,
              search: origen.search ?? "",
              hash: origen.hash ?? "",
            }
          : "/",
        { replace: true },
      );
    } catch (err) {
      if (err instanceof ApiError || err instanceof ErrorDeRed) {
        setError(err.message);
      } else {
        setError("no se pudo iniciar sesión");
      }
    } finally {
      setEnviando(false);
    }
  }

  return (
    <div className="acceso">
      <main className="acceso-formulario">
        <div className="acceso-columna">
          {/*
            El logo se pinta como mascara y no como <img> porque el activo de
            origen es gris con alfa: la forma esta en el canal alfa y el color
            lo mandan los tokens.
            Lleva `role="img"` con nombre y no `aria-hidden` porque es lo unico
            que identifica de que sistema es esta pantalla: quien no ve la
            imagen se quedaria solo con "Iniciar sesion", que no dice de que.
          */}
          <span
            className="acceso-logo"
            style={{
              maskImage: `url(${logo})`,
              WebkitMaskImage: `url(${logo})`,
            }}
            role="img"
            aria-label="Intela"
          />

          <h1 className="acceso-titulo">
            {creando ? "Crear una cuenta" : "Iniciar sesión"}
          </h1>
          <p className="acceso-entradilla">
            {creando
              ? "El alta de titulares de REDES SGC."
              : "Reconocimiento de obras y reparto de derechos para los escritores audiovisuales de REDES SGC."}
          </p>

          {/*
            Se llego aqui tras un "Salir" en el que el servidor no confirmo la
            revocacion: en este equipo la sesion se cerro, pero alla sigue viva.
            En un equipo compartido eso hay que decirlo, no tragarselo.
          */}
          {salidaSinRevocar && (
            <p role="alert" className="acceso-aviso">
              Se cerró la sesión en este equipo, pero el servidor no confirmó la
              revocación. Si estás en un equipo compartido, avísale a un
              administrador.
            </p>
          )}

          {creando && (
            <p className="acceso-nota" role="note">
              El alta en línea todavía no está abierta. Para afiliarte, escribe
              a REDES SGC; cuando el formulario esté conectado podrás enviarlo
              desde aquí.
            </p>
          )}

          <form onSubmit={(e) => void manejarEnvio(e)} noValidate={false}>
            {creando && (
              <div className="acceso-campo">
                <label htmlFor={idNombre}>Nombre</label>
                <input
                  id={idNombre}
                  name="nombre"
                  type="text"
                  autoComplete="name"
                  placeholder="Nombre y apellidos"
                  value={nombre}
                  onChange={(e) => setNombre(e.target.value)}
                />
              </div>
            )}

            <div className="acceso-campo">
              <label htmlFor={idEmail}>Correo electrónico</label>
              <input
                id={idEmail}
                name="email"
                type="email"
                required
                autoComplete="username"
                placeholder="nombre@redes.co"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                aria-describedby={error ? idError : undefined}
                aria-invalid={error ? true : undefined}
              />
            </div>

            <div className="acceso-campo">
              <div className="acceso-fila-clave">
                <label htmlFor={idClave}>Contraseña</label>
                {!creando && (
                  <button
                    type="button"
                    className="enlace-tenue"
                    disabled
                    title="Aún no disponible"
                  >
                    ¿Olvidaste tu contraseña?
                  </button>
                )}
              </div>
              <div className="acceso-clave">
                <input
                  id={idClave}
                  name="clave"
                  type={claveVisible ? "text" : "password"}
                  required
                  autoComplete={creando ? "new-password" : "current-password"}
                  placeholder="Tu contraseña"
                  value={clave}
                  onChange={(e) => setClave(e.target.value)}
                  aria-describedby={error ? idError : undefined}
                  aria-invalid={error ? true : undefined}
                />
                {/*
                  `aria-pressed` y no solo un aria-label que cambia: el estado
                  del boton es lo que hay que anunciar, y el nombre se queda
                  quieto para que no parezca otro control cada vez.
                */}
                <button
                  type="button"
                  className="acceso-ojo"
                  onClick={() => setClaveVisible((v) => !v)}
                  aria-pressed={claveVisible}
                  aria-controls={idClave}
                  aria-label="Mostrar la contraseña"
                >
                  <Ojo tachado={!claveVisible} />
                </button>
              </div>
            </div>

            {error && (
              <p role="alert" id={idError} className="acceso-error">
                {error}
              </p>
            )}

            {creando ? (
              <button
                type="submit"
                className="boton-primario"
                disabled
                title="El alta en línea llega con el issue #50"
              >
                Crear cuenta
              </button>
            ) : (
              <button
                type="submit"
                className="boton-primario"
                disabled={enviando}
              >
                {enviando ? "Ingresando…" : "Ingresar"}
              </button>
            )}
          </form>

          <p className="acceso-cambio">
            {creando ? "¿Ya tienes una cuenta? " : "¿No tienes una cuenta? "}
            <button
              type="button"
              className="enlace"
              onClick={() => cambiarModo(creando ? "entrar" : "crear")}
            >
              {creando ? "Iniciar sesión" : "Crear una cuenta"}
            </button>
          </p>
        </div>
      </main>

      {/*
        `aside`: es acompanamiento de marca, no contenido de la pagina. Va
        despues del formulario en el DOM para que el tabulador y los lectores
        de pantalla lleguen primero a lo que se viene a hacer aqui, aunque en
        pantalla ancha se vea a la derecha.
      */}
      <aside className="acceso-marca">
        <SnakeCanvas />
      </aside>
    </div>
  );
}

/** El ojo del boton de visibilidad. Decorativo: el nombre lo da el boton. */
function Ojo({ tachado }: { tachado: boolean }) {
  return (
    <svg
      viewBox="0 0 24 24"
      width="20"
      height="20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12Z" />
      <circle cx="12" cy="12" r="3.1" />
      {tachado && <path d="M4 20 20 4" />}
    </svg>
  );
}
