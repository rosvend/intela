import { FormEvent, useState } from "react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { ApiError, ErrorDeRed, token } from "./api";
import { useSesion } from "./sesion";

// `RutaProtegida` guarda el `location` entero, no solo la ruta: un filtro de
// tabla vive en la query y un ancla en el fragmento. Quedarse con `pathname`
// devolveria /catalogo cuando se pidio /catalogo?pagina=2#resultados.
type EstadoDeUbicacion = {
  from?: { pathname: string; search?: string; hash?: string };
};

/**
 * Copia literal del mockup (issue #19, seccion "Pase visual", version 6).
 *
 * Lo que el mockup trae y aqui NO se implementa:
 * - El toggle Admin/Titular: el rol lo emite el servidor dentro de la
 *   respuesta de login, no lo elige quien usa el formulario (M-2). Para
 *   probar otro rol se inicia sesion con otra de las cuentas sembradas.
 * - "¿Olvidaste tu contraseña?" no tiene endpoint ni pantalla detras (M-8):
 *   se muestra pero no hace nada, en vez de simular un flujo que no existe.
 */
export default function Login() {
  const { usuario, entrar, salidaSinRevocar } = useSesion();
  const navigate = useNavigate();
  const location = useLocation();
  const [email, setEmail] = useState("");
  const [clave, setClave] = useState("");
  const [enviando, setEnviando] = useState(false);
  const [error, setError] = useState("");

  // Llegar a /login con una sesion ya vigente (atras del navegador, una
  // pestana vieja) no tiene por que mostrar el formulario otra vez.
  if (token() && usuario) {
    return <Navigate to="/" replace />;
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
    <div className="login">
      <div className="login-tarjeta">
        <h1>Iniciar sesión</h1>
        <p className="muted">Accede al gestor de propiedad intelectual</p>

        {/*
          Se llego aqui tras un "Salir" en el que el servidor no confirmo la
          revocacion: en este equipo la sesion se cerro, pero alla sigue viva.
          En un equipo compartido eso hay que decirlo, no tragarselo.
        */}
        {salidaSinRevocar && (
          <p role="alert" className="login-aviso">
            Se cerró la sesión en este equipo, pero el servidor no confirmó la
            revocación. Si estás en un equipo compartido, avísale a un
            administrador.
          </p>
        )}

        <form onSubmit={(e) => void manejarEnvio(e)}>
          <label htmlFor="email">Correo electrónico</label>
          <input
            id="email"
            type="email"
            required
            autoComplete="username"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />

          <div className="login-fila-clave">
            <label htmlFor="clave">Contraseña</label>
            <button
              type="button"
              className="enlace-tenue"
              disabled
              title="Aún no disponible"
            >
              ¿Olvidaste tu contraseña?
            </button>
          </div>
          <input
            id="clave"
            type="password"
            required
            autoComplete="current-password"
            value={clave}
            onChange={(e) => setClave(e.target.value)}
          />

          {error && (
            <p role="alert" className="login-error">
              {error}
            </p>
          )}

          <button type="submit" className="boton-primario" disabled={enviando}>
            {enviando ? "Ingresando…" : "Ingresar"}
          </button>
        </form>

        <p className="muted login-demo">
          Credenciales de demo — Admin: admin@redes.co · Titular: ana@redes.co
        </p>
      </div>
    </div>
  );
}
