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

type ContextoSesion = {
  usuario: Usuario | null;
  cargando: boolean;
  entrar: (email: string, clave: string) => Promise<void>;
  salir: () => Promise<void>;
};

const ContextoSesion = createContext<ContextoSesion | null>(null);

export function ProveedorDeSesion({ children }: { children: ReactNode }) {
  const [usuario, setUsuario] = useState<Usuario | null>(null);
  const [cargando, setCargando] = useState(true);
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
  }

  async function salir() {
    // Se sale en local aunque el servidor no responda: "Salir" no depende de la red.
    try {
      await cerrarSesion();
    } catch {
      // Sin aviso: lo que importa es que el token ya no viva en este equipo.
    }
    clearToken();
    setUsuario(null);
  }

  return (
    <ContextoSesion.Provider value={{ usuario, cargando, entrar, salir }}>
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
