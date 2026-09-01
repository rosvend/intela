const tokenKey = "intela.token";

export function token() {
  return localStorage.getItem(tokenKey) || "";
}

export async function api(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers);
  if (token()) headers.set("Authorization", `Bearer ${token()}`);
  if (
    !(init.body instanceof FormData) &&
    !headers.has("Content-Type") &&
    init.body
  ) {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(path, { ...init, headers });
  if (res.status === 401) {
    localStorage.removeItem(tokenKey);
    if (!path.includes("/sesiones")) window.location.href = "/login";
  }
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
  const ct = res.headers.get("content-type") || "";
  if (ct.includes("json")) return res.json();
  return res;
}

export function setToken(t: string) {
  localStorage.setItem(tokenKey, t);
}

export function nombreDeContentDisposition(cabecera: string): string {
  const m = /filename="([^"]+)"/.exec(cabecera);
  return m?.[1] ?? "";
}

export async function descargar(path: string) {
  const res = await api(path);
  if (!(res instanceof Response)) {
    throw new Error("se esperaba un archivo");
  }
  const blob = await res.blob();
  const nombre =
    nombreDeContentDisposition(res.headers.get("content-disposition") || "") ||
    "liquidacion";
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = nombre;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
