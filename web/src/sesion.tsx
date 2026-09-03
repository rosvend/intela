import {
  ReactNode,
  createContext,
  useContext,
  useEffect,
  useState,
} from "react";
import { useNavigate } from "react-router-dom";
import {
  ApiError,
  api,
  clearToken,
  setToken,
  setUnauthorizedHandler,
  token,
  tokenKey,
} from "./api";

export type Rol =
  "administrador" | "distribucion" | "contabilidad" | "auditor" | "titular";

export type Usuario = {
  id: string;
  email: string;
  nombre: string;
  rol: Rol;
  titular_id: string;
};

export type Sesion = {
  token: string;
  expira: string;
  usuario: Usuario;
};

// Los `as` de aqui para abajo son las fronteras sin validar que menciona el
// comentario de `api()`: quedan a la vista, en un solo lugar por endpoint, en
// vez de disueltos en cada sitio que llama a estas funciones.
export function iniciarSesion(email: string, clave: string): Promise<Sesion> {
  return api("/api/auth/session", {
    method: "POST",
    body: JSON.stringify({ email, clave }),
    // Un 401 aqui es "clave mala", no "sesion vencida": no debe disparar la
    // redireccion de sesion expirada, sino dejar que Login muestre el error.
    anonima: true,
  }) as Promise<Sesion>;
}

export const sesionActual = (): Promise<Usuario> =>
  api("/api/auth/session") as Promise<Usuario>;

export const cerrarSesion = (): Promise<Response> =>
  api("/api/auth/session", { method: "DELETE" }) as Promise<Response>;

/**
 * Resultado de cerrar sesion.
 *
 * `salir()` limpia el estado local pase lo que pase -pulsar "Salir" tiene que
 * sacarte de esta maquina aunque no haya red-, pero si el servidor no alcanzo
 * a revocar la sesion sigue viva alla. En un equipo compartido eso importa, y
 * quien llama necesita poder decirlo.
 */
export type ResultadoDeSalida = { revocadaEnServidor: boolean };

type ContextoSesion = {
  usuario: Usuario | null;
  cargando: boolean;
  /**
   * La ultima salida limpio el estado local pero el servidor no confirmo la
   * revocacion. Vive aqui y no en `Layout` porque al salir `usuario` pasa a
   * null y `Layout` se desmonta: el aviso tiene que sobrevivir a eso para que
   * alguien llegue a leerlo, y donde se lee es en la pantalla de login.
   */
  salidaSinRevocar: boolean;
  entrar: (email: string, clave: string) => Promise<void>;
  salir: () => Promise<ResultadoDeSalida>;
};

const ContextoSesion = createContext<ContextoSesion | null>(null);

export function ProveedorDeSesion({ children }: { children: ReactNode }) {
  const [usuario, setUsuario] = useState<Usuario | null>(null);
  const [cargando, setCargando] = useState(true);
  const [salidaSinRevocar, setSalidaSinRevocar] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    let vigente = true;

    if (!token()) {
      setCargando(false);
    } else {
      sesionActual()
        .then((u) => vigente && setUsuario(u))
        .catch(() => vigente && setUsuario(null))
        .finally(() => vigente && setCargando(false));
    }

    // Solo limpia el usuario: NO navega. `alExpirarSesion` corre dentro de
    // `api()`, en medio de una peticion de una pantalla ya montada bajo
    // RutaProtegida. Si este handler tambien llamara a `navigate("/login")`,
    // ganaria la carrera contra el <Navigate state={{from: location}}> de
    // RutaProtegida -que se dispara solo hasta el siguiente render- y la
    // ubicacion real (p. ej. /estado) se perderia: siempre se aterrizaria en
    // "/". Dejar que sea RutaProtegida quien redirija, al volver a evaluar
    // `!token()` en su propio render, es la unica ruta de redireccion y
    // conserva `state.from` con la ubicacion correcta.
    setUnauthorizedHandler(() => {
      setUsuario(null);
    });

    // El token vive en localStorage -compartido entre pestanas- pero el
    // `usuario` vive en el estado de React, que es por pestana. Sin esto, abrir
    // una segunda pestana con otra cuenta deja a la primera mostrando al
    // usuario viejo mientras sus peticiones ya viajan con el token nuevo:
    // ensena una identidad y actua con otra.
    //
    // El evento `storage` solo se dispara en las OTRAS pestanas, que es justo
    // lo que hace falta: la que hizo el cambio ya actualizo su propio estado.
    function alCambiarElAlmacen(evento: StorageEvent) {
      if (evento.key !== tokenKey) return;

      if (!evento.newValue) {
        // Cerraron sesion en otra pestana.
        setUsuario(null);
        navigate("/login", { replace: true });
        return;
      }
      // Entraron con otra cuenta: volver a resolver para que lo que se muestra
      // corresponda al token que se esta usando.
      setCargando(true);
      sesionActual()
        .then((u) => vigente && setUsuario(u))
        .catch(() => vigente && setUsuario(null))
        .finally(() => vigente && setCargando(false));
    }

    window.addEventListener("storage", alCambiarElAlmacen);

    return () => {
      vigente = false;
      setUnauthorizedHandler(null);
      window.removeEventListener("storage", alCambiarElAlmacen);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function entrar(email: string, clave: string) {
    const sesion = await iniciarSesion(email, clave);
    setToken(sesion.token);
    // El usuario ya viene en la respuesta del login: no hace falta un GET
    // adicional a /auth/session para pintar la sesion recien abierta.
    setUsuario(sesion.usuario);
    setSalidaSinRevocar(false);
  }

  async function salir(): Promise<ResultadoDeSalida> {
    let revocadaEnServidor = true;
    try {
      await cerrarSesion();
    } catch (err) {
      // Un 401 no es "no se pudo revocar": es el servidor diciendo que esa
      // sesion ya no vale, que es exactamente lo que se queria lograr. La
      // incertidumbre real es de red (ErrorDeRed) o un 5xx -ahi si el
      // servidor pudo no haberse enterado-. Sin esta distincion, la sesion
      // que caduca con la pestana abierta dispara el aviso de "el servidor
      // no confirmo la revocacion" en cada logout normal, y un aviso de
      // seguridad que salta en falso con frecuencia es uno que se aprende a
      // ignorar.
      if (!(err instanceof ApiError && err.status === 401)) {
        revocadaEnServidor = false;
      }
    }
    clearToken();
    setUsuario(null);
    setSalidaSinRevocar(!revocadaEnServidor);
    return { revocadaEnServidor };
  }

  return (
    <ContextoSesion.Provider
      value={{ usuario, cargando, salidaSinRevocar, entrar, salir }}
    >
      {children}
    </ContextoSesion.Provider>
  );
}

export function useSesion(): ContextoSesion {
  const ctx = useContext(ContextoSesion);
  if (!ctx) {
    throw new Error("useSesion() se llamo fuera de <ProveedorDeSesion>");
  }
  return ctx;
}
