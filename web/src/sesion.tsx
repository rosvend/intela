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

    setUnauthorizedHandler(() => {
      setUsuario(null);
      navigate("/login", { replace: true });
    });

    return () => {
      vigente = false;
      setUnauthorizedHandler(null);
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
    try {
      await cerrarSesion();
    } finally {
      clearToken();
      setUsuario(null);
    }
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
