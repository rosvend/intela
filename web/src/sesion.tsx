import {
  ReactNode,
  createContext,
  useContext,
  useEffect,
  useState,
} from "react";
import { useNavigate } from "react-router-dom";
import {
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

export function iniciarSesion(email: string, clave: string): Promise<Sesion> {
  return api("/api/auth/session", {
    method: "POST",
    body: JSON.stringify({ email, clave }),
    // Un 401 aqui es "clave mala", no "sesion vencida": no debe disparar la
    // redireccion de sesion expirada, sino dejar que Login muestre el error.
    anonima: true,
  });
}

export const sesionActual = (): Promise<Usuario> => api("/api/auth/session");

export const cerrarSesion = (): Promise<Response> =>
  api("/api/auth/session", { method: "DELETE" });

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

    setUnauthorizedHandler(() => {
      setUsuario(null);
      navigate("/login", { replace: true });
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
    } catch {
      // No se relanza: el logout local tiene que funcionar aunque el servidor
      // este caido o no haya red. Pero tampoco se traga en silencio, porque la
      // sesion sigue viva alla y en un equipo compartido eso importa.
      revocadaEnServidor = false;
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
