import type { ComponentType, SVGProps } from "react";
import { useEffect, useRef, useState } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import {
  ArrowRightOnRectangleIcon,
  ArrowUpTrayIcon,
  BookOpenIcon,
  ChartPieIcon,
  ChevronDownIcon,
  Cog6ToothIcon,
  DocumentChartBarIcon,
  ExclamationTriangleIcon,
  HomeIcon,
  ReceiptPercentIcon,
  ShieldCheckIcon,
  UsersIcon,
} from "@heroicons/react/24/outline";
import logo from "./logo-intela.png";
import { RUTAS, Seccion, itemsDeNav } from "./navegacion";
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

type Icono = ComponentType<SVGProps<SVGSVGElement>>;

// Icono Heroicons por ruta del sidebar.
const ICONOS_NAV: Record<string, Icono> = {
  "/": HomeIcon,
  "/ingesta": ArrowUpTrayIcon,
  "/catalogo": BookOpenIcon,
  "/titulares": UsersIcon,
  "/distribucion": ChartPieIcon,
  "/anomalias": ExclamationTriangleIcon,
  "/reportes": DocumentChartBarIcon,
  "/deducciones": ReceiptPercentIcon,
  "/auditoria": ShieldCheckIcon,
};

function iniciales(nombre: string): string {
  return nombre
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((parte) => parte[0]?.toUpperCase() ?? "")
    .join("");
}

// Todo el chrome vive en el sidebar; el guard es cosmetico, la autorizacion real es `requiereRol` en el servidor.
export default function Layout() {
  const { usuario, salir } = useSesion();
  const location = useLocation();
  const navigate = useNavigate();
  const [menuAbierto, setMenuAbierto] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  // El `if (!usuario)` va tras todos los hooks: un retorno entre hooks rompe las reglas de React.
  const items = usuario ? itemsDeNav(usuario.rol) : [];

  // "Configuración" lleva al primer modulo visible de esa seccion, o a Inicio.
  const destinoConfiguracion =
    items.find((item) => item.seccion === "configuracion")?.to ?? "/";

  // El menu se cierra con Escape o con clic fuera.
  useEffect(() => {
    if (!menuAbierto) return;
    function alPulsarFuera(evento: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(evento.target as Node)) {
        setMenuAbierto(false);
      }
    }
    function alTeclar(evento: KeyboardEvent) {
      if (evento.key === "Escape") setMenuAbierto(false);
    }
    document.addEventListener("mousedown", alPulsarFuera);
    document.addEventListener("keydown", alTeclar);
    return () => {
      document.removeEventListener("mousedown", alPulsarFuera);
      document.removeEventListener("keydown", alTeclar);
    };
  }, [menuAbierto]);

  // RutaProtegida garantiza sesion; esto solo estrecha el tipo para TypeScript.
  if (!usuario) return null;

  // El guard solo cubre modulos de RUTAS (por prefijo, incluyendo detalles); las utilidades como /estado pasan.
  const modulo = RUTAS.find(
    (r) =>
      r.to === location.pathname ||
      (r.to !== "/" && location.pathname.startsWith(r.to + "/")),
  );
  const autorizado = !modulo || modulo.roles.includes(usuario.rol);

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
                {deLaSeccion.map((item) => {
                  const IconoNav = ICONOS_NAV[item.to] ?? HomeIcon;
                  return (
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
                      <IconoNav
                        className="sidebar-item-icono"
                        aria-hidden="true"
                      />
                      <span className="sidebar-item-texto">{item.label}</span>
                      {item.contador !== undefined && (
                        <span className="sidebar-badge">{item.contador}</span>
                      )}
                    </NavLink>
                  );
                })}
              </div>
            );
          })}
        </nav>
        <div className="sidebar-pie" ref={menuRef}>
          {/* El aviso de salida sin revocar lo muestra Login: al salir este Layout se desmonta. */}
          <button
            type="button"
            className="sidebar-perfil"
            onClick={() => setMenuAbierto((abierto) => !abierto)}
            aria-haspopup="menu"
            aria-expanded={menuAbierto}
            aria-label="Abrir menú de usuario"
          >
            <span className="avatar" aria-hidden="true">
              {iniciales(usuario.nombre)}
            </span>
            <div className="sidebar-usuario">
              <p className="sidebar-nombre">{usuario.nombre}</p>
              <p className="sidebar-rol">{ETIQUETA_ROL[usuario.rol]}</p>
            </div>
            <ChevronDownIcon
              className={
                menuAbierto
                  ? "sidebar-caret sidebar-caret-abierto"
                  : "sidebar-caret"
              }
              aria-hidden="true"
            />
          </button>
          {menuAbierto && (
            <div className="sidebar-menu" role="menu">
              <button
                type="button"
                role="menuitem"
                className="sidebar-menu-item"
                onClick={() => {
                  setMenuAbierto(false);
                  navigate(destinoConfiguracion);
                }}
              >
                <Cog6ToothIcon
                  className="sidebar-menu-icono"
                  aria-hidden="true"
                />
                Configuración
              </button>
              <button
                type="button"
                role="menuitem"
                className="sidebar-menu-item"
                onClick={() => void salir()}
              >
                <ArrowRightOnRectangleIcon
                  className="sidebar-menu-icono"
                  aria-hidden="true"
                />
                Salir
              </button>
            </div>
          )}
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
