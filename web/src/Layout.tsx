import { NavLink, Outlet, useLocation } from "react-router-dom";
import logo from "./logo-intela.png";
import { RUTAS, Seccion, itemsDeNav, puedeVer } from "./navegacion";
import { Rol, useSesion } from "./sesion";

const ETIQUETA_ROL: Record<Rol, string> = {
  administrador: "Administrador",
  distribucion: "Distribución",
  contabilidad: "Contabilidad",
  auditor: "Auditor",
  titular: "Titular",
};

const TITULO_SECCION: Record<Seccion, string> = {
  principal: "Principal",
  configuracion: "Configuración",
};

const SECCIONES: readonly Seccion[] = ["principal", "configuracion"];

function iniciales(nombre: string): string {
  return nombre
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((parte) => parte[0]?.toUpperCase() ?? "")
    .join("");
}

/**
 * Sin topbar: el mockup lo elimino por completo, todo el chrome vive en el
 * sidebar (issue #19, seccion "Pase visual"). El guard de `autorizado` es
 * cosmetico -evita mostrar una pantalla inutil cuando la URL no esta en la
 * nav del rol actual- y no reemplaza la autorizacion real, que es el
 * `requiereRol` de #17 en el servidor: ocultar un link no protege nada.
 */
export default function Layout() {
  const { usuario, salir } = useSesion();
  const location = useLocation();

  // RutaProtegida garantiza sesion antes de montar Layout; esto solo evita
  // que TypeScript trate a `usuario` como nulable de aqui en adelante.
  if (!usuario) return null;

  const items = itemsDeNav(usuario.rol);

  // El guard solo se aplica a rutas que SON un modulo del mockup (RUTAS): una
  // pagina fuera de ese modelo -como /estado, un utilitario de diagnostico
  // que no aparece en la nav de nadie- no es competencia del guard y se deja
  // pasar. `puedeVer` no puede distinguir "tu rol no ve esto" de "esto no es
  // un modulo": esa distincion se hace aqui.
  const esModuloDelMockup = RUTAS.some((r) => r.to === location.pathname);
  const autorizado =
    !esModuloDelMockup || puedeVer(usuario.rol, location.pathname);

  return (
    <div className="shell">
      <aside className="sidebar">
        <span
          className="sidebar-logo"
          style={{ maskImage: `url(${logo})`, WebkitMaskImage: `url(${logo})` }}
          role="img"
          aria-label="Intela"
        />
        <nav className="sidebar-nav">
          {SECCIONES.map((seccion) => {
            const deLaSeccion = items.filter(
              (item) => item.seccion === seccion,
            );
            if (deLaSeccion.length === 0) return null;
            return (
              <div key={seccion} className="sidebar-seccion">
                <p className="sidebar-titulo-seccion">
                  {TITULO_SECCION[seccion]}
                </p>
                {deLaSeccion.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={item.to === "/"}
                    className={({ isActive }) =>
                      isActive
                        ? "sidebar-item sidebar-item-activo"
                        : "sidebar-item"
                    }
                  >
                    <span>{item.label}</span>
                    {item.contador !== undefined && (
                      <span className="sidebar-badge">{item.contador}</span>
                    )}
                  </NavLink>
                ))}
              </div>
            );
          })}
        </nav>
        <div className="sidebar-pie">
          <span className="avatar" aria-hidden="true">
            {iniciales(usuario.nombre)}
          </span>
          <div className="sidebar-usuario">
            <p className="sidebar-nombre">{usuario.nombre}</p>
            <p className="sidebar-rol">{ETIQUETA_ROL[usuario.rol]}</p>
          </div>
          {/*
            El aviso de "el servidor no revoco" NO se pinta aqui: al salir,
            `usuario` pasa a null y este Layout se desmonta entero, asi que
            nadie alcanzaria a leerlo. Vive en el contexto y lo muestra Login,
            que es donde se aterriza.
          */}
          <button
            type="button"
            className="sidebar-salir"
            onClick={() => void salir()}
            aria-label="Cerrar sesión"
          >
            Salir
          </button>
        </div>
      </aside>
      <main className="contenido">
        {autorizado ? <Outlet /> : <NoAutorizado />}
      </main>
    </div>
  );
}

function NoAutorizado() {
  return (
    <section>
      <h1>No autorizado</h1>
      <p>Tu rol no tiene acceso a esta pantalla.</p>
    </section>
  );
}
